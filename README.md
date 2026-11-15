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

## PMS configuration (Issue #22)

The first PMS business slice supports **Property → RoomType → individual Room** create/read/replace flows under `/pms/v1/`, using PostgreSQL-owned `pms_*` tables and domain/application/infrastructure/API boundaries. Apply migrations before issuing requests.

See [PMS configuration API, data rules, tests and security caveats](docs/PMS_CONFIGURATION.md). **These endpoints are development-only and have no authentication or tenant authorization**; keep the service on localhost/isolation and use synthetic data. Reservations, guests, folios and optional modules are not implemented.

## PMS nightly rates and inventory eligibility (Issue #23)

Set/read one current positive THB minor-unit nightly rate per RoomType and query eligible physical rooms using hotel-local `[check_in, check_out)` calendar dates. Rate preparation under inactive parents is allowed; search requires the Property, RoomType, Room and rate all active. **This is inventory eligibility, not reservation-aware availability or a booking guarantee.** Results explicitly report `reservation_conflicts_checked: false`, are date-independent for valid intervals, and create no hold/allocation. Reservation-aware behavior belongs to Issue #24.

See [accepted rules, API examples, schema/rollback and TST-PMS-001-02](docs/PMS_RATES_AND_ELIGIBILITY.md). Apply migration `00003_pms_room_type_rates.sql` before using these development-only endpoints. No tax, discount, seasonal/agent rate or stay-total calculation is implemented.

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

Implemented packages: `cmd/hospitality` (composition/CLI/server), `internal/platform` (configuration, pooling configuration and HTTP probes), `internal/modules/pms` (owned domain/application/infrastructure/API), and `migrations` (platform- and PMS-owned SQL). Add further modules only with actual approved stories.
