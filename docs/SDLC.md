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
- Prefer squash merge; one PR per story; owner approves merges. Document DB migration and rollback when relevant.
- CI checks are only enforceable when repository branch rules can require them. If branch protection is unavailable, owner must manually ensure green CI and PR review before merge.

## Initial bootstrap limitation
This repository initially has no selected application language/framework, and therefore no real application test runner. Bootstrap validation only checks repository policy and documentation. When an application stack is chosen, the **first implementation PR** must replace this with real unit tests and CI. Never represent bootstrap checks as unit test coverage.

## Operational ownership
- **Owner:** business priorities, acceptance, final merge, deployment authorization.
- **ChatGPT:** business analysis, traceability, story drafting, change assessment and independent PR/CI review when asked.
- **Codex:** implement bounded stories, tests, documentation and draft PR, respond to review comments.
- **GitHub Actions:** objective repeatable validation.
- **GitHub Project:** work queue and state, not another requirements database.

## Repository administration checklist (manual until supported)
- [ ] Set merge strategy to squash preferred; avoid accidental direct merges if possible.
- [ ] Require PR review to merge into master and required successful CI checks (where plan supports rulesets/branch protection).
- [ ] Restrict force pushes and branch deletion of master.
- [ ] Enable secret scanning/dependency alerts where available.
- [ ] Enable automatic deletion of head branches after merge.
- [ ] Configure Project 14 statuses and add repository issues.
- [ ] Confirm which Dropbox documents are approved, and provide Codex with explicit accessible copies/links.
