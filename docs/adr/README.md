# Architecture Decision Records

Use `ADR-NNNN-short-slug.md` for decisions about the **new** implementation (stack, database, architectural boundaries, API contracts, migrations, deployment).

Template:
```md
# ADR-0001: <title>
Status: Proposed | Accepted | Superseded
Date: YYYY-MM-DD
Owners: <names/roles>
Links: GitHub #issue, Dropbox decision/requirement

## Context
## Decision
## Alternatives considered
## Consequences and risks
## Validation / review trigger
```

Current decisions:
- [ADR-0002 — Modular Hospitality Platform](ADR-0002-modular-hospitality-platform.md): business-module ownership and in-process contracts.
- [ADR-0003 — Go + PostgreSQL](ADR-0003-go-postgres-defer-redis.md): implementation stack and Redis deferral (supersedes ADR-0001's stack).
- [ADR-0004 — Configuration-driven business policies](ADR-0004-configuration-driven-business-policies.md): accepted architectural direction; formal repository adoption pending PR merge. Business-specific hotel policies remain unapproved until their stories are accepted.
- [ADR-0005 — Configurable authorization across groups and resources](ADR-0005-configurable-authorization.md): OIDC-based authentication target, resource-neutral groups, group-scoped configurable roles and permissions, and API-enforced authorization with Casbin.

For stories with hotel-variable business rules, use [the policy design checklist](../POLICY_DESIGN_CHECKLIST.md) to distinguish invariant logic from allowed configuration and to record ownership, role, effective date, explanation, evidence and tests.

Do not document a business hypothesis as an accepted ADR before the owner approves it. Important business decisions should also be reflected in the approved Dropbox decision log.
