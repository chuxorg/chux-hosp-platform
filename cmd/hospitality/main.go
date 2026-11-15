package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/chuxorg/chux-hosp-platform/internal/platform"
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
	if len(args) > 0 && !isMigration {
		return errors.New("usage: hospitality [--version | migrate up|status | migrate down --allow-down]")
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
	// Pool connects lazily: liveness works during DB outages; readiness reports 503.
	listener, err := net.Listen("tcp", cfg.HTTPAddr)
	if err != nil {
		return errors.New("cannot listen on HTTP_ADDR")
	}
	return serve(ctx, listener, platform.Handler(pool, cfg.DatabaseTimeout))
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
