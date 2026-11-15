# ADR-0004: Configuration-driven business policies with module-owned evaluators

Status: **Accepted as architectural direction by the project owner on 2026-11-15; repository adoption pending reviewed merge.** Specific hotel policy types, permissions and data contracts remain subject to story-level business acceptance.
Date: 2026-11-15
Owner: Hospitality project owner
Links: [Architecture adoption #38](https://github.com/chuxorg/chux-hosp-platform/issues/38); [PMS policy discovery #39](https://github.com/chuxorg/chux-hosp-platform/issues/39); [room rate/candidate query #23](https://github.com/chuxorg/chux-hosp-platform/issues/23); [guest/stay allocation #24](https://github.com/chuxorg/chux-hosp-platform/issues/24); [ADR-0002](ADR-0002-modular-hospitality-platform.md); [ADR-0003](ADR-0003-go-postgres-defer-redis.md); [approved product direction](https://www.dropbox.com/scl/fi/zu6lowh2qhiumdxenbbvy/03-Approved-Greenfield-Specification-REQ.paper?rlkey=0ue4d29y20azrkjs0yg4ai0cu&dl=0).

## Context

Hotel operators can share the same core PMS capabilities while differing in commercial eligibility, rate-plan restrictions, market segments, allotments and other operating practices. The rules may also change within one property over time. For example, a Long Stay offering may impose a minimum stay or a room-night threshold, while a Honeymoon or Family plan may impose different requirements.

A legacy PMS may encode these differences as hardcoded conditions. Reimplementing per-hotel branches in Go would create brittle deployments and make routine business changes require engineering releases. The opposite extreme—letting a hotel write code, SQL, YAML, a rule expression language, or an unbounded workflow definition—would undermine security, correctness and operability.

The owner approved a middle path: **business invariants live in implementation; permitted business variations live in validated policy configuration operated through a UI by authorized hotel personnel**. Solution architects / implementation teams design and test the supported policy types; product/domain owners accept their meaning. Hotels configure only the supported parameters, scopes and effective periods that their permissions allow.

This decision *extends* ADR-0002 modular ownership; it does not establish a shared universal rules engine or change ADR-0003's Go/PostgreSQL architecture.

## Decision

1. **Distinguish invariant from variation.** Non-negotiable safety, security, ledger and transaction invariants are implemented and tested in code and enforced in PostgreSQL where appropriate. Examples: a room cannot be atomically confirmed twice for overlapping stays; a property may not read another property's confidential records; a financial amount cannot silently overflow. **No hotel-configured policy may disable an invariant.** Business conditions that legitimately differ—e.g., a minimum qualified length of stay, a rate plan's market eligibility, an allotment limit, or an approval threshold—are *candidates* for explicit, scoped configuration after story-level acceptance.

2. **Keep policy vocabulary finite, typed and module-owned.** Add a small policy type and its Go evaluator only for an accepted use case. Its input schema, allowed values, evaluator behavior, output and failure conditions are fixed in reviewed application code and tests. Configuration is **data**, not executable logic. No runtime scripts, arbitrary expressions, user-supplied SQL/YAML, dynamic plugins, centralized rule interpreter, cross-module ORM, or generalized workflow engine.

3. **Separate who defines a policy from who configures it.**
   - **Product/domain owner:** approves the semantic meaning, supported variations, exceptions, precedence and acceptance criteria; engineering cannot infer requirements from old PHP alone.
   - **Solution architects / implementation developers:** design the typed policy contract, bounded parameter validators, invariant portions and evaluator algorithms; make changes via reviewed code and migrations when needed.
   - **Authorized hotel roles:** through ordinary validated forms, choose approved values, scope and applicability/effective dates; may preview proposed outcomes and use any configured approval flow. They do **not** edit evaluator code, SQL or YAML.
   - **Platform/security:** implements authentication, per-property authorization and change audit when those services are approved. This ADR does not invent a specific role name or confer permissions on the current unauthenticated development APIs.

4. **Policies have explicit ownership and applicability.** PMS owns its accommodation rate-plan, market-segment, length-of-stay, allotment and reservation policies; Experiences/Procurement/Operations/Loyalty own their own future policies. A policy may be scoped to a property, room type, rate plan, market segment or another **approved** dimension. Define defaults, specificity, conflicts, precedence and absent-policy behavior **per actual policy type** in its accepted story. Do not assume a generic universal inheritance or override tree. A policy must not grant access to another property.

5. **Version and effective-date active policies.** Once a policy is used to make a material commitment, preserve enough immutable identity and applied values to explain the decision and protect existing confirmed reservations from a later policy edit. Provide effective dating in the property's business time zone when that rule requires it, avoiding accidental retroactive changes. Design snapshots/references, publication and retirement rules **with the first concrete policy story**; the ADR does not mandate policy tables or a particular policy state machine today.

6. **Produce explainable and deterministic decisions.** Evaluations should use explicit typed context and return a structured outcome, the applicable policy/type/version, and concise reasons (e.g., required 15 nights, requested 7), without disclosing other properties' data or sensitive guest details. Include validation and preview paths for hotel operators before applying a policy. Invalid or contradictory configuration must be rejected or fail according to a documented, safe, deterministic rule; it must not silently broaden eligibility.

7. **Separate eligibility, inventory availability and reservation confirmation.**
   - **Configured room candidates:** physical room, room type, property and current base rate are configured and active. This is the intentionally narrow **#23** read-only result (`search_mode: inventory_eligibility`, `reservation_conflicts_checked: false`). The label is historical to the current API and **does not demonstrate market-segment eligibility**.
   - **Commercial eligibility:** proposed booking/request meets the selected rate plan, segment and other approved policy requirements. A property-specific Long Stay rule is a *possible* example, not yet an approved universal value or evaluation algorithm.
   - **Sellable availability:** date-specific capacity/allotment and blocking constraints, including any relevant existing reservations/holds, permit selling under the selected inventory allocation policy.
   - **Confirmed reservation:** the system atomically revalidates applicable eligibility/availability and commits an allocation in authoritative PostgreSQL under concurrency protection. Neither a search result nor an in-memory event is an allocation guarantee.
   
   *Important vocabulary ambiguity:* `15 room-nights` may mean 15 consecutive nights in one room **or** 15 aggregate room-nights across multiple rooms; do not equate these or hardcode a threshold until business owners resolve this in #39.

8. **Audit policy changes and evaluation context.** When actual policy management is implemented, record who changed/published a policy, for which property and scope, the previous/new values, timestamps, approval where required and version/effective periods. Bind policy evidence or applied conditions to a booking confirmation as required for reproducibility; don't recalculate old commitments against today's policies. Avoid storing unneeded guest PII. Audit enforcement belongs at authorized business/API boundaries and durable storage, not UI alone.

9. **Build the smallest concrete implementation.** No new shared policy framework or UI framework is authorized by this ADR. Start with the minimum domain-specific evaluators justified by evidence and accepted REQs; extend the PMS application boundary with typed value objects/ports only when needed. Keep a catalog of policy types/constraints **as documentation** before introducing generalized tables. Separate a hotel's business policy settings from code deployments, with safe validation and change review.

10. **Proof and regression tests.** For every implemented policy type, test its approved scope and default/precedence, range/boundary cases, effective dates/time-zone edges where material, invalid and conflicting configuration, permissions (once available), preview versus committed decision, historical snapshot/version preservation, safe error behavior, and PostgreSQL concurrency when inventory or money is affected. Test PMS without optional modules. Do not let a hotel policy weaken an invariant.

## Roles and example (illustrative; not yet accepted operational rules)

Consider Hotel A's future `Long Stay` rate plan for `Deluxe` rooms. A solution architect might implement a typed `MinimumStayNights` evaluator. An authorized hotel operator might later set the minimum to **15 nights**, effective on a particular date, through a form with clear validation and a result preview; Hotel B could set **21**. The evaluator validates both using the same reviewed code. Existing bookings preserve their accepted terms. A separate allotment decision must still check remaining inventory. The sample values, segment names, effective dates and rule precedence are **illustrations**, not requirements to code now.

A `room-night` volume threshold could require a **different** evaluator if the business rule concerns multiple rooms × nights rather than per-room minimum stay. The consultant/product owner must make that distinction before implementation.

## Alternatives considered

- **Per-property hardcoded branches:** low initial effort but changes require code/deployment and invite unintended differences between properties. Rejected.
- **Arbitrary SQL/YAML/scripts or visual low-code rule builder for hotel employees:** offers flexibility but exposes business operators to a programming language, increases injection/security risk and hinders consistent testing. Rejected.
- **Generic rules engine, policy DSL, global workflow engine or shared policy database for every module:** front-loads abstraction, precedence, lifecycle and debugging overhead before business cases justify it. Rejected.
- **Typed module-owned evaluators with validated scoped configuration and UI:** selected; preserves variation and maintainable invariants with smaller, testable contracts.

## Consequences and risks

- Some new policy types still require a Go release; editing an **existing supported policy's permitted values** should not.
- Data models must eventually support policy versioning, business-effective dates and historical booking evidence. Those schema details **are not approved by this ADR alone**.
- Multi-property isolation and authorization are security gates. Current PMS development endpoints in #22/#23 remain unauthenticated and must not be treated as operator-ready production UI.
- Invalid/conflicting policy configurations need explicit semantics; unspecified defaults are dangerous. Use bounded, documented behavior instead of guessing.
- Search/preview results can become stale; confirmation rechecks authoritative conditions atomically.
- An overly broad policy catalog can quietly become a rules engine. Require a named real scenario and accepted REQ before introducing a new type, precedence layer or abstraction.

## Scope, adoption and validation triggers

- **#23 is unchanged.** Its positive THB base rate and date-validated *configured candidates* remain as approved; do not retrofit a market eligibility/allotment engine into its PR. Clarify misleading public-facing terminology in a separately reviewed contract change when needed.
- **#39 is the discovery gate** for RatePlan, MarketSegment, allotment semantics, minimum stay vs aggregate room-nights, UI roles, precedence and versioning. These details require explicit operator/consultant input and their own REQ/TST approval.
- **#24 must still guarantee physical-room allocation integrity** in PostgreSQL. Do not delay that invariant by inventing a generic policy engine. Any segment/allotment behavior that is essential to the first accepted reservation scenario must be specified through #39 or scoped into a separately approved story **before** its implementation.
- When introducing the first policy variation, document its **policy type, owner, supported scope, configurable fields, who may edit, default/conflict rules, evaluation contract, effective-date behavior, snapshot/audit strategy and objective tests**.
- Revisit this ADR only after evidence of truly shared policy semantics, material performance issues, or a business-approved need for more dynamic policy composition.

The owner has **accepted this architectural principle**. Its repository publication, policy-management implementation and all product-specific variations remain subject to independent PR reviews and requirements acceptance.
