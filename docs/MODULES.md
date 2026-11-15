# Module boundaries and Go composition (Issue #20)

**Status:** implemented architecture slice; no PMS business functionality. See [ADR-0002](adr/ADR-0002-modular-hospitality-platform.md) for domain ownership, [ADR-0003](adr/ADR-0003-go-postgres-defer-redis.md) for the Go/PostgreSQL technology decision, and the [approved Dropbox scope](https://www.dropbox.com/scl/fi/zu6lowh2qhiumdxenbbvy/03-Approved-Greenfield-Specification-REQ.paper?rlkey=0ue4d29y20azrkjs0yg4ai0cu&dl=0).

## Composition and executable boundaries

- `cmd/hospitality/composition.go` is the **application composition root**. It mounts the existing Platform HTTP probes, then the PMS-owned handler at `/pms/v1/`. The composition root may know both; the Platform package cannot import business modules.
- `internal/modules/pms/api/` owns its HTTP handler. Its only endpoint for Issue #20 is **`GET /pms/v1/status`**, returning `{"module":"pms","status":"bootstrap"}`. This is a diagnostic proving a mounted PMS-only module, **not** a usable PMS, hotel operational readiness, or a public business operation. New domain-specific endpoints arrive in approved PMS stories (starting #22).
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

This is **not** a complete SQL-access analyzer. No PMS tables or queries exist yet. When #22 introduces PMS persistence, add migration/SQL ownership checks and integration tests ensuring `pms_*`-owned tables and no cross-module table reads/writes. PostgreSQL remains authoritative for room allocation, transactions, and future pricing/financial state.

## Typed contracts and fake-provider tests — when needed

Do not introduce a speculative `Service`, `Module`, `EventBus`, `Repository` or generic port interface now.

When an approved application use case needs synchronous cross-module cooperation, define **a small consumer-owned Go interface** with only the required typed methods; expose immutable DTO types in the provider's `contracts/` package when appropriate. Compose the real provider in `cmd/hospitality/` and test a substitute fake with the same interface. Use typed in-process events for notification only when a concrete story needs them. In-memory events do not guarantee durable delivery.

This deliberately defers TST-PLT-005-02's future real provider/fake integration test: the present architecture test checks that contracts can be introduced without dependency violations. Implementing a fake for a nonexistent PMS business command would be artificial scaffolding.

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

This issue does **not** approve property/room fields, tenant authorization, guest PII, booking states, or a universal Facility/Reservation model. Those belong to the PMS requirements spike (#21) and first functional story (#22).
