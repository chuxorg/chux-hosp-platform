# Policy design checklist — Hospitality modules

**Authority:** [ADR-0004](adr/ADR-0004-configuration-driven-business-policies.md) (owner-approved architectural direction), [ADR-0002](adr/ADR-0002-modular-hospitality-platform.md), [approved requirements](https://www.dropbox.com/scl/fi/zu6lowh2qhiumdxenbbvy/03-Approved-Greenfield-Specification-REQ.paper?rlkey=0ue4d29y20azrkjs0yg4ai0cu&dl=0). This checklist is a *delivery contract/template*, **not an instruction to create a policy engine or a generalized policy schema**.

## Requirement intake and ownership

Before implementing a hotel-variable rule, capture these items in its GitHub story and approved REQ/TST:

| Field | Questions to answer |
| --- | --- |
| Business scenario and evidence | What hotel/operator variation is real? What source is observed LEG vs inferred INF vs approved REQ? |
| Owning module | PMS, Experiences, Procurement, Operations or Loyalty? Does any other module genuinely need a stable public port? |
| Invariant vs parameter | Which conditions may be configured by a hotel, and which must remain enforced by code/DB? |
| Policy type | What named, typed evaluator is needed? Can an **existing** evaluator satisfy this variation? |
| Applicability | Property? RoomType? RatePlan? MarketSegment? Other approved dimensions? Are they mutually consistent? |
| Parameters | Exact type, units, required/optional fields, ranges, blank handling, defaults, invalid-combination behavior |
| Effective period | Which hotel calendar/time zone and time interval? When does a change take effect? What happens to past/future bookings? |
| Conflict/precedence | What if multiple scoped policies match or none matches? Is override allowed, by whom? Avoid guessed default inheritance |
| Operator control | Which **authorized role** configures values via a UI form, who approves/publishes them, and who may preview changes? |
| Decision evidence | Explainable result, applied policy type/identifier/version and relevant input facts; safe log/redaction |
| Historical integrity | What immutable policy reference/snapshot is bound to a booking or financial commitment? |
| Authoritative writes | What PostgreSQL constraints/transactions preserve correctness despite concurrent requests or stale searches? |
| Tests | Allowed/denied boundaries, invalid config, time zones, isolation, version changes, conflicts, regression, permission checks |
| Rollout | Migrations, backward compatibility, previously confirmed stays, operational rollback and data safety |

A policy type is not complete simply because a hotel can enter values. There must be an **approved interpretation**, safe evaluator, error behavior and tests.

## Roles and separation of duties

- **Hotel user (authorized, future UI):** selects from supported policy options; changes bounded values and approved applicability windows; sees previews and explanations; cannot write code, SQL, YAML or run arbitrary expressions.
- **Solution architect/implementation developer:** defines supported Go evaluator types, validation, invariant guards and published contracts; changes code through issue/PR/test process.
- **Product/domain owner:** approves what a policy means, whether it may vary, which properties or segments it covers, and objective acceptance tests.
- **Platform/security owner:** defines/enforces property/tenant permissions and change audit when approved. **Do not assume a role name or security infrastructure exists today.**

These are governance responsibilities, not implemented authorization grants. Current dev-only PMS endpoints are **not** a substitute for role-restricted policy UI.

## Hospitality example and vocabulary

| Distinct question | Outcome / example | Initial location |
| --- | --- | --- |
| Configured candidate | Is Property, RoomType, Room and current base rate active? | #23, already scoped; date labels validated but no reservation check |
| Commercial eligibility | Does a specific booking meet a RatePlan/MarketSegment rule (e.g. Long Stay threshold)? | [Discovery #39](https://github.com/chuxorg/chux-hosp-platform/issues/39), then separately approved policy slice |
| Sellable availability | Is stock/allotment capacity left for every relevant date, considering blocks/holds/reservations? | To be specified for #24 and later allotment stories |
| Reservation confirmation | Was the permitted physical allocation rechecked and committed atomically? | #24 PostgreSQL transactional invariant |

**Do not treat `15 room-nights` as equivalent to `15 nights`.** One room for 15 nights vs five rooms for three nights have the same aggregate room-nights but different stay-length eligibility. The business must specify which is intended.

## Worked sample: not yet approved

A hypothetical *Hotel A / Deluxe / Long Stay* policy could have a typed `MinimumStayNights` evaluator with configurable parameter `minimum_nights=15`. This is a **demonstration of separation of concerns**, not permission to create that evaluator, field or universal threshold now.

A future configuration UI might present a number field and a result preview such as:

> Requested: 7 nights. Policy: minimum 15 nights. Not eligible.

Another hotel can choose 21 within the same policy type without requiring an engineering release. A new volume-based `MinimumAggregateRoomNights` condition, if needed, is **a separate proposed evaluator** and must be accepted/tested.

## Delivery safeguards for Codex

1. Read ADR-0004 and this checklist during story intake; do not make up domain policy details that the owner has not signed off.
2. If the requested rule varies by hotel, **propose an approved typed configuration point instead of `if propertyID == ...` business logic**.
3. If the required policy type does not exist, stop and request acceptance of the new type; do not build a generic interpreter.
4. Preserve historical booking evidence and implement safe re-evaluation at confirmation; search/preview does not guarantee allocation.
5. Include source-to-REQ-to-TST trace, versioning/effective date/role decisions, and migration/rollback impacts in PR review.
6. Do not retroactively expand #23 with RatePlans or allotments; do not embed an unapproved rules engine in #24.
