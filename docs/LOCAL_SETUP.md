# Local development: Go + PostgreSQL

Use **Go 1.27.4** (`.go-version` and `go.mod`), Git, Docker Engine **27+** / Docker Desktop with Linux containers, and Docker Compose **v2.24+**. No Python, SQLite or Redis runtime is required. `scripts/*.sh` select the pinned Go toolchain; a Go installation supporting automatic toolchain downloads can fetch it. Direct `go` commands also respect the minimum version in `go.mod`. Keep real credentials, legacy source, databases and guest data outside Git.

For the Mac Auth0 localhost sign-in/sign-out test, use the
[authentication runbook](AUTHENTICATION.md#mac-localhost-test--issue-52) and its
ignored `.env.local` configuration. It reuses these Compose services.

## Host application, Docker PostgreSQL

```bash
git config core.hooksPath .githooks
docker compose up -d --wait postgres
export DATABASE_URL='postgres://hospitality:local_only_change_me@127.0.0.1:5432/hospitality?sslmode=disable'
go run ./cmd/hospitality migrate up
go run ./cmd/hospitality
```

In another terminal:

```bash
curl --fail http://127.0.0.1:8000/healthz
curl --fail http://127.0.0.1:8000/readyz
```

`GET /healthz` returns `{"status":"ok"}` without querying PostgreSQL. `GET /readyz` pings the pool and returns `{"status":"ready"}` or HTTP 503 `{"detail":"database unavailable"}`. Readiness checks connectivity, not migration currency. Apply migrations explicitly before deploying features; application startup never changes the schema. The server starts during a database outage and handles SIGINT/SIGTERM by draining requests for up to ten seconds.

Environment:

| Variable | Meaning |
| --- | --- |
| `DATABASE_URL` | Required PostgreSQL URL, with explicit host/database. No SQLite fallback. URL-encode username/password characters. Never log or commit production URLs. |
| `HTTP_ADDR` | `host:port`, default `127.0.0.1:8000`; container uses `0.0.0.0:8000`. |
| `DATABASE_TIMEOUT` | Connection/readiness deadline, default `2s`, greater than zero and at most `30s`. |
| `POSTGRES_USER`, `POSTGRES_PASSWORD`, `POSTGRES_DB` | Compose-only overrides; defaults `hospitality`, `local_only_change_me`, `hospitality`. |
| `POSTGRES_PORT`, `APP_PORT` | Compose host ports, defaults 5432/8000, bound to localhost. |

Pool: at most ten connections, five-minute idle and thirty-minute lifetime limits. HTTP headers/read/write/idle timeouts are explicit. Production must supply its own secrets, PostgreSQL TLS configuration (for example `sslmode=verify-full` and trusted CA), network restrictions and proxy/TLS termination. Compose defaults are **local development examples**, not deployment credentials. Simple credentials work in Compose's interpolated URL; percent-encode reserved URI characters when supplying overrides. Changing `POSTGRES_*` does not reconfigure credentials in an existing initialized volume.

## Container application and database

```bash
VERSION=0.1.0-dev COMMIT="$(git rev-parse HEAD)" BUILD_DATE="$(git show -s --format=%cI HEAD)" \
  docker compose up -d --build --wait
docker compose run --rm app migrate up
docker compose exec app hospitality --version
curl --fail http://127.0.0.1:8000/readyz
docker compose down
```

PostgreSQL remains a separate service and is reached at `postgres:5432` from the app. Compose waits for PostgreSQL health; the app healthcheck exercises readiness. The runtime is a non-root, read-only Alpine image with only the static executable, OS utilities and CA certificates; migrations are compiled into the binary. `.dockerignore` permits only build inputs. `docker compose down` retains the database volume. **Do not run `down -v` on an environment containing wanted data.** No automatic database migration or volume reset occurs on startup.

Build a container without Compose:

```bash
docker build --build-arg VERSION=0.1.0-dev \
  --build-arg COMMIT="$(git rev-parse HEAD)" \
  --build-arg BUILD_DATE="$(git show -s --format=%cI HEAD)" \
  -t hospitality:0.1.0-dev .
```

## Versioned migrations and safe rollback

Goose **v3.30.1** is used as a Go library with the pgx driver, embedded SQL and PostgreSQL advisory migration locking. This avoids a separate CLI and its multi-database drivers in the runtime. `migrations/00001_platform_metadata.sql` preserves `platform_metadata(key VARCHAR(80) PRIMARY KEY, value VARCHAR(250) NOT NULL)`. Goose tracks versions in `goose_db_version`. Keep applied migrations immutable; add reviewed five-digit versioned SQL files in `migrations/` and rebuild the executable.

```bash
go run ./cmd/hospitality migrate status
go run ./cmd/hospitality migrate up
# Destructive: explicitly authorized rollback of the last migration only.
# For the initial migration this DROPS platform_metadata and its contents.
go run ./cmd/hospitality migrate down --allow-down
```

Migrations use transactions. Roll back only against an authorized database after checking data loss and backups; use forward repair for data-bearing deployments. Apply migrations using one deployment operator, not every application replica. Commands have a one-minute deadline; future long-running migrations need a separately reviewed strategy. Driver errors are not printed because they can expose secrets; failures return a safe error and nonzero exit. Inspect database state with `migrate status` and an authorized PostgreSQL client.

### Existing Alembic development databases

This replacement is **not an in-place Alembic migration**. It refuses a database containing `alembic_version`; it will not rename history, stamp versions or drop existing objects. A conflicting pre-existing metadata table also causes migration failure rather than replacement.

Preferred procedure: retain the old database/volume, choose a **new database name**, create it using your authorized database owner, point `DATABASE_URL` to it and apply Goose migrations. For example, against the local development service:

```bash
docker compose exec postgres createdb -U hospitality hospitality_go_dev
export DATABASE_URL='postgres://hospitality:local_only_change_me@127.0.0.1:5432/hospitality_go_dev?sslmode=disable'
go run ./cmd/hospitality migrate up
```

To use that database in container mode, set `POSTGRES_DB=hospitality_go_dev`; the database must already exist when reusing a volume. The old database remains available for inspection. No production or legacy data is migrated by this story.

If the owner explicitly decides an old **development-only** database is disposable, first stop all clients, confirm its host/name/owner and that no wanted data exists, take a backup if needed, then use an authorized PostgreSQL client to drop/recreate only that database and apply migrations. This is a manual destructive operation, **not** a setup or validation step. Never delete the shared Compose volume merely to resolve migration history.

## Validation and isolated integration tests

```bash
./scripts/validate.sh            # gofmt, dependency verification, vet, unit tests, build
go test -race ./...             # race-enabled unit tests
./scripts/test-build.sh          # actual executable with embedded metadata
```

The pre-commit hook runs routine validation. Install it for every clone. Hooks can be bypassed; CI repeats the checks. Formatting repair: `gofmt -w cmd internal migrations`. Pin updates together in `.go-version`, `go.mod`, `Dockerfile` and these docs; CI reads `.go-version`.

Database tests are explicit and **never skipped** when invoked. Provision a separate test database; use a dedicated test role with `CREATEDB` and no access to real data:

```bash
docker compose up -d --wait postgres
# Run once. An existing database is not dropped/reset by this command.
docker compose exec postgres createdb -U hospitality hospitality_test
export TEST_DATABASE_URL='postgres://hospitality:local_only_change_me@127.0.0.1:5432/hospitality_test?sslmode=disable'
./scripts/test-integration.sh
# Or run all routine, race and integration checks before review:
FULL_VALIDATION=1 ./scripts/validate.sh
```

The URL must name a database ending in `_test` and must differ from the configured application database. Tests create a cryptographically unique `hospitality_it_<random>_test` database, exercise it exclusively, and drop only that database on cleanup. The supplied provisioning database is never reset. CI provisions a fresh PostgreSQL 17 service. An interrupted process may leave an `hospitality_it_*_test` database; inspect ownership and confirm it is unused before manually removing it. Do not automate wildcard cleanup.

Coverage includes configuration/security checks, health and unavailable readiness, graceful shutdown, version output, successful real PostgreSQL readiness, original schema/constraints, parameterized reads/writes, transaction commit/rollback, migration up/idempotence/down/reapply and refusal to adopt Alembic history. The actual built executable's linker metadata is also checked. This proves the bootstrap, not PMS functionality or concurrency invariants of future modules.

## Standalone binary

```bash
VERSION=0.1.0-dev ./scripts/build.sh
./build/hospitality --version
./build/hospitality migrate up
./build/hospitality
```

The script embeds version, full Git commit SHA and commit time (overridable via `COMMIT`/`BUILD_DATE`), builds with `CGO_ENABLED=0`, `-trimpath`, and no ambient VCS build stamps. Identical source/toolchain/metadata inputs give repeatable builds. The binary is specific to the build OS/architecture; cross-compile with e.g. `GOOS=linux GOARCH=amd64 ./scripts/build.sh`. It runs independently of Docker, given PostgreSQL/network access, database configuration and trusted TLS certificates. No `.env` auto-loader is used; export variables explicitly or provide them through your deployment environment.

## CI gate

Quality runs `go-checks`, `postgres-tests` (race-enabled real PostgreSQL 17 tests), and `container-smoke` (Docker build, non-root runtime, HTTP and migration CLI checks). The unchanged required **`bootstrap-validation`** aggregate uses `if: always()` and requires all three results to be `success`; failed, cancelled or skipped dependencies fail the gate. Conventional Commit PR title validation stays mandatory. No branch protections are weakened.
