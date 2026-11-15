# PMS nightly rates and inventory eligibility (Issue #23)

**Authority:** The owner [explicitly approved](https://github.com/chuxorg/chux-hosp-platform/issues/23#issuecomment-6894946587) the [first-slice proposal](https://github.com/chuxorg/chux-hosp-platform/issues/23#issuecomment-6894403882), including positive-only rates and configuration under inactive parents. **Trace:** LEG-AVL-001/002, LEG-VEN-002 → INF-AVL-001/002 → REQ-PMS-001 → **TST-PMS-001-02**. These are accepted greenfield rules, not reconstructed legacy pricing/status semantics. ADR-0002 module ownership and ADR-0003 Go/PostgreSQL remain unchanged.

## Accepted contract

- One current base nightly rate per Property-owned RoomType; setting it replaces the value. No seasonal/effective dates, history, per-room overrides, discounts or agent precedence. The current value applies to every valid queried interval.
- Money is an exact **positive int64 minor-unit amount**, 1–9,223,372,036,854,775,807. Currency is exactly `THB`; one baht is 100 satang. Reject zero, negatives, fractional/exponent/quoted numeric representations and overflow. No floating-point money, FX, rounding, tax/fee calculation or stay total. This is configuration, not a tax/invoice/final booking quote.
- Writes require explicit `active`; rate configuration and reads are allowed under inactive parents. Activation flags remain independent; no cascading activation/deactivation.
- Search requires Property, RoomType, physical Room and its rate to be active, with the same property/type ownership. Missing or inactive rates exclude rooms. It returns each physical room once, ordered by numeric room ID.
- Dates are strict valid Gregorian `YYYY-MM-DD` **hotel calendar labels**, interpreted using the Property's configured IANA zone. They are not instants. Check-in is inclusive, checkout exclusive; require checkout later than check-in. Reject impossible dates, zero-night/reversed intervals and timestamps. No server/client time-zone conversion, elapsed-hours division, past-date, lead-time or maximum-stay policy.
- **Results are configured inventory eligibility only.** `search_mode` is `inventory_eligibility`; `reservation_conflicts_checked` is always `false`. Candidate membership is date-independent for valid intervals because no reservation/hold/allocation model exists. Search creates no reservation, hold, lock or allocation and guarantees no future bookability. Issue #24 must define blocking states, reservation overlap and atomic allocation before those claims/tests are meaningful.

## ADR-0004 terminology and scope

Under [ADR-0004](adr/ADR-0004-configuration-driven-business-policies.md), this API returns **configured room candidates**. The existing `room-eligibility` endpoint and `inventory_eligibility` response label are preserved for compatibility; they do not establish commercial eligibility.

| Distinct concept | Current #23 scope |
| --- | --- |
| Configured room candidates | Active Property, RoomType, Room and current base rate; date labels validated |
| Commercial eligibility | Deferred: RatePlan/MarketSegment restrictions and minimum-stay policies are not evaluated |
| Date-specific sellable availability | Deferred: no allotments, reservations/holds, housekeeping/maintenance blocks or date-specific inventory accounting |
| Reservation confirmation | Deferred: no atomic booking confirmation or physical-room allocation |

[Discovery #39](https://github.com/chuxorg/chux-hosp-platform/issues/39) governs future hotel-policy semantics; [#24](https://github.com/chuxorg/chux-hosp-platform/issues/24) owns approved reservation/allocation behavior. Follow the [policy design checklist](POLICY_DESIGN_CHECKLIST.md) before adding hotel-specific variation. Neither ADR-0004 nor this synchronization adds a rules engine, policy-management framework, property-specific branches or new business functionality to #23. The accepted base-rate rules, API paths and response structures remain unchanged, including the development-only restrictions below.

## Development-only API

The same isolation requirements as [PMS configuration](PMS_CONFIGURATION.md) apply. **No authentication or tenant/role authorization exists.** Bind to localhost/isolated development networks and use synthetic data; do not expose management publicly.

| Method | Full path | Result |
| --- | --- | --- |
| PUT | `/pms/v1/properties/{propertyID}/room-types/{roomTypeID}/rate` | Complete set/replace of `{amount_minor,currency,active}`; 201 + Location on create, 200 on replace |
| GET | `/pms/v1/properties/{propertyID}/room-types/{roomTypeID}/rate` | Current configuration, including inactive rate or parents; 404 if no rate in that property scope |
| GET | `/pms/v1/properties/{propertyID}/room-eligibility?check_in=YYYY-MM-DD&check_out=YYYY-MM-DD` | Hotel zone/dates, explicit limitation and eligible physical rooms with current nightly rates |

PUT requires `Content-Type: application/json`; fields are typed and unknown fields/trailing JSON are rejected. IDs must be positive int64. Query parameters must contain exactly one `check_in` and one `check_out`; missing, duplicated, malformed or unsupported filters are rejected. No guest-count/occupancy filter is implied by the RoomType's existing `max_occupancy` field.

Error vocabulary follows #22: 400 invalid input, currency, amount, dates or IDs; 404 unknown read resource/property; 422 missing or cross-property RoomType on a write; 500 sanitized unexpected persistence error. Failed writes leave existing configuration unchanged. Existing inactive property or no matching inventory yields **200 with `rooms: []`**, not a validation failure. Search results are a read-time configuration view, not a reserved or version-locked quote.

Example after creating the property/type/rooms from [configuration setup](PMS_CONFIGURATION.md), substituting their actual IDs:

```bash
curl --fail-with-body -X PUT http://127.0.0.1:8000/pms/v1/properties/1/room-types/1/rate \
  -H 'Content-Type: application/json' \
  -d '{"amount_minor":12345,"currency":"THB","active":true}'
curl --fail-with-body http://127.0.0.1:8000/pms/v1/properties/1/room-types/1/rate
curl --fail-with-body 'http://127.0.0.1:8000/pms/v1/properties/1/room-eligibility?check_in=2026-11-15&check_out=2026-11-16'
```

Representative response (IDs depend on your fixtures):

```json
{
  "property_id": 1,
  "time_zone": "Asia/Bangkok",
  "check_in": "2026-11-15",
  "check_out": "2026-11-16",
  "search_mode": "inventory_eligibility",
  "reservation_conflicts_checked": false,
  "rooms": [{
    "room": {"id": 1, "property_id": 1, "room_type_id": 1, "number": "A101", "active": true},
    "nightly_rate": {"property_id": 1, "room_type_id": 1, "amount_minor": 12345, "currency": "THB", "active": true}
  }]
}
```

## Persistence, migration and boundaries

`migrations/00003_pms_room_type_rates.sql` adds exclusively PMS-owned `pms_room_type_rates`. RoomType ID is the primary key (one current rate), and the composite `(room_type_id, property_id)` FK enforces ownership against `pms_room_types`. Positive BIGINT amount, exact THB currency and non-null activation are database constraints as well as application validation. A transaction distinguishes create/replace and serializes concurrent writes via the unique key; no PostgreSQL tuple-internals trick or booking lock is used.

Apply with `go run ./cmd/hospitality migrate up` or `docker compose run --rm app migrate up`. Startup does not migrate. Rolling back migration 3 with `migrate down --allow-down` **deletes all rate configuration**, preserving Property/RoomType/Room tables and data. Reapplying creates an empty rate table; it does not resurrect removed rates. Use a backup/authorized forward repair for wanted data. Automated rollback tests run only in verified, uniquely named disposable child databases; no developer or production database is reset.

Domain owns the rate/interval/result types; application validates the approved use cases and extends the existing typed Store port; infrastructure owns parameterized PostgreSQL queries/transaction; API parses requests and sanitizes errors. No cross-module contracts are consumed or provided by this slice. The composition root mounts PMS alone; Experiences, Procurement, Operations and Loyalty remain absent. No cache or additional infrastructure.

## Acceptance evidence / commands

```bash
./scripts/validate.sh
go test -race ./...
# With a separate *_test provisioning database; see LOCAL_SETUP.md:
./scripts/test-integration.sh
```

TST-PMS-001-02 coverage:

- `domain.TestStayInterval`: adjacent/next-day, leap and DST-spanning calendar labels, past/long intervals, impossible/malformed dates, timestamps, zero/reversed intervals.
- `application.TestRateService` / `TestEligibilityService`: exact money, inactive-parent preparation, set/read/replace, ownership/error propagation, explicit metadata and non-null empty results across hotel zones.
- `api.TestRateHTTPValidationAndErrors` / `TestEligibilityHTTPRejectsAmbiguousQuery`: typed JSON representations, int64 boundary, create/replace statuses, unsupported currency, explicit activation, query ambiguity and sanitized errors.
- `infrastructure.TestRatesAndEligibilityAgainstPostgres`: real HTTP/SQL roundtrips, maximum int64, invalid-change atomicity, unknown/cross-property references, all activation levels, missing rates, multiple physical rooms, property isolation, hotel zones/DST, date-independent output, direct database constraint rejection, concurrent complete replacement/one rate, and up/down/reapply preserving earlier records.
- Existing PMS-only composition, import and SQL ownership checks, platform PostgreSQL, configuration CRUD/concurrency tests and required `bootstrap-validation` remain mandatory. The existing configuration migration rollback test now removes dependent migrations before asserting its own rollback; no original assertion is removed.

**Explicitly deferred to #24:** reservation overlap/hold/blocking semantics, no-overbooking assertions and concurrent physical-room allocation tests. Also excluded: housekeeping/maintenance states, taxes, invoices, historical price snapshots and production authorization. No fabricated booking structures are introduced to satisfy coverage.
