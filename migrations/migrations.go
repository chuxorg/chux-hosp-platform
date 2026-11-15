// Package migrations embeds platform-owned SQL; future modules own their migrations.
package migrations

import (
	"context"
	"embed"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
	"github.com/pressly/goose/v3/lock"
)

//go:embed *.sql
var files embed.FS

// Run never automatically adopts an Alembic database. See LOCAL_SETUP for the
// explicitly disposable-development reset procedure. Migrations are transactional.
func Run(ctx context.Context, databaseURL, command string) (int64, error) {
	if command != "up" && command != "down" && command != "status" {
		return 0, errors.New("migration command must be up, down or status")
	}
	cfg, err := pgx.ParseConfig(databaseURL)
	if err != nil {
		return 0, errors.New("invalid PostgreSQL configuration")
	}
	cfg.ConnectTimeout = 5 * time.Second
	db := stdlib.OpenDB(*cfg)
	defer db.Close()
	db.SetMaxOpenConns(2)
	var alembic bool
	if err := db.QueryRowContext(ctx, "SELECT to_regclass('alembic_version') IS NOT NULL").Scan(&alembic); err != nil {
		return 0, errors.New("cannot inspect migration database")
	}
	if alembic {
		return 0, errors.New("Alembic database detected: use the documented disposable-development reset; no schema was changed")
	}
	locker, err := lock.NewPostgresSessionLocker()
	if err != nil {
		return 0, err
	}
	provider, err := goose.NewProvider(goose.DialectPostgres, db, files, goose.WithSessionLocker(locker))
	if err != nil {
		return 0, err
	}
	switch command {
	case "up":
		_, err = provider.Up(ctx)
	case "down":
		_, err = provider.Down(ctx)
	}
	if err != nil {
		return 0, err
	}
	return provider.GetDBVersion(ctx)
}
