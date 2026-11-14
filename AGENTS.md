# Codex Instructions — Hospitality Platform

## Mission and authority
Build a **greenfield** hospitality platform: minimal PMS/CRM foundation plus reusable Experience workflows. Phase 1 is a Thailand cooking experience pilot with simulated payments/commissions, no live integrations.

The legacy PHP/MyISAM PMS is **evidence, not implementation**. Never import it as a code dependency, copy its schema table-for-table, or assume every legacy behavior is required.

### Source-of-truth order
1. Approved greenfield specifications and decisions in the [Dropbox Hospitality workspace](https://www.dropbox.com/home/Hospitality); do not assume Dropbox content is automatically visible in the execution environment.
2. Linked GitHub Issue (implementation scope and acceptance criteria), provided it does not contradict approved specs.
3. Repository architecture decisions (`docs/adr/`) and tests.
4. Legacy inventory, inferred requirements, and discussions only as *unapproved reference material*.

If authoritative Dropbox material cannot be accessed, **stop before implementing affected behavior**, state what document/version is missing, and request an accessible sanitized copy. Never quietly guess. Do not edit the approved Dropbox source from the Git checkout.

## Work intake
- Work only from a GitHub Issue marked Ready or explicitly authorized by the project owner. Use one delivery story per issue and, ordinarily, one PR per story.
- Issue descriptions include goal, background, linked requirement IDs, acceptance criteria, test cases, scope exclusions, dependencies, and tasks (checklist).
- Branch from current `master` using `feat/<issue>-<slug>`, `fix/<issue>-<slug>`, `chore/<issue>-<slug>`, `docs/<issue>-<slug>`.
- Raise missing acceptance criteria, changed scope, significant architectural tradeoffs, or new requirements in the issue before implementing them.

## Implementation
- Favor a modular monolith for the pilot; avoid premature microservices or live external integrations.
- Preserve clear interfaces between PMS/CRM foundations and Experience layer.
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

Domains initially: `RES` reservations, `AVL` availability, `GUE` guest/CRM, `EXP` experiences, `VEN` vendors, `FIN` simulated finance/commissions, `OPS` operations, `SEC` security, `PLT` platform.

See [docs/SDLC.md](docs/SDLC.md) for the full lifecycle.
