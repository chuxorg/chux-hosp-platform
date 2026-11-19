# ADR-0005: Configurable authorization across groups and resources
Status: **Accepted by the project owner on 2026-11-19.**
Date: 2026-11-19
Owner: Hospitality project owner
Links: Authorization design discussion; implementation issue to be linked when created.

## Context

The platform needs real authentication and authorization from its foundation. The first UI will support Property Manager and Front Desk Operator roles, while the model must allow additional roles and resources, including organization-wide capabilities such as loyalty programs.

Groups must remain resource-neutral. A group may represent an organization, department, regional team, or another configured access context. It must not inherently mean a collection of properties.

## Decision

1. **Separate authentication from authorization.** A trusted identity provider authenticates users. The Hospitality platform evaluates what an authenticated user may do. The platform will not implement its own password-authentication system.

2. **Use OpenID Connect as the identity-provider integration target.** The specific provider and session design remain open pending deployment and operational requirements.

3. **Use Casbin as the authorization policy evaluator.** Casbin will evaluate access decisions in the Go application. The platform remains the system of record for users, group membership, role assignments, permissions, and resource grants.

4. **Keep groups generic and resource-neutral.** A group is an authorization context. Explicit resource grants associate groups with resources such as a property or loyalty program.

5. **Scope role assignments to groups.** A user may have different roles in different groups. A role may be reused across groups. The group context and resource grant determine where its permissions apply.

6. **Define permissions as `resource.action`.** Ordinary data permissions use `create`, `read`, `update`, and `delete`. Domain actions with distinct business meaning use explicit verbs such as `reservation.cancel`. Cancellation does not imply deletion. Umbrella permissions such as `manage` or `override` are not part of the initial vocabulary; if introduced later, their included permissions must be explicit.

7. **Keep roles configurable.** Property Manager and Front Desk Operator are initial demo roles, not hard-coded authorization identities. Both may cancel eligible reservations in the demo. Additional roles and permission assignments can be defined later.

8. **Enforce authorization in the API.** UI visibility is a usability aid; every protected API operation must enforce authentication, permission, group context, and resource access. Access is denied when a required assignment or grant is absent.

## Alternatives considered

- **UI-only role simulation:** rejected because it would not protect API operations.
- **Hard-coded role checks:** rejected because adding or changing roles would require application-code changes.
- **Property-bound groups:** rejected because authorization must support non-property resources and organizational contexts.
- **OPA or OpenFGA:** not selected. Casbin's domain-scoped RBAC appears to fit the current group-scoped role model with less additional infrastructure; revisit if policy needs outgrow this model.

## Consequences and risks

- The Go API needs authentication middleware, authorization checks, and resource-scope enforcement before protected operations are exposed to real users.
- The data model must represent users, groups, role definitions, permissions, group-scoped user-role assignments, and explicit group-to-resource grants.
- Authorization decisions require tests for allowed and denied requests, including cross-group and cross-resource cases.
- The identity provider, sign-in/session flow, user provisioning, and administration experience remain unresolved.
- Casbin evaluates policy; it does not replace the platform's ownership of authorization data or resource checks.
- This design adds upfront implementation work, including identity-provider integration and protected API enforcement, before the UI can safely rely on user roles.
- The authorization model must prevent a group assignment or role permission from implicitly granting access to unrelated resources.

## Validation / review trigger

Before implementation, validate the model against concrete PMS and organization-level scenarios, including:

- A Front Desk Operator can cancel a reservation at an authorized property.
- The same user cannot read a reservation at an ungranted property.
- A Front Desk Operator cannot change rates.
- An Accountant can read an organization's loyalty program but cannot update it without that permission.
- A user may hold different roles in different groups.
- An authenticated user without the required assignment or resource grant is denied.
- The API denies access even when a client manually invokes an operation hidden by the UI.

Review this ADR when selecting the identity provider and session design, or if authorization requirements require resource relationships or policy logic beyond Casbin's role-and-domain model.
