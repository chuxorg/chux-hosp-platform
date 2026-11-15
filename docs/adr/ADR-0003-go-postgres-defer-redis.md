# ADR-0003: Go backend, PostgreSQL-first and defer Redis

Status: **Accepted direction by owner on 2026-11-15; documentation and implementation PRs require their own review and merge**
Date: 2026-11-15
Owner: Hospitality project owner
Supersedes: **ADR-0001** technology choice (Python/FastAPI, SQLAlchemy/Alembic and SQLite portability); it does **not** supersede ADR-0002 modular boundaries
Refs: [Issue #31](https://github.com/chuxorg/chux-hosp-platform/issues/31), [ADR-0002](ADR-0002-modular-hospitality-platform.md), [approved business direction in Dropbox](https://www.dropbox.com/scl/fi/zu6lowh2qhiumdxenbbvy/03-Approved-Greenfield-Specification-REQ.paper?rlkey=0ue4d29y20azrkjs0yg4ai0cu&dl=0)

## Context

Hospitality remains a small, owner-led modular monolith: first a standalone PMS, then optional Experiences/Thai Cooking, Procurement, Operations and Loyalty. The existing repository contains only an initial Python scaffold, health/readiness API, migration and five scaffold tests, **not** PMS business functionality. The owner prefers Go for compile-time type checks, straightforward packages/interfaces and a versioned compiled executable. A costly post-implementation rewrite is avoidable by deciding now.

The early SQLite/PostgreSQL dual-backend path is less attractive for hotel room allocation and other transactional invariants that must be verified in PostgreSQL. Redis may eventually help high-volume repeated catalog/read traffic, but no measured bottleneck or cache consistency requirement exists.

## Decision

1. **Backend language and build:** Go. Keep a single module and one executable initially, with module-owned domain/application/infrastructure/API packages added as real PMS stories are implemented. A release build is labeled with semantic version and commit hash (e.g. build metadata embedded with `-ldflags -X`); keep binary/source/release provenance explicit. Go binaries are platform-specific build artifacts, **not** containers holding PostgreSQL or all external dependencies.
2. **HTTP:** start with standard `net/http`; add a minimal router only when endpoint ergonomics warrant it. Publish OpenAPI contracts when consumer-facing API design stabilizes.
3. **PostgreSQL is the required relational backend** for local, CI and deployment use. Use `pgx` for DB connectivity; consider `sqlc` to generate checked typed SQL once concrete queries make it valuable. No SQLite compatibility target.
4. **Migrations:** use versioned SQL migrations with a Go-compatible migration runner (choose between Goose/Atlas or a similarly established tool in implementation Issue #31). Never run destructive integration tests against a non-dedicated test database.
5. **Local environment:** Docker Compose provides PostgreSQL (and optionally builds/runs the Go app). Secure per-environment configuration; illustrative dev credentials only, no live secrets or data.
6. **Tests and CI:** `go test ./...`, `go test -race ./...`, `go vet ./...`, `gofmt`, PostgreSQL migration and actual read/write/transaction tests. Preserve the master branch required `bootstrap-validation` job name as the aggregate quality gate until ruleset changes are explicitly coordinated. Replace Python-specific workflow, packages and validation as one coherent transition PR.
7. **Modularity:** ADR-0002 remains accepted without change. PMS, Experiences, Procurement, Operations and Loyalty own their behavior/data; integrate using typed in-process ports and optional events, not direct cross-module persistence or a broker.
8. **Redis deferred:** do **not** provision Redis, introduce its client, or create a speculative generic cache interface in Phase 1. Start with correct indexed PostgreSQL queries and measure latency, traffic, read frequency and database load. Add cache-aside for demonstrated repeated, staleness-tolerant reads (e.g. static hotel/Experience catalog), with bounded TTL, explicit invalidation and DB fallback. A Redis outage must not prevent an authoritative reservation/charge.
9. **Authoritative writes:** all room/seat allocation, booking confirmations, financial state, loyalty ledger posting and procurement commitments are verified and committed through PostgreSQL transactional logic. A stale cached search may display provisional availability; confirmation must re-check live data. Redis is never authoritative for business integrity.

## Options considered

- Keep Python/FastAPI: excellent testing, validation and migrations, but no decisive benefit that offsets the owner's Go preference; rewrite cost is still small.
- Use TypeScript/Node: also viable, good full-stack tooling, but the preference for a versioned compiled Go executable and module-level compile-time checks favors Go here.
- Build Redis now: possible performance optimization with immediate complexity for cache invalidation, consistency, configuration, error handling and additional deployment/runtime dependency, without observed demand.
- Design no caching accommodation at all: risks hard-wired read paths later; instead keep application query methods separable from transport and persistence so an optional read-through/cache-aside decorator can be introduced at a concrete bottleneck, **without** a generic cache abstraction today.

## Consequences

- Existing Python scaffold, migrations and tests will be **replaced**, not migrated line-for-line. This is a controlled, limited bootstrap rewrite; business requirements, legacy evidence and ADR-0002 remain unchanged.
- One PostgreSQL test environment has slightly more setup than SQLite but gives representative transactions and locking from the outset.
- Go requires more explicit database query and API validation code than FastAPI; normal compiler/test feedback may improve long-term maintainability.
- Single binary is simple to version and deploy, but still needs PostgreSQL, configuration and infrastructure.
- A cache can later wrap selected read/query boundaries. It does not belong inside core transaction logic.

## Validation / review trigger

Implementation is done when the clean checkout starts a Go service against Docker PostgreSQL, with real migrations, liveness/readiness tests, database integration, a version-identifiable build, required CI success, and no Python runtime dependency. Add no PMS business feature during the transition. Revisit Redis only with evidence of high-cost, high-volume repeated reads or a concrete additional requirement such as ephemeral rate limiting; write a focused ADR for its purpose, staleness budget, failure behavior and invalidation policy.
