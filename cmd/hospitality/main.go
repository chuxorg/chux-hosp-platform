package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/chuxorg/chux-hosp-platform/internal/platform"
	platformauth "github.com/chuxorg/chux-hosp-platform/internal/platform/auth"
	"github.com/chuxorg/chux-hosp-platform/migrations"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Overridden by scripts/build.sh; defaults also identify go run builds.
var version = "dev"
var commit = "unknown"
var buildDate = "unknown"

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	if err := run(ctx, os.Args[1:], os.Getenv, os.Stdout); err != nil {
		// Driver errors may contain credentials or server details; log only safe errors.
		slog.Error(err.Error())
		os.Exit(1)
	}
}

func run(ctx context.Context, args []string, getenv func(string) string, out io.Writer) error {
	if len(args) == 1 && args[0] == "--version" {
		_, err := fmt.Fprintf(out, "hospitality %s commit=%s built=%s\n", version, commit, buildDate)
		return err
	}
	isMigration := len(args) >= 1 && args[0] == "migrate"
	isAuthAdmin := len(args) >= 1 && args[0] == "auth"
	if len(args) > 0 && !isMigration && !isAuthAdmin {
		return errors.New("usage: hospitality [--version | migrate up|status | migrate down --allow-down | auth provision-user|revoke-user]")
	}
	if isMigration && !(len(args) == 2 && (args[1] == "up" || args[1] == "status") || len(args) == 3 && args[1] == "down" && args[2] == "--allow-down") {
		return errors.New("usage: hospitality migrate up|status | migrate down --allow-down (drops the last migration's objects)")
	}
	cfg, err := platform.LoadConfig(getenv)
	if err != nil {
		return err
	}
	if isMigration {
		migrationCtx, cancel := context.WithTimeout(ctx, time.Minute)
		defer cancel()
		v, err := migrations.Run(migrationCtx, cfg.DatabaseURL, args[1])
		if err != nil {
			return errors.New("migration failed; verify connectivity and migration state (Alembic databases require the documented reset)")
		}
		_, err = fmt.Fprintf(out, "database migration version=%d\n", v)
		return err
	}
	pc, err := platform.PoolConfig(cfg)
	if err != nil {
		return err
	}
	pool, err := pgxpool.NewWithConfig(ctx, pc)
	if err != nil {
		return errors.New("cannot initialize PostgreSQL pool")
	}
	defer pool.Close()
	if isAuthAdmin {
		return runAuthAdmin(ctx, pool, args[1:], out)
	}
	var authHandler http.Handler
	if cfg.Auth.Enabled {
		providerCtx, providerCancel := context.WithTimeout(ctx, 10*time.Second)
		provider, providerErr := platformauth.NewOIDCProvider(providerCtx, cfg.Auth.IssuerURL, cfg.Auth.ClientID, cfg.Auth.ClientSecret, cfg.Auth.RedirectURL)
		providerCancel()
		if providerErr != nil {
			return providerErr
		}
		store := platformauth.NewStore(pool)
		cleanupCtx, cleanupCancel := context.WithTimeout(ctx, cfg.DatabaseTimeout)
		cleanupErr := store.Cleanup(cleanupCtx)
		cleanupCancel()
		if cleanupErr != nil {
			return errors.New("authentication store unavailable")
		}
		authHandler = platformauth.NewHandler(store, provider, platformauth.Config{
			IssuerURL: cfg.Auth.IssuerURL, LogoutReturnURL: cfg.Auth.LogoutReturnURL,
			CookieSecure: cfg.Auth.CookieSecure, IdleTimeout: cfg.Auth.SessionIdleTimeout,
			AbsoluteLifetime: cfg.Auth.SessionAbsoluteLife, DatabaseTimeout: cfg.DatabaseTimeout,
		})
	}
	// Pool connects lazily: liveness works during DB outages; readiness reports 503.
	listener, err := net.Listen("tcp", cfg.HTTPAddr)
	if err != nil {
		return errors.New("cannot listen on HTTP_ADDR")
	}
	return serve(ctx, listener, composeHandler(pool, cfg.DatabaseTimeout, authHandler))
}

func runAuthAdmin(ctx context.Context, pool *pgxpool.Pool, args []string, out io.Writer) error {
	if len(args) == 0 || (args[0] != "provision-user" && args[0] != "revoke-user") {
		return errors.New("usage: hospitality auth provision-user|revoke-user --issuer URL --subject SUBJECT [--display-name NAME]")
	}
	command := args[0]
	flags := flag.NewFlagSet("auth "+command, flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	issuer := flags.String("issuer", "", "OIDC issuer")
	subject := flags.String("subject", "", "OIDC subject")
	displayName := flags.String("display-name", "", "display name")
	if err := flags.Parse(args[1:]); err != nil || flags.NArg() != 0 {
		return errors.New("invalid auth administration arguments")
	}
	u, err := url.Parse(*issuer)
	if err != nil || u.Scheme != "https" || u.Host == "" || len(*issuer) > 500 || *subject == "" || len(*subject) > 500 {
		return errors.New("issuer must be HTTPS and subject must be non-empty")
	}
	store := platformauth.NewStore(pool)
	adminCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	if command == "provision-user" {
		if *displayName == "" || len(*displayName) > 200 {
			return errors.New("display-name must be 1–200 characters")
		}
		id, err := store.ProvisionUser(adminCtx, *issuer, *subject, *displayName)
		if err != nil {
			return errors.New("user provisioning failed")
		}
		_, err = fmt.Fprintf(out, "provisioned hospitality user id=%d with no roles or resource grants\n", id)
		return err
	}
	if err := store.RevokeUser(adminCtx, *issuer, *subject); err != nil {
		if errors.Is(err, platformauth.ErrNotFound) {
			return errors.New("provisioned user not found")
		}
		return errors.New("user revocation failed")
	}
	_, err = fmt.Fprintln(out, "revoked hospitality user and active sessions")
	return err
}

func serve(ctx context.Context, listener net.Listener, handler http.Handler) error {
	server := &http.Server{Handler: handler, ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 10 * time.Second, WriteTimeout: 10 * time.Second, IdleTimeout: time.Minute}
	result := make(chan error, 1)
	go func() { result <- server.Serve(listener) }()
	slog.Info("HTTP server started", "address", listener.Addr().String())
	select {
	case err := <-result:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return errors.New("HTTP server failed")
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := server.Shutdown(shutdownCtx); err != nil {
			_ = server.Close()
			return errors.New("HTTP shutdown deadline exceeded")
		}
		<-result
		return nil
	}
}
