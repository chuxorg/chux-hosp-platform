# PMS Property / RoomType / Room configuration (Issue #22)

**Status:** First bounded PMS business slice, owner review pending. No guest data, rates, availability, reservations or production authentication.
**Requirement trace:** LEG-AVL-001/002 → INF-AVL-001 → REQ-PMS-001 → **TST-PMS-001-01**.
**Discovery:** [Issue #21 legacy evidence and proposed decisions](https://github.com/chuxorg/chux-hosp-platform/pull/35) (discovery PR #35; proposed scope remains subject to owner review).

## Purpose and model

- `Property`: `id`, `name`, `time_zone` (validated IANA), `active`.
- `RoomType`: `id`, `property_id`, `name`, `max_occupancy` (positive integer), `active`.
- `Room`: `id`, `property_id`, `room_type_id`, `number` (a physical room identifier), `active`.

**Scope caveat:** The legacy `room_rate` row bundled room identity, hotel mapping and prices. Our physical Room is a greenfield distinction; the source does not prove every `room_rate` row was exactly one physical unit. Occupancy, IANA time zone, uniqueness and activation rules are **proposed** PMS configuration policy, not observed historical rules. The owner's code review must confirm this scope before merge.

Database constraints:
- All tables are exclusively PMS-owned: `pms_properties`, `pms_room_types`, `pms_rooms`.
- Each RoomType belongs to one Property; room-type name is unique case-insensitively within that Property.
- Each Room belongs to a Property and a RoomType belonging to **the same Property** (composite foreign key), with room number unique case-insensitively within that Property.
- Active state is a configuration flag, **not** a validated legacy `roomstatus` code and **not** a live allocation/housekeeping state.
- No destructive API operations. Future booking stories must define whether a room with an active stay can be disabled or reassigned; that policy is **not implemented** here.
- A relational violation causes the single SQL statement to fail atomically; PostgreSQL guards concurrent inserts.
- RoomType capacity is a maximum-occupancy placeholder for later booking rules; adults/children, extra beds and infant policies are not defined.

## Development-only JSON API

All commands are under `/pms/v1` and currently require **local-only or otherwise isolated network access**. There is **no authentication, tenant-level authorization or user-role enforcement** in Issue #22. Do not expose these endpoints to the public internet or real hotel staff/customer data. Production security requires a separately approved story before deployment.

| Method | Path | Meaning |
| --- | --- | --- |
| POST | `/properties` | Create Property |
| GET / PUT | `/properties/{propertyID}` | Read / replace Property |
| POST | `/properties/{propertyID}/room-types` | Create RoomType |
| GET / PUT | `/properties/{propertyID}/room-types/{roomTypeID}` | Read / replace RoomType |
| POST | `/properties/{propertyID}/rooms` | Create physical Room |
| GET / PUT | `/properties/{propertyID}/rooms/{roomID}` | Read / replace Room |

Request bodies require `Content-Type: application/json` and an explicit `active` boolean. PUT replaces the mutable fields with a **complete** body; no PATCH support. Unknown fields, invalid IDs, trailing JSON, invalid time zones, zero/negative occupancy and blank names/numbers are rejected. Responses include generated stable numeric IDs.

Example on a local instance with migration `00002_pms_configuration.sql` applied:

```bash
# POST returns a generated id, for example 1:
curl -X POST http://127.0.0.1:8000/pms/v1/properties \
  -H 'Content-Type: application/json' \
  -d '{"name":"Pilot Hotel","time_zone":"Asia/Bangkok","active":true}'

# Substitute the actual property ID from the response:
curl -X POST http://127.0.0.1:8000/pms/v1/properties/1/room-types \
  -H 'Content-Type: application/json' \
  -d '{"name":"Standard","max_occupancy":2,"active":true}'

# Substitute the returned room_type_id:
curl -X POST http://127.0.0.1:8000/pms/v1/properties/1/rooms \
  -H 'Content-Type: application/json' \
  -d '{"room_type_id":1,"number":"A101","active":true}'
```

Expected HTTP statuses: 201 created with Location header, 200 read/update, 400 malformed/invalid request, 404 unknown resource, 409 duplicate/collision, 422 missing or cross-property parent, and 500 sanitized unexpected infrastructure error. No raw SQL/connection error text is returned. The API is not publicly advertised as production-ready.

## Layers / composition

- `internal/modules/pms/domain/`: immutable-by-contract data transfer structs; no DB, HTTP or other-module imports.
- `internal/modules/pms/application/`: validated use cases and a typed **Store port**.
- `internal/modules/pms/infrastructure/`: PostgreSQL-backed Store using parameterized SQL and module-owned tables.
- `internal/modules/pms/api/`: request parsing, route ownership and sanitized errors, no direct SQL.
- `cmd/hospitality/composition.go`: binds infrastructure to the application and mounts PMS. No optional modules loaded; Platform probes unchanged.

Do not reuse this Stay/Room model as an Experiences Session model or as an all-purpose shared Facility.

## Tests / validation

- `application.TestServiceWithFakeStore`: validates business inputs, read/update, and substituting a fake for the typed Store port without PostgreSQL.
- `infrastructure.TestPMSConfigurationAgainstPostgres` (integration): real PostgreSQL migration up/down/reapply, CRUD, unique names/numbers, invalid time zones/capacity/parent, cross-property type rejection, no partial writes on failed updates and simultaneous duplicate-room creation.
- Existing PMS-only composition test, import-boundary architecture suite, PostgreSQL bootstrap tests, Docker smoke and required `bootstrap-validation` stay green.
- Tests only create/drop their own cryptographically unique disposable test database. The supplied `TEST_DATABASE_URL` provisioning DB must end in `_test` and must not be the application DB.

**Next slices:** Issue #23 now implements the owner-approved [nightly rate and inventory-eligibility contract](PMS_RATES_AND_ELIGIBILITY.md). Reservation-aware blocking/overlap and atomic room allocation remain #24; #25 owns check-in/out and #26 folio. Nothing in Issue #22 validates those workflows.
