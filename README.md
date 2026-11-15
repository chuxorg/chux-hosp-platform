# Hospitality Platform

Greenfield, **modular Hospitality Platform**: independent PMS, Experiences, Loyalty, Procurement, and Operations modules inside one deployable application. **PMS is the first functional delivery**; Thai Cooking demonstrates the separate Experiences framework later. No distributed services, real payment processing or live integrations required initially.

## Project references
- [GitHub Project #14](https://github.com/orgs/chuxorg/projects/14)
- [Dropbox Hospitality knowledge base](https://www.dropbox.com/home/Hospitality)
- [Codex instructions](AGENTS.md)
- [SDLC](docs/SDLC.md)
- [Local setup and tests](docs/LOCAL_SETUP.md)
- [ADR-0001 historical Python decision (superseded)](docs/adr/ADR-0001-python-fastapi-postgres.md)
- [ADR-0003 Go and PostgreSQL replacement decision](docs/adr/ADR-0003-go-postgres-defer-redis.md)
- [ADR-0002 modular platform architecture](docs/adr/ADR-0002-modular-hospitality-platform.md)
- [Approved module boundary map (Dropbox)](https://www.dropbox.com/scl/fi/4ia47cl33y9ibtighkm3j/Hospitality-Modular-Platform-Module-Boundaries-and-Integration-Contracts.paper?rlkey=x5zlc6w9s6u2w9yqh94yye8yk&dl=0)

## First PMS module boundary (Issue #20)

The Go composition root mounts a standalone PMS HTTP package at `/pms/v1/`.
`GET /pms/v1/status` currently reports `{"module":"pms","status":"bootstrap"}`:
it is a **module wiring diagnostic**, not an implemented hotel operations service.
No Experiences, Loyalty, Procurement or Operations module is required.
Read [Module boundaries and composition](docs/MODULES.md) for checked dependency rules,
data-ownership limitations and the next approved PMS implementation steps.

## Go/PostgreSQL bootstrap

Go **1.27.4**, standard `net/http`, pgx pooling and PostgreSQL **17**. Goose SQL migrations are embedded in one version-identifiable executable. ADR-0003 replaces Python/SQLite; ADR-0002's modular-monolith boundaries remain unchanged. No business module, Redis or broker is introduced.

Prerequisites: Go 1.27.4, Git, Docker Engine 27+ (or Docker Desktop) and Compose v2.24+.

```bash
git config core.hooksPath .githooks
docker compose up -d --wait postgres
export DATABASE_URL='postgres://hospitality:local_only_change_me@127.0.0.1:5432/hospitality?sslmode=disable'
go run ./cmd/hospitality migrate up
go run ./cmd/hospitality
# GET http://127.0.0.1:8000/healthz and /readyz
```

Or build/run the app and PostgreSQL together:

```bash
COMMIT="$(git rev-parse HEAD)" BUILD_DATE="$(git show -s --format=%cI HEAD)" docker compose up -d --build --wait
docker compose run --rm app migrate up
```

Compose credentials are **development-only examples** and ports bind to localhost. Existing Alembic databases are refused rather than reset; choose a fresh development database using the [safe transition instructions](docs/LOCAL_SETUP.md#existing-alembic-development-databases). Startup does not automatically migrate or delete data.

```bash
./scripts/validate.sh
# Provision once, without resetting an existing DB:
docker compose exec postgres createdb -U hospitality hospitality_test
TEST_DATABASE_URL='postgres://hospitality:local_only_change_me@127.0.0.1:5432/hospitality_test?sslmode=disable' ./scripts/test-integration.sh
VERSION=0.1.0-dev ./scripts/build.sh
./build/hospitality --version
```

See [local setup](docs/LOCAL_SETUP.md) for configuration, migrations/rollback, race tests, isolated test database safeguards, container builds and standalone deployment. CI verifies Go checks, real PostgreSQL and the container, aggregated by the unchanged required `bootstrap-validation` check. Original PHP, databases, real guest data and credentials stay outside Git.

Implemented packages: `cmd/hospitality` (composition/CLI/server), `internal/platform` (configuration, pooling configuration and HTTP probes), `migrations` (platform-owned SQL). Add module-owned domain/application/infrastructure/API packages only with actual approved stories.
