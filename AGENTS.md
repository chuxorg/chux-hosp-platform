# Codex Instructions — Hospitality Platform

## Mission and authority
Build a **greenfield, modular Hospitality Platform**, not a PMS with built-in extras. The first functional module is a standalone PMS informed by the private legacy PHP/MyISAM capability evidence. Separate Experiences (Thai Cooking after PMS), Procurement, Operations and Loyalty modules are independently useful and integrate only through explicit contracts. One FastAPI deployable/database initially; no broker, microservices, plugin framework or live integrations as prerequisites.

The legacy PHP/MyISAM PMS is **evidence, not implementation**. Never import it as a code dependency, copy its schema table-for-table, or assume every legacy behavior is required.

### Source-of-truth order
1. Approved greenfield specifications and decisions in the [Dropbox Hospitality workspace](https://www.dropbox.com/home/Hospitality); do not assume Dropbox content is automatically visible in the execution environment.
2. Linked GitHub Issue (implementation scope and acceptance criteria), provided it does not contradict approved specs.
3. Repository architecture decisions (`docs/adr/`) and tests.
4. Legacy inventory, inferred requirements, and discussions only as *unapproved reference material*.

If authoritative Dropbox material cannot be accessed, **stop before implementing affected behavior**, state what document/version is missing, and request an accessible sanitized copy. Never quietly guess. Do not edit the approved Dropbox source from the Git checkout.

## Module boundaries — ADR-0002
- The owner-approved modular architecture is recorded in [ADR-0002](docs/adr/ADR-0002-modular-hospitality-platform.md), [Dropbox module ownership](https://www.dropbox.com/scl/fi/4ia47cl33y9ibtighkm3j/Hospitality-Modular-Platform-Module-Boundaries-and-Integration-Contracts.paper?rlkey=x5zlc6w9s6u2w9yqh94yye8yk&dl=0) and the [legacy capability catalog](https://www.dropbox.com/scl/fi/nbk31b6fa9sv347xf4iog/Legacy-PMS-Capability-Catalog-and-Proposed-Module-Ownership.paper?rlkey=xycfneji3bchtiy658rlsf8zo&dl=0).
- PMS owns accommodation/property, physical rooms/rates, stays and folios. Experiences owns offerings/sessions/experience bookings and can run **without PMS**. Loyalty owns its rules/ledger and is optional. Procurement owns sourcing; Operations owns reusable work orders when justified by more than one use case.
- Modules own their persistent tables, migrations and invariants. **No cross-module internal imports or direct SQL against another module's tables.** Use public typed DTOs/ports for immediate operations, typed versioned in-process events for notifications; an event without durable storage is best-effort, not guaranteed delivery.
- Standard module shape as needed: `domain/`, `application/`, `infrastructure/`, `api/`. `platform/` provides only minimal wiring, shared contracts/identity/auth when approved, and audit infrastructure. Do not generate empty modules/framework layers prematurely.
- Build PMS first in small end-to-end increments: property → rooms/rates → availability → guest/stay → check-in/out → folio. Existing Experiences stories remain Backlog until PMS foundation and detailed acceptance criteria are ready.
- Every new vertical slice explicitly records affected modules, provided/consumed contracts, required optional modules, owned tables/migrations, test boundaries, and independent startup/contract test implications.
- Do not infer implementation rules from legacy status codes, agent invoicing flows or duplicated reservation structures; preserve LEG → INF → owner-accepted REQ → test traceability.

## Work intake
- Work only from a GitHub Issue marked Ready or explicitly authorized by the project owner. Use one delivery story per issue and, ordinarily, one PR per story.
- Issue descriptions include goal, background, linked requirement IDs, acceptance criteria, test cases, scope exclusions, dependencies, and tasks (checklist).
- Branch from current `master` using `feat/<issue>-<slug>`, `fix/<issue>-<slug>`, `chore/<issue>-<slug>`, `docs/<issue>-<slug>`.
- Raise missing acceptance criteria, changed scope, significant architectural tradeoffs, or new requirements in the issue before implementing them.

## Implementation
- Favor a modular monolith for the pilot; avoid premature microservices or live external integrations.
- Enforce each module's public contracts and data ownership boundaries; PMS must not depend on Experiences/Loyalty and Experiences must not require PMS.
- New production behavior requires automated unit tests, negative/edge cases, and relevant integration tests when persistence or cross-module behavior is involved.
- Run the repo's declared validation command **before every commit**, and again before the PR is marked ready. Do not bypass failing tests. If tests cannot run, report the blocker and keep the PR draft.
- New application stack selection requires an ADR, actual reproducible local validation, and equivalent GitHub Actions CI. A documentation-only bootstrap is not evidence of application test coverage.
- No raw legacy archives, databases, credentials, secrets, personal guest data, or copied production records in the repository or CI logs.

## Commits, PRs and review
- Use Conventional Commits: `feat:`, `fix:`, `test:`, `docs:`, `refactor:`, `chore:`, `ci:` (optional scope: `feat(reservations): ...`).
- Keep each commit coherent and passing available validation.
- Create draft PRs early; use the PR template, link the issue with `Closes #N`, and include requirement IDs, tests executed with results, migration considerations, and any unresolved questions.
- Update issue task checkboxes/status as work progresses. Request human review when CI passes; do **not** approve, merge, or bypass protections on behalf of the owner without explicit instruction.
- Preferred integration: **squash merge** after review, yielding one traceable merge commit per story. Delete merged feature branches where practical.
- Do not self-certify compliance; CI reports objective checks, and the human reviewer authorizes merge.

## Traceability identifiers
- Legacy evidence: `LEG-<DOMAIN>-NNN`
- Inferred business requirements: `INF-<DOMAIN>-NNN`
- Approved requirements: `REQ-<DOMAIN>-NNN`
- Design decisions: `ADR-NNNN` (repo-local implementation decisions), with Dropbox link when business-significant.
- Tests cite the relevant `REQ-...` in test names or nearby comments; issues and PRs map each acceptance criterion to its test.
- Delivery identifiers are canonical GitHub `#issue` and `#PR` numbers; don't invent parallel story IDs. Include `Refs #N` in commit bodies and `Closes #N` in PRs.

Domain identifiers: `RES` reservations, `AVL` availability, `GUE` guest/CRM, `EXP` experiences, `VEN` vendors, `FIN` finance, `OPS` operations, `PLT` platform, `SEC` security, `PMS` hotel operations, `SRC` procurement/sourcing, `LOY` loyalty. Existing IDs are never renumbered.

See [docs/SDLC.md](docs/SDLC.md) for the full lifecycle.
