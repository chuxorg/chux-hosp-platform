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

## Logout return parameters

The discovered OIDC `end_session_endpoint` receives `client_id` and
`post_logout_redirect_uri=OIDC_LOGOUT_RETURN_URL`. Only the Auth0 `/v2/logout`
fallback (when discovery omits that endpoint) uses legacy `returnTo`. Register
the same exact return URL under the Auth0 application's **Allowed Logout URLs**;
changing the local environment value alone does not update Auth0's allowlist.
See [OIDC logout](https://auth0.com/docs/authenticate/login/logout/log-users-out-of-auth0)
and [legacy logout](https://auth0.com/docs/authenticate/login/logout/redirect-users-after-logout).

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

## Mac localhost test — Issue #52

Use the existing Compose services and the `hospitality` **Regular Web
Application** in Auth0. This setup is separate from #51 authorization; no grants
are created. Docker Desktop must be running; use the repo's pinned Go toolchain
for host validation. Run commands from the repository root in macOS **zsh**.

### Private configuration

The tracked [`.env.example`](../.env.example) contains only placeholders and the
localhost settings. The real local file is **`.env.local`**, already ignored by
`.gitignore` and excluded from the Docker build context. Create it only if absent:

```zsh
git check-ignore -q -- .env.local &&
  (umask 077; set -o noclobber; cat .env.example > .env.local)
```

An existing file causes the copy to fail safely; preserve it and edit locally.
Use your Mac editor to fill in just these values in `.env.local`:

- `OIDC_ISSUER_URL`: `https://<your-auth0-domain>/`, including the trailing slash.
  Use the exact issuer returned by that tenant's discovery document; provisioning
  and callback identity matching compare the issuer exactly.
- `OIDC_CLIENT_ID`: the `hospitality` application's Client ID.
- `OIDC_CLIENT_SECRET`: its Client Secret, entered **only in the local editor**.
  Keep single quotes so Compose treats `$` literally; escape an embedded single
  quote as `\'` per [Compose environment-file syntax](https://docs.docker.com/compose/how-tos/environment-variables/variable-interpolation/#env-file-syntax).

Keep `AUTH_ENABLED=true`, `AUTH_ALLOW_INSECURE_HTTP=true`, `APP_PORT=8000`,
`OIDC_REDIRECT_URL=http://localhost:8000/auth/v1/callback`, and
`OIDC_LOGOUT_RETURN_URL=http://localhost:8000/`. The insecure-cookie exception is
for localhost only. In the Auth0 application's Settings, the exact Allowed
Callback and Allowed Logout URLs must match those two URLs, respectively; see
[Auth0 setup](https://auth0.com/docs/quickstart/webapp/golang). Use `localhost`
consistently in the browser rather than switching to `127.0.0.1`.

Do not put the test user's password in any repo environment file: enter it only
on Auth0's hosted sign-in page. Its `auth0|…` User ID is the stable **subject** for
the provisioning command below, not a client ID, password, role or property grant.

Compose does not merge `.env` into an explicit `--env-file .env.local`. If you
already keep database overrides in `.env`, preserve them: add `--env-file .env`
**before** `--env-file .env.local` on every command below, or retain the same
settings in `.env.local` using your editor. Existing exported shell settings take
precedence over these files; clear stale overrides locally without printing them.
Keep the existing database name/user/password/ports when reusing its volume;
changing POSTGRES variables does not reinitialize that database. Do not reset or
remove an existing database/volume for this test.

Use `config -q` only. Plain `docker compose config`, `config --environment`,
container environment dumps, shell tracing (`set -x`), verbose HTTP traces and
exported HARs can expose credentials/cookies/tokens. Do not paste those outputs,
session JSON, callback URLs, or browser storage into chat, issues or PRs.
The host Go executable has no `.env` loader; these instructions use Compose's
explicit environment-file handling instead of sourcing the secret file.

### Build, migrate, provision identity and start

After entering the local values:

```zsh
docker compose --env-file .env.local config -q
docker compose --env-file .env.local build app
docker compose --env-file .env.local up -d --wait postgres
docker compose --env-file .env.local run --rm -e AUTH_ENABLED=false app migrate up
docker compose --env-file .env.local run --rm -e AUTH_ENABLED=false app migrate status
```

Migration/admin commands explicitly disable authentication bootstrap; they need
PostgreSQL, not a live OIDC login. Migration version is currently **6**; `up` is
idempotent and never resets a database. Retain existing data; do not run `down`.

Enter the test user's Auth0 User ID at the local prompt (the value is not part of
shell command history). The container reuses `OIDC_ISSUER_URL` from your private
configuration so the same issuer is used for provisioning and login:

```zsh
read -r 'auth0_subject?Auth0 test user ID (auth0|...): '
AUTH0_TEST_SUBJECT="$auth0_subject" docker compose --env-file .env.local run --rm \
  -e AUTH_ENABLED=false -e AUTH0_TEST_SUBJECT --entrypoint /bin/sh app -c \
  'exec hospitality auth provision-user --issuer "$OIDC_ISSUER_URL" --subject "$AUTH0_TEST_SUBJECT" --display-name "Synthetic Auth0 Test User"'
unset auth0_subject

docker compose --env-file .env.local up -d --wait app
curl --fail --silent --show-error http://localhost:8000/healthz
curl --fail --silent --show-error http://localhost:8000/readyz
```

Provisioning prints `provisioned hospitality user id=<number> with no roles or
resource grants`. It creates/updates the local identity; it does not create the
Auth0 account. The subject is passed temporarily to the admin container and is
not needed in the server environment. Startup performs real OIDC discovery;
placeholders are insufficient. Expected probes: `{"status":"ok"}` and
`{"status":"ready"}`. After editing `.env.local`, rerun `up -d --wait app` to
apply changed container configuration; `restart` alone does not reload it.

### Browser verification — live results pending

All rows below are **pending** until the real local values are entered and each
check is actually run. Automated fake-provider tests do not verify this tenant.

| Check | Expected result on this baseline | Live result |
| --- | --- | --- |
| Anonymous `GET /auth/v1/session` | 401 `authentication required` | Pending |
| Open `http://localhost:8000/auth/v1/login` | Redirect to Auth0 hosted login; enter the test user's password there only | Pending |
| Successful callback for the provisioned subject | 303 to `/auth/v1/session`, then 200 with local user ID/display name and session-bound CSRF token; opaque HttpOnly session cookie, no OIDC tokens in app browser storage | Pending |
| Same-browser session lookup | 200 while valid; no need to copy its CSRF token anywhere | Pending |
| Zero-grant test identity, `GET /pms/v1/status` (or another PMS route) | **403 `authorization not configured`**, both signed in and signed out. Auth-enabled mode currently denies every PMS route; #51 will add permission/property-grant evaluation | Pending |
| CSRF-protected local logout | POST returns 303 to provider logout, clears local cookies and revokes the DB session | Pending |
| Follow provider logout redirect | Auth0 session ends and returns to `http://localhost:8000/`; the current app has **no home page**, so its root 404 is expected | Pending |
| Session lookup after logout | 401 in the same browser; PMS remains 403 | Pending |

The callback currently lands on session JSON, which includes a CSRF token. Keep
that response private. There is no sign-out button yet. In that same localhost
browser tab, open Developer Tools → Network (do not export it), then run this in
the Console; it keeps the CSRF token inside the function and prints no values:

```javascript
void (async () => {
  const session = await fetch('/auth/v1/session', { cache: 'no-store' });
  if (!session.ok) throw new Error('A signed-in local session is required');
  const { csrf_token } = await session.json();
  const logout = await fetch('/auth/v1/logout', {
    method: 'POST',
    headers: { 'X-CSRF-Token': csrf_token },
    credentials: 'same-origin',
    redirect: 'manual'
  });
  if (logout.type !== 'opaqueredirect') throw new Error('Logout did not redirect');
  document.title = 'Local logout accepted; follow provider Location in Network';
})();
```

`redirect: 'manual'` prevents a cross-origin fetch/CORS failure from obscuring the
local result; it **does not navigate to Auth0**. In Network, select that POST and
verify HTTP 303. Follow **only its Location response header** in the address bar
to complete provider logout; do not copy request cookies or other headers. The
server uses the discovered logout endpoint, falling back to Auth0 `/v2/logout`.
The live tenant's endpoint/return handling still needs verification; if it fails,
record only a sanitized status/reason, not full headers or redirect traces. See
[Auth0 logout allowlisting](https://auth0.com/docs/authenticate/login/logout/redirect-users-after-logout).

To inspect session/PMS HTTP status in the same browser without printing response
JSON, run either line separately (the tab title shows the result):

```javascript
void fetch('/auth/v1/session', { cache: 'no-store' }).then(r => { document.title = `Session HTTP ${r.status}`; });
void fetch('/pms/v1/status', { cache: 'no-store' }).then(r => { document.title = `PMS HTTP ${r.status}`; });
```

A new terminal `curl` has no browser cookie and will report anonymous session
status even while the browser is signed in. Do not copy cookies into curl or
save cookie jars/CSRF tokens in the repository. Stop containers without deleting
data using `docker compose --env-file .env.local stop` when finished.

### Checks that work with placeholders

```zsh
docker compose --env-file .env.example config -q
./scripts/validate.sh
go test -race ./...
./scripts/test-build.sh
```

These do not require the user's Auth0 credentials. PostgreSQL integration uses
the isolated test-database procedure in [LOCAL_SETUP.md](LOCAL_SETUP.md), never the
application database. Do not try to prove a live login by starting the enabled
server with placeholders, and do not turn off auth to bypass a failed login or
PMS denial. Record live migration/provisioning/discovery/browser checks separately.
