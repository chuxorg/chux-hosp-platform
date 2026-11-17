# PMS pooled reservations — Issue #24

REQ-PMS-002 / TST-PMS-001-03/04, owner-approved 2026-11-16:
[approved requirements](https://www.dropbox.com/scl/fi/zu6lowh2qhiumdxenbbvy/03-Approved-Greenfield-Specification-REQ.paper?dl=0),
[accepted tests](https://www.dropbox.com/scl/fi/4ds7lh5avgridmjx8gz2x/Hospitality-Pilot-Rules-Acceptance-and-Traceability.paper?dl=0),
[design](https://github.com/chuxorg/chux-hosp-platform/issues/24#issuecomment-6906821233),
[review safeguards](https://github.com/chuxorg/chux-hosp-platform/issues/24#issuecomment-6907519749).
ADR-0002/0003/0004 govern ownership, Go/PostgreSQL, and invariant versus policy separation.

Development-only, synthetic data. No property-scoped authentication exists yet.
Do not expose these APIs publicly or use real guests/inventory. Pool attestation
is a bounded operator reference, not verified suitability, regulatory compliance,
or authenticated approval. Restricted, heterogeneous, encumbered, or segmented
inventory is outside this slice. Existing catalog rows are **unenrolled**.

## HTTP contract

All paths below are relative to `/pms/v1`. JSON mutations reject unknown fields,
trailing JSON, non-integer money and bodies over 16 KiB. IDs must be positive.
Synthetic references use 1–80 ASCII letters/digits plus `._:-`, starting with a
letter/digit; surrounding whitespace is normalized. Never put PII in references.

- `PUT /properties/{p}/room-types/{t}/reservation-pool`, then `GET` the same path.
  Body: `{"scope":"homogeneous_segment_neutral","enabled":true,"attestation_ref":"SYN-NEUTRAL-001","operator_ref":"SYN-OP-001"}`.
  All fields required (including explicit enabled). Scope is fixed. No capacity
  input: capacity derives from active physical Rooms of this owned RoomType.
  Revision and timestamp are server generated. Registration may be configured
  under inactive parents; confirmation requires active property/type/base rate.
  Enabling requires at least one active backing room. An absent pool returns 404
  on read and prevents confirmation. Closing sales preserves all commitments.
- `POST /properties/{p}/stay-reservations`. Required single `Idempotency-Key`
  header: UUID-form string; case is normalized. Body:
  `{"room_type_id":12,"guest_ref":"SYN-GUEST-001","check_in":"2027-01-01","check_out":"2027-01-03","expected_nightly_rate_minor":12500,"currency":"THB","operator_ref":"SYN-OP-001"}`.
  Creates **one pooled unit per local date**, excluding checkout, confirmed
  immediately. No Room ID/physical assignment. Returns 201 + Location for first
  commit; same property/key/normalized command returns 200 and
  `Idempotency-Replayed: true`, same ID, original terms and current status. The
  command fingerprint covers every input, including expected price/operator.
  Changing any semantic input with the same key conflicts. Rolled-back failures
  retain no key. Replay precedes current price, active flags, pool and local-date
  checks, so lost responses remain recoverable after configuration changes.
- `GET /properties/{p}/stay-reservations/{id}`: property-scoped current status and immutable booking terms;
  wrong property/unknown ID returns 404.

Booking terms version 1: positive current nightly THB minor amount, calendar
night count, checked int64 `accommodation_base_subtotal_minor`. This is an
accommodation **base subtotal**, without tax, fees, payment or final payable total.
The response preserves property time zone, pool revision/attestation, originating
synthetic operator, capacity at confirmation and database confirmation time.

Errors: 400 malformed/unsupported inputs or overflowing subtotal; 404 missing
lookup; 422 invalid/cross-property references, past local dates or technical night
limit; 409 inactive inventory, price changed, exhausted nights, conflicting key,
or protected catalog edits; 412 absent/closed pool; 503 bounded transaction/lock
retry exhaustion (`Retry-After: 1`). Unexpected persistence details are sanitized.
A 503/transport failure is not proof that nothing committed: retry the **same key
and payload**, never generate a replacement key to recover a lost response.

## Transaction and database decisions

PMS owns the typed `ReservationStore` port and all three new tables; composition
wires the PostgreSQL adapter. No optional business modules, cache, network service,
generic UnitOfWork or policy evaluator is required. Existing #23 configured
`room-eligibility` behavior is unchanged; it still does not check reservations.

At READ COMMITTED: lock property FOR SHARE, owned RoomType FOR UPDATE, then run a
**separate fresh statement** to read the key/configuration and every requested
night's demand. This serializes same-type confirmations, including an empty
ledger, while allowing different types to proceed. Capacity is active Room count,
never max occupancy. Insert immutable reservation and exactly one commitment per
night together. `(property_id,idempotency_key)` uniqueness also serializes keys
racing across different types. `ON CONFLICT DO NOTHING` followed by a fresh read
observes the winner. A request with any full night rolls back completely.

Existing configuration mutations acquire property FOR UPDATE first, then types
in ascending ID order, then child rows. Rate edits therefore yield coherent old
or new snapshots. Active stock membership, deactivation/reassignment, and type
occupancy changes are frozen while enrolled **or** while today/future confirmed
nights remain. This includes stock increases, which need closed, unencumbered
inventory and reattestation. Cosmetic names/room numbers and property/type sales
closure remain editable. Time zone becomes immutable after the first booking.
These conservative restrictions are temporary safety scaffolding, not commercial
hotel policies or a maintenance lifecycle.

Migration 00004 adds `pms_reservation_pools`, `pms_stay_reservations`, and
`pms_reservation_inventory_commitments`. Composite FKs preserve ownership. Fixed
scope, bounded references, exact numeric arithmetic, unique nights, immutable
history triggers and deferred completeness/capacity triggers are database
backstops. VOLATILE trigger queries take fresh snapshots after locks. Raw SQL
writers receive the same protections; affected writes reject transaction isolation
other than READ COMMITTED so a caller cannot bypass fresh-snapshot enforcement; conflicting lock inversion can abort a raw
transaction and must not be blindly retried statement by statement. TRUNCATE on
history/backing catalogs is rejected. Database owners/superusers can disable
triggers; production runtime privilege separation is a later deployment concern,
not a claim that this development role can resist a malicious DB owner.

Confirmation retries the entire command at most three times for serialization
failure, deadlock or lock timeout, with 20/40 ms backoff. Per-lock wait is 1500 ms;
the application confirmation deadline is 10 seconds. Unknown COMMIT outcomes are
looked up through a fresh pool checkout by original key; if absent, only that
same command may be retried. Unresolved outcomes return 503, never guessed success.

## Technical bounds and migration evidence

366 expanded nights bounds each request to at most 366 ledger inserts and 366
night groups. Deferred guards also recheck bounded coverage/capacity; this favors
simple integrity over booking throughput. The 10-second deadline and 1500-ms lock
wait limit request/connection occupancy, including three attempts. These are
initial **technical processing budgets**, not a maximum hotel stay, lead time,
minimum stay or the unresolved “15 room-nights” policy. The integration boundary
test records the 366-night transaction time; production capacity/throughput still
requires measurement before relaxing bounds or changing serialization.

Run `FULL_VALIDATION=1 TEST_DATABASE_URL=... ./scripts/validate.sh` using only a
separate `*_test` provisioning database. Tests create cryptographically unique
child databases and verify `current_database()` before any DDL/DML; cleanup drops
only those children. The suite proves empty 4→3→4 migration, existing rate rollback
through 3→2 and reapply, full older-schema regression, and populated-history down
refusal without row loss. Down obtains exclusive table locks before inspecting
history, preventing concurrent check/drop data loss. Do not down/reset a populated
application database; use reviewed forward repair or an explicitly authorized
backup/restoration procedure. No existing catalog is auto-enrolled by migration.

Tests map REQ-PMS-002/TST-PMS-001-03/04 to API/typed-port validation, last-unit and
N-unit races, same/different-type idempotency, replay after price/closure changes,
full-interior-night rollback, adjacent reuse, raw SQL partial/duplicate/out-of-range
and cross-property guards, catalog races/freeze/cosmetic edits, local calendars,
retry/commit recovery and real bounded lock contention. Commit-acknowledgment loss
is deterministically fault-injected around a real PostgreSQL commit; SQLSTATE retry
faults are injected at the transaction-attempt boundary. This does not emulate
all operating-system/network partitions. Physical assignment tests remain deferred.

Extensions, room assignment, RatePlans, market allotments,
manager-approved transfers, real guest profiles, tax/folio/payments, operational
policy UI or generic policy framework is implemented. Future authorized lifecycle
changes must preserve this append-only inventory/price evidence and current stock.


## Pre-arrival cancellation — Issue #43

[REQ-PMS-003, specification revision 12](https://www.dropbox.com/scl/fi/zu6lowh2qhiumdxenbbvy/03-Approved-Greenfield-Specification-REQ.paper?dl=0) and
[TST-PMS-001-07, catalog revision 9](https://www.dropbox.com/scl/fi/4ds7lh5avgridmjx8gz2x/Hospitality-Pilot-Rules-Acceptance-and-Traceability.paper?dl=0)
cover the development-only synthetic cohort.

- POST `/properties/{p}/stay-reservations/{id}/cancellation` with one UUID
  `Idempotency-Key` and `{"operator_ref":"SYN-OP-001","reason_ref":"SYN-REASON-001"}`.
  Both references follow the existing synthetic-reference bounds. The first
  committed full cancellation returns 201 and Location. An identical command
  replays the original event with 200, even after cutoff. A changed command
  under the same property key, a different key on an already cancelled stay,
  or a new request on or after the local check-in date returns 409.
- GET the same resource for the original immutable cancellation event.
  A missing or foreign-property reservation or a missing event returns 404.
  Reservation GET and original confirmation replay return current status and
  the original accepted terms. Cancellation and confirmation keys are independent.
- The primary PostgreSQL clock supplies the decision timestamp after Property
  SHARE, RoomType UPDATE and Reservation UPDATE locks. The original booking
  time zone determines the local date. Inactive sales configuration and pool
  closure still permit a valid pre-arrival cancellation. Operator and reason
  references are synthetic evidence. The operation has zero financial or
  physical-room-assignment side effects.

Migration 00005 adds the immutable singleton event and derives current status
and consuming nights from it. Original booking rows and nightly commitments
remain the historical ledger. Capacity and the backing-stock freeze use
consuming commitments. Enabled pools still freeze backing stock, and the
property time zone remains frozen after any booking.

Drain all old application instances, apply 00005, and start the updated
application before exposing cancellation. Old instances read historical demand
and historical status, so a mixed-version rollout is unsafe. Before the first
cancellation, an authorized rollback may run 00005 down under exclusive locks
and then restart the old binary. Once any cancellation exists, downgrade
refuses even when its nights have been resold. Plan for forward repair from
that point. Keep these endpoints on the trusted synthetic demo environment.

The cancellation PostgreSQL integration suite traces REQ-PMS-003 and cases
07-A through 07-M to evidence/resale, key races, lock ordering, commit recovery,
raw SQL, cutoff, API and migration tests. Case 07-N is covered by the existing
confirmation regression suites, the full validator and the four quality jobs.
