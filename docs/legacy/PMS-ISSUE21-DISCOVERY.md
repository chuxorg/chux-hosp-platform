# Issue #21: PMS legacy evidence, first-slice priorities and decision register

**Analysis date:** 2026-11-15. **State:** discovery/proposal pending product-owner review; no legacy operations asserted active.
**Scope:** Source inspected read-only from private `PMS.zip`, cross-checked with existing Dropbox LEG/INF/REQ registers. Original code/data must never enter this repository.
**Decision authority:** [Approved greenfield specification](https://www.dropbox.com/scl/fi/zu6lowh2qhiumdxenbbvy/03-Approved-Greenfield-Specification-REQ.paper?rlkey=0ue4d29y20azrkjs0yg4ai0cu&dl=0) · [Legacy evidence](https://www.dropbox.com/scl/fi/y7ftwjaq8mtjwptgdn910/Legacy-PMS-Domain-Relationships-and-Workflow-Traces.paper?rlkey=88ezq8p0ts7p6o06mcr7592ak&dl=0) · [Architecture](https://www.dropbox.com/scl/fi/4ia47cl33y9ibtighkm3j/Hospitality-Modular-Platform-Module-Boundaries-and-Integration-Contracts.paper?rlkey=x5zlc6w9s6u2w9yqh94yye8yk&dl=0).
**Accepted high-level direction:** REQ-PMS-001 standalone PMS first; REQ-PLT-005/006 module ownership; Go/PostgreSQL per ADR-0003; no other modules required. Specific business fields/states remain separate owner decisions.

## 1. Observed legacy evidence (LEG; not an implementation mandate)

| Evidence | Exact legacy PHP source inspected | What source actually shows |
| --- | --- | --- |
| LEG-AVL-001 — categories | `PMS.zip:admin/roomtype.php:L44-60,L97-112,L154-181`; `admin/roomtype_add.php:L11-36`; `admin/roomtype_edit.php:L18-22` | A `room_cate` record has a name and price/VAT-related field; edit page also references `typetxt` (add page does not visibly insert `typetxt`). No reliable occupancy capacity semantics from these fields. |
| LEG-AVL-002 — room-ish records | `admin/setrate_hotelroom.php:L45-74,L81-99,L116-149,L201-245,L352-398`; `admin/setrate_hotelroom_add.php:L11-85`; `admin/setrate_hotelroom_edit.php:L18-20` | `room_rate` rows link `roomcate` to `room_cate`, carry `name`, `hotelID`, `min_price`, `allotroom`, `roomstatus`, `roombed`. This mixes room catalog and commercial attributes. Three hotel selections are hard-coded as numbered labels, **not a separately observed property entity**. |
| LEG-AVL-002 — operational identity | `admin/booking_step1.php:L183-222`, `admin/booking_checkrun.php:L73-92`, `admin/booking_step1add.php:L38-49` | `sub_booking.roomID` refers to `room_rate.ID`; allocation detail is per night, and there is an overlap/status query. Real physical-room vs allotment meaning and status-code semantics still need confirmation. |
| LEG-RES-002/003 — two booking families | `admin/booking_checkrun.php:L64-92`; `admin/booking_rsvnadd.php:L132-212` | `main_booking/sub_booking` and `es_bookingrsvn/es_bookingrsvn_sub` are distinct write paths. Which is operational, historical, or agent-only **is unknown**. |
| LEG-RES-002 — check-in/out screens | `admin/booking_chkin.php:L30-75`, `admin/booking_checkout.php:L247-305`; `admin/room_updaterun.php:L11-24` | UI filters booking status values `11`/`4`, checkout listing displays a status `5`, and room status can be changed. **Not enough** to assign canonical meanings or transition rules. |
| LEG-GUE-002 — guest linkage | `admin/customer_run.php:L54-62`; `admin/crm_run.php:L3-9,L40-75` | Bookings can link to a customer and copy some contact fields. No consent/retention or minimal PII standard determined. |
| LEG-FIN-002 — operational closing | `admin/booking_closeroundrun.php:L44-89` | Close-round groups bookings, deposits and some F&B/charges; no verified VAT, payment/reconciliation business policy. |

**Confidence:** High for file/SQL constructs; low for whether an installation still uses them or whether statuses/prices behave as intended. Do not reproduce hard-coded hotel IDs, `room_rate` as both room and price, MyISAM constraints, raw customer data, or PHP implementation.

## 2. Recommended first-usable PMS order (proposal, not owner-signed detail)

| Priority | Story | Intended slice | What remains unresolved |
| --- | --- | --- | --- |
| P0 | [#22](https://github.com/chuxorg/chux-hosp-platform/issues/22) | Property → room type → named physical room; isolated PostgreSQL tables; create/read/update | Required fields, whether types are per-property, occupancy definition, status semantics, security |
| P1 | [#23](https://github.com/chuxorg/chux-hosp-platform/issues/23) | One simple rate and date-range availability; price as snapshot later | Daily/category/agent rate precedence, tax/VAT, what statuses block |
| P1 | [#24](https://github.com/chuxorg/chux-hosp-platform/issues/24) | Synthetic guest + StayReservation + atomic physical-room allocation; PostgreSQL concurrency | Two booking families, holds/confirmed/cancelled states, guest privacy |
| P1 | [#25](https://github.com/chuxorg/chux-hosp-platform/issues/25) | Approved check-in/out transitions and auditable stay status | Full transition diagram, early checkin/late departure |
| P1 | [#26](https://github.com/chuxorg/chux-hosp-platform/issues/26) | Basic stay folio and explicitly simulated charge | Tax, deposits, refunds, checkout/closure obligations |
| Later PMS | Agents / negotiated rates / reporting / CRM | Legacy-informed capabilities only when accepted | Hotel usage, priority and security model |
| Later shared modules | Procurement, Operations, Experiences, Loyalty | Business ownership from ADR-0002 | Integrate via actual contracts only, no premature framework |

## 3. Proposed Issue #22 minimal data contract — **owner review required**

Scope: one Property, many RoomTypes, many individual Rooms. These are PMS-owned, **not** platform-shared `Facility` objects. An Experience must be able to function without PMS.

- **Property:** generated stable ID, `name` (non-empty), `time_zone` (IANA identifier, e.g. `Asia/Bangkok`), `active` boolean. `time_zone` is a **new operational requirement**, not a field demonstrated by legacy PHP.
- **RoomType:** generated stable ID, `property_id`, `name`, `max_occupancy` (> 0), `active` boolean. `max_occupancy` is a greenfield limit for later bookings, **not a verified translation** of `room_cate.typetxt`.
- **Room:** generated stable ID, `property_id`, `room_type_id`, `number`/display identifier, `active` boolean. Treat as a **physical unit**, not pooled allotment or rate record; name `number` can be alphanumeric.
- **Invariants (proposed):** room numbers unique per property, case-insensitively; room-type names unique per property; room's RoomType must belong to the same property; invalid/missing parents rejected atomically; nonpositive occupancy rejected; soft activation toggle rather than data deletion.
- **Persistence:** `pms_properties`, `pms_room_types`, `pms_rooms` in PostgreSQL with owned versioned migrations; database constraints enforce associations/uniqueness across concurrent writes.
- **HTTP:** small versioned PMS-only JSON endpoints under `/pms/v1/` for POST/GET/PUT; no list, search, rates or booking in #22. Explicitly marked **development-only** until authorization/tenant policies are approved, with Compose localhost binding.
- **Testing:** TST-PMS-001-01 for create/get/update; duplicate room number, unknown room type, cross-property mismatch, bad time zone, nonpositive occupancy and unchanged persisted state on failure; PostgreSQL migrations and HTTP tests. PMS-only startup and module-boundary tests remain green.

**Tradeoff:** generated numeric IDs are acceptable stable keys in this local prototype, but all future access must be scoped and authorized before exposing across tenants. Enforce same-property room-type relationship via a composite FK, not only service logic.

These choices are explicitly **provisional greenfield specifications** for owner review in Issue #22. The owner's instruction to proceed authorizes development of a reviewable vertical slice; it does not validate as facts any undocumented legacy behavior. The PR must not be merged until this scope is accepted.

## 4. Open decisions for consultant/product owner (must not be silently filled)

1. Are hotel IDs 1/2/3 in `setrate_hotelroom.php` still used across actual properties? Are room types shared across hotels or configured separately?
2. Does a `room_rate` row represent one physical unit or a type/allotment with quantity? Is `allotroom` an operational allocation cap, inventory quantity, or something else?
3. Which of the two booking structures is live, and are they separate guest/agent processes?
4. What states consume room inventory (hold/confirmed/checked-in), and when is inventory released on cancellation/checkout? Do **not** map raw legacy values `1/7/11/4/5` to greenfield enums without confirmation.
5. Which roles can create/disable rooms, and who can see which hotel? **No production exposure or real guest data until authorization is defined.**
6. Define pricing incl. VAT, deposits, currency, checkout requirements and agent-rate precedence only for stories #23–26.
7. What is the minimum guest personal data and retention/consent policy for #24?
8. Which source-supported supporting functions are truly used now (F&B, stock, maintenance, agent accounting, CRM)? Owner priorities needed before second PMS tranche.

## 5. Accepted vs proposed change control

- **Accepted:** modular Hospitality Platform, standalone PMS first, Go, PostgreSQL, source evidence vs requirements distinction, no Redis/broker.
- **Proposed / pending scope review:** individual #22 fields/constraints/API and #23–26 lifecycle and finance rules.
- **Not implemented:** any reservation state machine, room availability policy, guest profile, rate, folio, loyalty or supplier system.
- **Issue #21 exit:** produce this evidence and open-decision record; obtain owner acceptance or document which decisions are deferred. #22 may proceed as a reviewable isolated, synthetic-data-only prototype without asserting rejected/unknown business rules.

No secret, real hotel records, customer attributes or database archive content is included.
