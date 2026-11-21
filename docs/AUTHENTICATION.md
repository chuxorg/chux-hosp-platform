# Auth0 OIDC browser authentication — Issue #49

This is the authentication-only BFF foundation accepted in ADR-0005. It proves
identity and maintains an opaque PostgreSQL-backed browser session. It does not
grant a role, resource, property or PMS permission. Keep existing PMS APIs in a
trusted synthetic environment until the separate authorization slice protects
them.

## Auth0 application

Create a **Regular Web Application** in the selected Auth0 tenant. Configure the
exact application URLs used by the server:

- Allowed Callback URL: `https://<app-host>/auth/v1/callback`
- Allowed Logout URL: the value supplied as `OIDC_LOGOUT_RETURN_URL`

Use the tenant's HTTPS issuer URL (for example,
`https://tenant-region.auth0.com`) and its application client ID and secret.
Do not commit the secret, place it in an image, or print it in logs.

## Server configuration

Authentication is off unless explicitly enabled:

```text
AUTH_ENABLED=true
OIDC_ISSUER_URL=https://tenant-region.auth0.com/
OIDC_CLIENT_ID=<client-id>
OIDC_CLIENT_SECRET=<secret>
OIDC_REDIRECT_URL=https://app.example/auth/v1/callback
OIDC_LOGOUT_RETURN_URL=https://app.example/
AUTH_SESSION_IDLE_TIMEOUT=8h
AUTH_SESSION_ABSOLUTE_LIFETIME=12h
```

HTTPS and Secure cookies are required by default. For an explicit localhost
development exception only, set `AUTH_ALLOW_INSECURE_HTTP=true` and use
`http://localhost` or `http://127.0.0.1` callback/logout URLs. Never use that
setting in a deployed environment.

Apply migrations before enabling authentication:

```sh
hospitality migrate up
```

Startup performs OIDC discovery and fails closed if Auth0 or the PostgreSQL
authentication tables cannot be reached. Authentication routes are under
`/auth/v1`: `GET /login`, `GET /callback`, `GET /session`, and the
CSRF-protected `POST /logout`.

## Provisioning and revocation

Auth0 login never creates a Hospitality user. Obtain the stable Auth0 `sub`
claim for the intended account and provision it through the controlled CLI:

```sh
hospitality auth provision-user \
  --issuer https://tenant-region.auth0.com/ \
  --subject 'auth0|stable-subject' \
  --display-name 'Synthetic Front Desk Operator'
```

This creates identity only and explicitly grants no role or resource access.
Run the later authorization provisioning flow before exposing protected hotel
operations. To disable the identity and revoke every application session:

```sh
hospitality auth revoke-user \
  --issuer https://tenant-region.auth0.com/ \
  --subject 'auth0|stable-subject'
```

## Browser behavior

The callback validates Auth0's signature, issuer, audience and expiry using
`coreos/go-oidc`, and separately validates one-time state, nonce and PKCE S256.
The browser receives a random HttpOnly session cookie and a session-bound CSRF
cookie. OIDC tokens are not stored in browser storage or retained in the
application session. JavaScript reads `csrf_token` from `GET /auth/v1/session`
and sends it as `X-CSRF-Token` on `POST /auth/v1/logout`; Origin is checked when
present.

Sessions expire after eight idle hours or twelve absolute hours by default.
Activity can extend idle expiry only up to the absolute deadline. Logout and
administrator revocation take effect immediately through PostgreSQL.
