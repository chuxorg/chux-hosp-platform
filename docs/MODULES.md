# Module boundaries and Go composition (Issue #20)

**Status:** Issue #20 module boundary established; Issue #22 adds the first PMS configuration-only business slice. Issue #23 adds current nightly rates and explicitly limited inventory eligibility; no booking, guest, folio or production authorization features. See [ADR-0002](adr/ADR-0002-modular-hospitality-platform.md) for domain ownership, [ADR-0003](adr/ADR-0003-go-postgres-defer-redis.md) for the Go/PostgreSQL technology decision, and the [approved Dropbox scope](https://www.dropbox.com/scl/fi/zu6lowh2qhiumdxenbbvy/03-Approved-Greenfield-Specification-REQ.paper?rlkey=0ue4d29y20azrkjs0yg4ai0cu&dl=0).

## Composition and executable boundaries

- `cmd/hospitality/composition.go` is the **application composition root**. It mounts the existing Platform HTTP probes, then the PMS-owned handler at `/pms/v1/`. The composition root may know both; the Platform package cannot import business modules.
- `internal/modules/pms/api/` owns its HTTP handler. Its initial Issue #20 endpoint was **`GET /pms/v1/status`**, returning `{"module":"pms","status":"bootstrap"}`. This is a diagnostic proving a mounted PMS-only module, **not** a usable PMS, hotel operational readiness, or a public business operation. Issue #22 adds Property, RoomType and Room endpoints documented in [PMS_CONFIGURATION.md](PMS_CONFIGURATION.md); the status diagnostic remains unchanged. Issue #23 adds the [PMS rate and inventory-eligibility API](PMS_RATES_AND_ELIGIBILITY.md), with no dependency on optional modules and no reservation-aware availability claim.
- Existing `GET /healthz` and `GET /readyz` remain Platform-owned, with the same PostgreSQL connectivity semantics. There is still **one Go executable and one PostgreSQL database**.
- Optional Experiences, Loyalty, Procurement and Operations modules are not imported, instantiated, or required. No dynamic module loader or feature-flag framework is implemented. When they are introduced, composition stays explicit.

## Dependency rules, automatically checked

`internal/architecture/boundaries_test.go` parses the **production Go import declarations** under `internal/` and fails `go test ./...` on violations. It verifies:

1. `internal/platform/` cannot import business modules.
2. PMS cannot import optional modules, including their contracts; optional modules can consume **another module's explicitly public `contracts/` package** where a real business use case requires it.
3. No module may import `cmd/hospitality`; this prevents a backwards dependency on executable wiring.
4. A module's `domain/` cannot import its `application/`, `infrastructure/`, or `api/`. Its `application/` cannot import concrete infrastructure/API; its `api/` cannot import infrastructure directly. The composition root connects concrete adapters later.
5. PostgreSQL drivers are confined to a module's `infrastructure/` (or Platform bootstrap), not the domain, application or API.
6. The test includes deliberately forbidden import examples to prove the checker is capable of rejecting coupling. It also fails if the initial PMS production source disappears, avoiding a vacuous pass.

Import analysis is not a complete SQL-access analyzer. Issue #22 adds `TestModuleSQLTableOwnership`, which inspects literal SQL in module infrastructure and rejects direct queries to another module's tables. This is a **limited static check**, not a general SQL parser: migrations and real PostgreSQL tests additionally verify the new `pms_*` tables, composite same-property FK, and atomic constraints. All later modules must likewise own their tables and avoid direct SQL into other modules. PostgreSQL remains authoritative for room allocation, transactions, and future pricing/financial state.

## Typed contracts and fake-provider tests — when needed

Do not introduce a speculative `Service`, `Module`, `EventBus`, `Repository` or generic port interface now.

When an approved application use case needs synchronous cross-module cooperation, define **a small consumer-owned Go interface** with only the required typed methods; expose immutable DTO types in the provider's `contracts/` package when appropriate. Compose the real provider in `cmd/hospitality/` and test a substitute fake with the same interface. Use typed in-process events for notification only when a concrete story needs them. In-memory events do not guarantee durable delivery.

Issue #22's PMS configuration use cases now justify a **typed application Store port** and a fake-adapter unit test, plus a real PostgreSQL implementation. This port is internal to PMS; do not confuse it with a speculative cross-module provider or event framework.

## How to verify

```bash
./scripts/validate.sh
go test ./internal/architecture ./internal/modules/pms/api ./cmd/hospitality
# For the existing full PostgreSQL tests:
TEST_DATABASE_URL='postgres://.../hospitality_test?sslmode=disable' ./scripts/test-integration.sh
```

- **TST-PLT-005-01:** `TestPMSOnlyComposition` checks existing platform liveness, the PMS diagnostic, 404 for absent optional modules, and method/path isolation.
- **TST-PLT-005-02:** `TestProductionModuleImportBoundaries` and `TestImportPolicyRejectsForbiddenDependencies` check compile-time import boundaries and negative cases.
- All pre-existing PostgreSQL tests and `bootstrap-validation` checks continue unchanged.

Issue #22's proposed Property/RoomType/Room fields and constraints are documented in [PMS_CONFIGURATION.md](PMS_CONFIGURATION.md) and require owner review before merge. Tenant authorization, guest PII, booking states and a universal Facility/Reservation model remain out of scope.
