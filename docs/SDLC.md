# Hospitality Platform — SDLC and information governance

**Status:** Bootstrap process, subject to review in PR #2. **Project:** [chuxorg Project 14](https://github.com/orgs/chuxorg/projects/14).

## One authoritative home per concern

| Concern | Authority | Copies and links |
| --- | --- | --- |
| Legacy evidence and inferred business needs | [Dropbox Hospitality](https://www.dropbox.com/home/Hospitality) | Repo issues link to relevant versions |
| Approved greenfield requirements and business decisions | Dropbox approved specification and decision log | Issues quote only scoped acceptance criteria and link to source |
| Work scope and delivery status | GitHub Issues + GitHub Project #14 | Sites may display status but must not keep an independent backlog |
| Implementation, tests, migrations and build | This Git repository | Dropbox may link to PRs and releases |
| Engineering ADRs | `docs/adr/` | Cross-reference Dropbox where decisions change business intent |
| Runtime/application data | Versioned migrations + sandbox/seed environment | Never raw legacy guest records |

**ChatGPT Sites** is an optional read-only cockpit unless its data sources and write semantics are explicitly verified. An exported Project JSON is *not* the authoritative task database or an automatic Codex synchronization channel.

## Three analytical layers
1. **LEG** — observed legacy behavior with evidence path and confidence, no inferred business mandate.
2. **INF** — plausible business requirement, assumptions and owner-validation status.
3. **REQ** — explicitly accepted greenfield behavior with objective acceptance tests.

Do not promote an INF to REQ merely because the legacy implementation contains it. Record decision rationale, owner approval and scope/phase.

## Story lifecycle
1. **Discovery/backlog:** ChatGPT and owner convert approved REQ into one bounded GitHub issue; link exact Dropbox doc/version, record story acceptance tests. If requirements are not approved, issue is research/spike, not ready for build.
2. **Ready:** Clear independent outcome, dependencies, exclusions, and test cases; owner/prioritizer accepts scope.
3. **In progress:** Codex checks out a short-lived feature branch from `master`, decomposes into checklist tasks, implements test-first or test-alongside, validates before each commit.
4. **PR review:** Codex opens a draft PR early; CI checks every push; after all acceptance criteria are met and checks green, mark ready and request human review.
5. **Done:** Human owner reviews and authorizes squash merge. GitHub issue closes via `Closes #N`. Update Project status and linked docs if scope or decisions changed.
6. **Change control:** New requirements during implementation are proposed as INF/REQ change. Do not stealth-expand active stories; amend scope explicitly or create a new issue.

Suggested Project statuses: Backlog → Ready → In Progress → Review → Done (Blocked is an optional flag/field rather than another lifecycle step). Suggested fields: Type, Priority, Domain, Phase, Owner. No sprints required initially; manage a small ordered Ready queue.

## Issue granularity and estimation
- **Epic:** a cross-cutting capability tracked by an issue linked to constituent stories; not itself a code PR.
- **Story:** one user-meaningful, demonstrable end-to-end slice; normally one PR. Use child tasks as checkboxes; create separate linked issues only for independent deliveries.
- **Spike:** time-boxed discovery with a written decision/evidence outcome; no feature commitment.
- **Bug/chore:** focused work item with reproducible verification.
Keep stories small enough to review without excessive context. Prefer vertical slices over separate frontend/backend/database stories that cannot pass alone.

## Identifier convention
- Evidence `LEG-RES-001`, inferred needs `INF-RES-001`, approved needs `REQ-RES-001`.
- Domain codes: RES, AVL, GUE, EXP, VEN, FIN, OPS, SEC, PLT.
- IDs are permanent, never reused, including when documents change; use explicit status (proposed/accepted/superseded) and links.
- Engineering decisions `ADR-0001`, `ADR-0002`, etc. (see template).
- GitHub issue `#N` = delivery/story ID; PR `#N` = implementation review ID. Commit references should be traceable through the PR.

## Git and quality contract
- `master` is integration branch. No direct feature commits after bootstrap.
- Branches: `feat/123-reservation-create`, `fix/124-availability-overlap`, `chore/125-ci`.
- Conventional Commits, readable scopes, no noisy WIP commits in merged history.
- Developers/agents run `./scripts/validate.sh` before committing. A Git hook is optional local guard and **not** a substitute for CI.
- Required CI on each PR: formatting/lint, static/type checks as relevant, unit tests, integration tests where appropriate, migration checks when schema changes, and secret/dependency scanning as the stack matures.
- No code without automated tests appropriate to behavior; no unreviewed skipped tests, no misleading mocks as integration coverage. Failures block merge.
- Prefer squash merge; one PR per story; owner authorizes merges; solo work does not require a separate approving review. Document DB migration and rollback when relevant.
- CI checks are only enforceable when repository branch rules can require them. If branch protection is unavailable, owner must manually ensure green CI and PR review before merge.

## Initial bootstrap limitation
This repository initially has no selected application language/framework, and therefore no real application test runner. Bootstrap validation only checks repository policy and documentation. When an application stack is chosen, the **first implementation PR** must replace this with real unit tests and CI. Never represent bootstrap checks as unit test coverage.

## Operational ownership
- **Owner:** business priorities, acceptance, final merge, deployment authorization.
- **ChatGPT:** business analysis, traceability, story drafting, change assessment and independent PR/CI review when asked.
- **Codex:** implement bounded stories, tests, documentation and draft PR, respond to review comments.
- **GitHub Actions:** objective repeatable validation.
- **GitHub Project:** work queue and state, not another requirements database.

## Repository administration checklist
- [x] Set merge strategy to squash preferred; avoid accidental direct merges if possible.
- [x] Require PRs and successful `bootstrap-validation` on master; zero approving reviews for solo work.
- [x] Restrict force pushes and branch deletion of master.
- [x] Enable secret scanning/dependency alerts where available.
- [x] Enable automatic deletion of head branches after merge.
- [x] Configure Project 14 statuses and add repository issues.
- [ ] Confirm which Dropbox documents are approved, and provide Codex with explicit accessible copies/links.

## Verified repository controls (2026-11-14)
- Active `master PR and quality gate` ruleset: PR required, zero approving reviews for solo development, resolve review conversations, required GitHub Actions `bootstrap-validation`, branch must be up to date, no force pushes or branch deletion, no bypass actors.
- Squash is the only allowed merge method; PR title becomes commit title and PR body preserves issue/requirements/test links. CI validates Conventional Commit PR titles. Merged branches are deleted automatically.
- Owner authorization is still required before an agent merges; a separate contributor approval is not required.
- Local pre-commit hook must be installed per `LOCAL_SETUP.md`; CI is the remote gate. Local hooks can be bypassed and do not guarantee secret detection.
- Application unit/integration tests remain Issue #3; bootstrap success proves only the documented checks. Replace/extend validation and required checks when the stack is selected.
- Add `Refs #N` to commit bodies. Map approved REQ IDs and acceptance criteria to test names/comments and record test commands/results in the PR.

## Project operating procedure
Set the issue Status to In Progress when starting its branch. Keep draft implementation work In Progress; move to Review when the PR is ready and CI passes. After the owner authorizes merge, verify the linked issue closes and becomes Done. Reopened issues return to Backlog for triage. Check transitions explicitly until Project built-in workflow targets are verified in the settings UI. The API reports enabled workflows but does not expose their target settings.

Project metadata: Priority P0–P3 (P0 highest), Domain RES/AVL/GUE/EXP/VEN/FIN/OPS/SEC/PLT, Phase Pilot/Later. Native issue Type uses organization issue types. Do not reinterpret these fields as approval of a stack or business requirement. Issue #4 tracks remaining setup/automation limitations.
