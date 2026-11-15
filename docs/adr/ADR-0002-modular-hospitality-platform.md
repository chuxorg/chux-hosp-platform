# ADR-0002: Modular Hospitality Platform and in-process module contracts

Status: **Accepted as architectural direction by the owner on 2026-11-14; implementation in this PR pending owner-reviewed merge.**
Date: 2026-11-14
Owner: Hospitality project owner
Links: Issue #15; ADR-0001; [approved business scope](https://www.dropbox.com/scl/fi/zu6lowh2qhiumdxenbbvy/03-Approved-Greenfield-Specification-REQ.paper?rlkey=0ue4d29y20azrkjs0yg4ai0cu&dl=0); [module boundaries](https://www.dropbox.com/scl/fi/4ia47cl33y9ibtighkm3j/Hospitality-Modular-Platform-Module-Boundaries-and-Integration-Contracts.paper?rlkey=x5zlc6w9s6u2w9yqh94yye8yk&dl=0); [legacy capability catalog](https://www.dropbox.com/scl/fi/nbk31b6fa9sv347xf4iog/Legacy-PMS-Capability-Catalog-and-Proposed-Module-Ownership.paper?rlkey=xycfneji3bchtiy658rlsf8zo&dl=0).

## Context

Hospitality is **not a PMS with optional extras**. It is one platform made of separately useful PMS, Experiences, Loyalty, Procurement, and Operations modules. The archived PHP/MyISAM PMS is evidence of business capabilities (room inventory, stays, pricing, CRM, financial close, F&B, suppliers, stock, maintenance); its physical tables and class boundaries are not a specification of the new module boundaries. The two legacy booking structures remain an unresolved consultant question.

We need maintainable modules and future integration choices without early distributed-system, workflow-engine, generalized ERP, or plugin-loader overhead. ADR-0001 selects Python 3.13, FastAPI, SQLAlchemy/Alembic, SQLite locally and PostgreSQL for deployment; this ADR does not change that technology selection.

## Decision

1. **Modular monolith**: one FastAPI application process and one relational database initially, not microservices. A module is a business capability boundary, not a directory naming convention.
2. **Module ownership**:
   - **PMS**: hotel/property configuration, rooms/types/rates, stay availability, stay reservations/room allocations, guest-stay operations, check-in/out, accommodation folios and applicable agency rates. PMS runs without Experiences/Loyalty.
   - **Experiences**: offerings, scheduled sessions, experience bookings/capacity, provider/participant coordination. Must run without PMS; Thai Cooking is the first experience *after a usable PMS vertical slice*.
   - **Loyalty**: enrollment, points/reward rules and ledger. Integrates with completed stays/experiences via stable contracts/events; neither source module requires Loyalty.
   - **Procurement**: shared suppliers, sourcing requests, purchasing/receipt statuses. Begin with an ingredient-request-to-receipt slice for Experiences; no full ERP.
   - **Operations**: reusable work orders and task assignments when the second concrete cross-module use case requires it (e.g. maintenance plus Experience fulfillment). PMS retains hotel-specific occupancy/maintenance consequences.
   - **Platform**: minimal composition, auth/identity/authorization infrastructure as approved, shared stable references, contract and event interfaces, auditing infrastructure. No universal booking, customer, price, financial or inventory aggregate.
3. **Vertical integration rules**: each module owns its domain and persistence. Other modules may not directly import internal models/repositories or query/update its owned tables. Public commands/queries return immutable typed DTOs; no cross-module ORM objects. A composition root wires ports, handlers and optional adapters.
4. **Events vs commands**: direct synchronous ports/interfaces for queries or actions requiring immediate response; typed in-process events (with identifiers/versioning and idempotent handlers where needed) for notifications such as `StayCompleted.v1`, `ExperienceCompleted.v1`, `SupplyRequestCreated.v1`. In-memory dispatch is **not durable**. Add transactional outbox/other reliability when an accepted business requirement makes a critical side effect dependent on delivery. No broker now.
5. **One database, owned schema**: begin with module-specific naming prefixes/table ownership and migration discipline compatible with SQLite and PostgreSQL; no cross-module SQL reads/joins/writes. Cross-module orchestration must respect transaction boundaries, errors, optional module absence and replay rules. Tests verify isolation on PostgreSQL where concurrency is material.
6. **Optional modules**: PMS-only deployment is valid; Experiences-only deployment (with minimal Platform) is valid once implemented; absent Loyalty or Procurement is not an import/startup failure. Avoid empty scaffold generation or dynamic loading frameworks.
7. **Consistent internal structure**: when code exists, each module may grow `domain/`, `application/`, `infrastructure/`, `api/`; place public contracts at module boundaries and reusable infrastructure in `platform/`. Do **not** create all folders or abstractions before a story requires them.
8. **Test expectations**: module unit/integration tests, API and migration tests, contract tests for synchronous calls/events, architecture-boundary checks, and targeted module-combination startup tests. Use synthetic records; keep source archives and guest data private.
9. **Build order**: first a working standalone PMS property → room/rate → availability → guest/stay → check-in/out → simple folio slice, then Experiences framework/Thai Cooking, then reusable Procurement/Operations needs and Loyalty. PMS discovery continues beyond the MVP.

## Illustrative structure (not a scaffold requirement)

```text
src/hospitality/
  platform/
    composition/
    contracts/
    identity/
    events/
  modules/
    pms/
      domain/
      application/
      infrastructure/
      api/
    experiences/    # same four layers when introduced
    loyalty/        # same four layers when introduced
    procurement/    # same four layers when introduced
    operations/     # same four layers when introduced
```

## Alternatives considered

- **Monolithic PMS including cooking, supplier and loyalty behavior**: fastest one-off but recreates legacy coupling and blocks hotels with different PMS.
- **Universal customer/reservation/payment/inventory platform aggregates**: looks reusable but forces incompatible invariants and causes cross-module ownership confusion.
- **Microservices/event broker/workflow engine/general ERP from the start**: introduces deployment, consistency and security overhead before product demand proves it necessary.
- **Modular monolith (selected)**: explicit ownership now; distribution later only with real scaling/organizational/deployment pressure.

## Consequences and limitations

- More deliberate contract and data-ownership work than a feature-first app; typed DTOs may duplicate a few read fields intentionally.
- One process/database does not make modules independently deployable today. Shared transactions are possible but should not become implicit cross-module writes.
- In-process events can be lost on crashes and need careful transactional ordering. Critical payment, inventory or loyalty effects must not rely on best-effort dispatch.
- Module enablement and identity/tenant policy need explicit story-level rules; no global identity implementation is approved merely by this ADR.
- Legacy F&B, procurement, stock, maintenance, agents and reporting must be classified through the living capability catalog; owning module assignment is a greenfield proposal, not a retroactive legacy fact.

## Validation and review triggers

Architecture validation begins with a small PMS-only slice, one import-boundary check and composition/contracts tests. Verify Experiences-only startup when Experiences exists. Revisit this ADR if independently deployed modules, third-party plugins, complex fulfillment or a durable event requirement arises. New physical schema fields and business rules still require owner-reviewed REQ/story acceptance, not just this ADR.
