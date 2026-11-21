package auth

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var ErrNotFound = errors.New("authentication record not found")

type User struct {
	ID          int64
	Issuer      string
	Subject     string
	DisplayName string
}

type LoginTransaction struct {
	NonceHash    []byte
	PKCEVerifier string
}

type Session struct {
	User
	CSRFHash []byte
}

type Store struct{ pool *pgxpool.Pool }

func NewStore(pool *pgxpool.Pool) *Store { return &Store{pool: pool} }

func (s *Store) CreateLogin(ctx context.Context, stateHash, nonceHash []byte, verifier string, expiresAt time.Time) error {
	_, err := s.pool.Exec(ctx, `INSERT INTO platform_oidc_login_transactions(state_hash,nonce_hash,pkce_verifier,expires_at) VALUES($1,$2,$3,$4)`, stateHash, nonceHash, verifier, expiresAt)
	return err
}

func (s *Store) ConsumeLogin(ctx context.Context, stateHash []byte) (LoginTransaction, error) {
	var tx LoginTransaction
	err := s.pool.QueryRow(ctx, `DELETE FROM platform_oidc_login_transactions WHERE state_hash=$1 AND expires_at>clock_timestamp() RETURNING nonce_hash,pkce_verifier`, stateHash).Scan(&tx.NonceHash, &tx.PKCEVerifier)
	if errors.Is(err, pgx.ErrNoRows) {
		return LoginTransaction{}, ErrNotFound
	}
	return tx, err
}

func (s *Store) FindUser(ctx context.Context, issuer, subject string) (User, error) {
	var u User
	err := s.pool.QueryRow(ctx, `SELECT id,oidc_issuer,oidc_subject,display_name FROM platform_users WHERE oidc_issuer=$1 AND oidc_subject=$2 AND disabled_at IS NULL`, issuer, subject).Scan(&u.ID, &u.Issuer, &u.Subject, &u.DisplayName)
	if errors.Is(err, pgx.ErrNoRows) {
		return User{}, ErrNotFound
	}
	return u, err
}

func (s *Store) CreateSession(ctx context.Context, userID int64, sessionHash, csrfHash []byte, idleExpiresAt, absoluteExpiresAt time.Time) error {
	_, err := s.pool.Exec(ctx, `INSERT INTO platform_sessions(user_id,session_hash,csrf_hash,idle_expires_at,absolute_expires_at) VALUES($1,$2,$3,$4,$5)`, userID, sessionHash, csrfHash, idleExpiresAt, absoluteExpiresAt)
	return err
}

func (s *Store) Authenticate(ctx context.Context, sessionHash []byte, idle time.Duration, extendIdle bool) (Session, error) {
	var session Session
	err := s.pool.QueryRow(ctx, `
UPDATE platform_sessions s SET
  last_seen_at=CASE WHEN $3 THEN clock_timestamp() ELSE s.last_seen_at END,
  idle_expires_at=CASE WHEN $3 THEN LEAST(clock_timestamp()+($2 * interval '1 second'),s.absolute_expires_at) ELSE s.idle_expires_at END
FROM platform_users u
WHERE s.session_hash=$1 AND s.user_id=u.id AND s.revoked_at IS NULL AND u.disabled_at IS NULL
  AND s.idle_expires_at>clock_timestamp() AND s.absolute_expires_at>clock_timestamp()
RETURNING u.id,u.oidc_issuer,u.oidc_subject,u.display_name,s.csrf_hash`, sessionHash, int64(idle/time.Second), extendIdle).Scan(&session.ID, &session.Issuer, &session.Subject, &session.DisplayName, &session.CSRFHash)
	if errors.Is(err, pgx.ErrNoRows) {
		return Session{}, ErrNotFound
	}
	return session, err
}

func (s *Store) RevokeSession(ctx context.Context, sessionHash []byte) error {
	_, err := s.pool.Exec(ctx, `UPDATE platform_sessions SET revoked_at=COALESCE(revoked_at,clock_timestamp()) WHERE session_hash=$1`, sessionHash)
	return err
}

func (s *Store) ProvisionUser(ctx context.Context, issuer, subject, displayName string) (int64, error) {
	var id int64
	err := s.pool.QueryRow(ctx, `
INSERT INTO platform_users(oidc_issuer,oidc_subject,display_name) VALUES($1,$2,$3)
ON CONFLICT(oidc_issuer,oidc_subject) DO UPDATE SET display_name=EXCLUDED.display_name,disabled_at=NULL
RETURNING id`, issuer, subject, displayName).Scan(&id)
	return id, err
}

func (s *Store) RevokeUser(ctx context.Context, issuer, subject string) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var id int64
	if err := tx.QueryRow(ctx, `UPDATE platform_users SET disabled_at=COALESCE(disabled_at,clock_timestamp()) WHERE oidc_issuer=$1 AND oidc_subject=$2 RETURNING id`, issuer, subject).Scan(&id); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotFound
		}
		return err
	}
	if _, err := tx.Exec(ctx, `UPDATE platform_sessions SET revoked_at=COALESCE(revoked_at,clock_timestamp()) WHERE user_id=$1`, id); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (s *Store) Cleanup(ctx context.Context) error {
	if _, err := s.pool.Exec(ctx, `DELETE FROM platform_oidc_login_transactions WHERE expires_at<=clock_timestamp()`); err != nil {
		return err
	}
	_, err := s.pool.Exec(ctx, `DELETE FROM platform_sessions WHERE absolute_expires_at<clock_timestamp()-interval '7 days'`)
	return err
}
