-- +goose Up
CREATE TABLE platform_users (
    id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    oidc_issuer VARCHAR(500) NOT NULL,
    oidc_subject VARCHAR(500) NOT NULL,
    display_name VARCHAR(200) NOT NULL,
    provisioned_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
    disabled_at TIMESTAMPTZ,
    UNIQUE (oidc_issuer, oidc_subject),
    CHECK (btrim(oidc_issuer) <> '' AND btrim(oidc_subject) <> '' AND btrim(display_name) <> '')
);

CREATE TABLE platform_oidc_login_transactions (
    state_hash BYTEA PRIMARY KEY CHECK (octet_length(state_hash) = 32),
    nonce_hash BYTEA NOT NULL CHECK (octet_length(nonce_hash) = 32),
    pkce_verifier VARCHAR(128) NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
    expires_at TIMESTAMPTZ NOT NULL,
    CHECK (expires_at > created_at)
);

CREATE TABLE platform_sessions (
    id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    user_id BIGINT NOT NULL REFERENCES platform_users(id),
    session_hash BYTEA NOT NULL UNIQUE CHECK (octet_length(session_hash) = 32),
    csrf_hash BYTEA NOT NULL CHECK (octet_length(csrf_hash) = 32),
    created_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
    last_seen_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
    idle_expires_at TIMESTAMPTZ NOT NULL,
    absolute_expires_at TIMESTAMPTZ NOT NULL,
    revoked_at TIMESTAMPTZ,
    CHECK (idle_expires_at > created_at AND absolute_expires_at > created_at),
    CHECK (idle_expires_at <= absolute_expires_at)
);

CREATE INDEX platform_sessions_user_active_idx ON platform_sessions(user_id, absolute_expires_at) WHERE revoked_at IS NULL;
CREATE INDEX platform_login_expiry_idx ON platform_oidc_login_transactions(expires_at);

-- +goose Down
LOCK TABLE platform_users, platform_sessions, platform_oidc_login_transactions IN ACCESS EXCLUSIVE MODE;
-- +goose StatementBegin
DO $$ BEGIN
    IF EXISTS (SELECT 1 FROM platform_users) THEN
        RAISE EXCEPTION 'rollback refused: provisioned identity and session history must be preserved';
    END IF;
END $$;
-- +goose StatementEnd
DROP TABLE platform_sessions;
DROP TABLE platform_oidc_login_transactions;
DROP TABLE platform_users;
