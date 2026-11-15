//go:build integration

package platform

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/chuxorg/chux-hosp-platform/migrations"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Issue #31/ADR-0003: actual PostgreSQL connectivity, transactions and migration lifecycle.
func TestPostgresLifecycle(t *testing.T) {
	cfg, err := testDatabaseConfig(os.Getenv("TEST_DATABASE_URL"), os.Getenv("DATABASE_URL"))
	if err != nil {
		t.Fatal(err)
	}
	cfg.ConnectTimeout = 3 * time.Second
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	admin, err := pgx.ConnectConfig(ctx, cfg)
	if err != nil {
		t.Fatal("cannot connect to dedicated test database")
	}
	defer admin.Close(context.Background())
	// Random, exclusively owned database: never drop tables/data in the supplied DB.
	suffix := make([]byte, 8)
	if _, err = rand.Read(suffix); err != nil {
		t.Fatal(err)
	}
	name := "hospitality_it_" + hex.EncodeToString(suffix) + "_test"
	quoted := pgx.Identifier{name}.Sanitize()
	if _, err = admin.Exec(ctx, "CREATE DATABASE "+quoted); err != nil {
		t.Fatal("test role needs CREATEDB permission")
	}
	defer func() {
		cleanup, done := context.WithTimeout(context.Background(), 10*time.Second)
		defer done()
		if _, err := admin.Exec(cleanup, "DROP DATABASE "+quoted+" WITH (FORCE)"); err != nil {
			t.Error("failed to remove exclusively owned test database")
		}
	}()
	child := cfg.Copy()
	child.Database = name
	databaseURL := child.ConnString()
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()

	t.Run("initial migration and idempotence", func(t *testing.T) {
		for range 2 {
			v, err := migrations.Run(ctx, databaseURL, "up")
			if err != nil || v != 1 {
				t.Fatalf("migration version %d: %v", v, err)
			}
		}
		var keyLen, valueLen int
		if err := pool.QueryRow(ctx, "SELECT character_maximum_length FROM information_schema.columns WHERE table_name='platform_metadata' AND column_name='key'").Scan(&keyLen); err != nil {
			t.Fatal(err)
		}
		if err := pool.QueryRow(ctx, "SELECT character_maximum_length FROM information_schema.columns WHERE table_name='platform_metadata' AND column_name='value'").Scan(&valueLen); err != nil {
			t.Fatal(err)
		}
		if keyLen != 80 || valueLen != 250 {
			t.Fatal("metadata schema changed")
		}
	})
	if t.Failed() {
		return
	}
	t.Run("real readiness", func(t *testing.T) {
		w := httptest.NewRecorder()
		Handler(pool, time.Second).ServeHTTP(w, httptest.NewRequest("GET", "/readyz", nil))
		if w.Code != 200 || w.Body.String() != "{\"status\":\"ready\"}\n" {
			t.Fatalf("readiness failed: %d %s", w.Code, w.Body.String())
		}
	})
	t.Run("transaction commit and read write", func(t *testing.T) {
		tx, err := pool.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		defer tx.Rollback(ctx)
		value := "synthetic ' quoted value"
		if _, err = tx.Exec(ctx, "INSERT INTO platform_metadata (key,value) VALUES ($1,$2)", "commit_test", value); err != nil {
			t.Fatal(err)
		}
		if err = tx.Commit(ctx); err != nil {
			t.Fatal(err)
		}
		var got string
		if err = pool.QueryRow(ctx, "SELECT value FROM platform_metadata WHERE key=$1", "commit_test").Scan(&got); err != nil || got != value {
			t.Fatalf("committed value was not persisted: %v", err)
		}
	})
	t.Run("transaction rollback", func(t *testing.T) {
		tx, err := pool.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		defer tx.Rollback(ctx)
		if _, err = tx.Exec(ctx, "INSERT INTO platform_metadata (key,value) VALUES ($1,$2)", "rollback_test", "not committed"); err != nil {
			t.Fatal(err)
		}
		if err = tx.Rollback(ctx); err != nil {
			t.Fatal(err)
		}
		var count int
		if err = pool.QueryRow(ctx, "SELECT count(*) FROM platform_metadata WHERE key=$1", "rollback_test").Scan(&count); err != nil || count != 0 {
			t.Fatalf("rollback persisted data: %v", err)
		}
	})
	t.Run("constraints preserved", func(t *testing.T) {
		if _, err := pool.Exec(ctx, "INSERT INTO platform_metadata (key,value) VALUES ('null_value',NULL)"); err == nil {
			t.Fatal("NOT NULL missing")
		}
		if _, err := pool.Exec(ctx, "INSERT INTO platform_metadata (key,value) VALUES ('commit_test','duplicate')"); err == nil {
			t.Fatal("primary key missing")
		}
	})
	t.Run("migration rollback and reapply", func(t *testing.T) {
		v, err := migrations.Run(ctx, databaseURL, "down")
		if err != nil || v != 0 {
			t.Fatalf("rollback version %d: %v", v, err)
		}
		var absent bool
		if err = pool.QueryRow(ctx, "SELECT to_regclass('platform_metadata') IS NULL").Scan(&absent); err != nil || !absent {
			t.Fatal("rollback left metadata table")
		}
		if _, err = migrations.Run(ctx, databaseURL, "up"); err != nil {
			t.Fatal(err)
		}
	})
	t.Run("Alembic refusal preserves data", func(t *testing.T) {
		if _, err := pool.Exec(ctx, "CREATE TABLE alembic_version (version_num VARCHAR(32) PRIMARY KEY); INSERT INTO alembic_version VALUES ('0001_platform_metadata')"); err != nil {
			t.Fatal(err)
		}
		if _, err := migrations.Run(ctx, databaseURL, "down"); err == nil {
			t.Fatal("adopted Alembic database")
		}
		var count int
		if err := pool.QueryRow(ctx, "SELECT count(*) FROM alembic_version").Scan(&count); err != nil || count != 1 {
			t.Fatal("Alembic history changed")
		}
		var exists bool
		if err := pool.QueryRow(ctx, "SELECT to_regclass('platform_metadata') IS NOT NULL").Scan(&exists); err != nil || !exists {
			t.Fatal("refusal changed metadata schema")
		}
	})
}
