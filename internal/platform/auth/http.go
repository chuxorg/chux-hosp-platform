package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"time"
)

const (
	sessionCookie = "hospitality_session"
	csrfCookie    = "hospitality_csrf"
)

type Config struct {
	IssuerURL        string
	LogoutReturnURL  string
	CookieSecure     bool
	IdleTimeout      time.Duration
	AbsoluteLifetime time.Duration
	DatabaseTimeout  time.Duration
}

type AuthStore interface {
	CreateLogin(context.Context, []byte, []byte, string, time.Time) error
	ConsumeLogin(context.Context, []byte) (LoginTransaction, error)
	FindUser(context.Context, string, string) (User, error)
	CreateSession(context.Context, int64, []byte, []byte, time.Time, time.Time) error
	Authenticate(context.Context, []byte, time.Duration, bool) (Session, error)
	RevokeSession(context.Context, []byte) error
}

type Handler struct {
	routes   http.Handler
	store    AuthStore
	provider IdentityProvider
	config   Config
	now      func() time.Time
}

func NewHandler(store AuthStore, provider IdentityProvider, config Config) *Handler {
	h := &Handler{store: store, provider: provider, config: config, now: time.Now}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /login", h.login)
	mux.HandleFunc("GET /callback", h.callback)
	mux.HandleFunc("GET /session", h.currentSession)
	mux.HandleFunc("POST /logout", h.logout)
	h.routes = securityHeaders(mux)
	return h
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h.routes.ServeHTTP(w, r)
}

func (h *Handler) login(w http.ResponseWriter, r *http.Request) {
	state, err := randomToken(32)
	if err != nil {
		h.problem(w, http.StatusServiceUnavailable, "authentication unavailable")
		return
	}
	nonce, err := randomToken(32)
	if err != nil {
		h.problem(w, http.StatusServiceUnavailable, "authentication unavailable")
		return
	}
	verifier, err := randomToken(48)
	if err != nil {
		h.problem(w, http.StatusServiceUnavailable, "authentication unavailable")
		return
	}
	challenge := sha256.Sum256([]byte(verifier))
	ctx, cancel := context.WithTimeout(r.Context(), h.config.DatabaseTimeout)
	defer cancel()
	if err := h.store.CreateLogin(ctx, digest(state), digest(nonce), verifier, h.now().Add(10*time.Minute)); err != nil {
		h.problem(w, http.StatusServiceUnavailable, "authentication unavailable")
		return
	}
	http.Redirect(w, r, h.provider.AuthorizationURL(state, nonce, base64.RawURLEncoding.EncodeToString(challenge[:])), http.StatusFound)
}

func (h *Handler) callback(w http.ResponseWriter, r *http.Request) {
	state, code := r.URL.Query().Get("state"), r.URL.Query().Get("code")
	if state == "" || len(state) > 512 || code == "" || len(code) > 4096 || r.URL.Query().Get("error") != "" {
		h.problem(w, http.StatusBadRequest, "invalid authentication callback")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), h.config.DatabaseTimeout)
	login, err := h.store.ConsumeLogin(ctx, digest(state))
	cancel()
	if err != nil {
		h.problem(w, http.StatusBadRequest, "invalid or expired authentication callback")
		return
	}
	exchangeCtx, exchangeCancel := context.WithTimeout(r.Context(), 10*time.Second)
	identity, err := h.provider.Exchange(exchangeCtx, code, login.PKCEVerifier)
	exchangeCancel()
	if err != nil || subtle.ConstantTimeCompare(digest(identity.Nonce), login.NonceHash) != 1 || identity.Issuer != h.config.IssuerURL {
		h.problem(w, http.StatusBadRequest, "invalid authentication callback")
		return
	}
	databaseCtx, databaseCancel := context.WithTimeout(r.Context(), h.config.DatabaseTimeout)
	defer databaseCancel()
	user, err := h.store.FindUser(databaseCtx, identity.Issuer, identity.Subject)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			h.problem(w, http.StatusForbidden, "identity is not provisioned")
		} else {
			h.problem(w, http.StatusServiceUnavailable, "authentication unavailable")
		}
		return
	}
	if old, err := r.Cookie(sessionCookie); err == nil {
		_ = h.store.RevokeSession(databaseCtx, digest(old.Value))
	}
	sessionID, err := randomToken(32)
	if err != nil {
		h.problem(w, http.StatusServiceUnavailable, "authentication unavailable")
		return
	}
	csrf, err := randomToken(32)
	if err != nil {
		h.problem(w, http.StatusServiceUnavailable, "authentication unavailable")
		return
	}
	now := h.now()
	if err := h.store.CreateSession(databaseCtx, user.ID, digest(sessionID), digest(csrf), now.Add(h.config.IdleTimeout), now.Add(h.config.AbsoluteLifetime)); err != nil {
		h.problem(w, http.StatusServiceUnavailable, "authentication unavailable")
		return
	}
	http.SetCookie(w, &http.Cookie{Name: sessionCookie, Value: sessionID, Path: "/", HttpOnly: true, Secure: h.config.CookieSecure, SameSite: http.SameSiteLaxMode, MaxAge: int(h.config.AbsoluteLifetime.Seconds())})
	http.SetCookie(w, &http.Cookie{Name: csrfCookie, Value: csrf, Path: "/", HttpOnly: false, Secure: h.config.CookieSecure, SameSite: http.SameSiteLaxMode, MaxAge: int(h.config.AbsoluteLifetime.Seconds())})
	http.Redirect(w, r, "/auth/v1/session", http.StatusSeeOther)
}

func (h *Handler) currentSession(w http.ResponseWriter, r *http.Request) {
	session, csrf, ok := h.authenticate(w, r, true)
	if !ok {
		return
	}
	h.writeJSON(w, http.StatusOK, map[string]any{"user": map[string]any{"id": session.ID, "display_name": session.DisplayName}, "csrf_token": csrf})
}

func (h *Handler) logout(w http.ResponseWriter, r *http.Request) {
	session, csrfCookieToken, ok := h.authenticate(w, r, false)
	if !ok {
		return
	}
	csrf := r.Header.Get("X-CSRF-Token")
	cookie, _ := r.Cookie(sessionCookie)
	if csrf == "" || subtle.ConstantTimeCompare([]byte(csrf), []byte(csrfCookieToken)) != 1 || subtle.ConstantTimeCompare(digest(csrf), session.CSRFHash) != 1 || !validOrigin(r) {
		h.problem(w, http.StatusForbidden, "CSRF validation failed")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), h.config.DatabaseTimeout)
	defer cancel()
	if err := h.store.RevokeSession(ctx, digest(cookie.Value)); err != nil {
		h.problem(w, http.StatusServiceUnavailable, "authentication unavailable")
		return
	}
	http.SetCookie(w, &http.Cookie{Name: sessionCookie, Value: "", Path: "/", HttpOnly: true, Secure: h.config.CookieSecure, SameSite: http.SameSiteLaxMode, MaxAge: -1})
	http.SetCookie(w, &http.Cookie{Name: csrfCookie, Value: "", Path: "/", HttpOnly: false, Secure: h.config.CookieSecure, SameSite: http.SameSiteLaxMode, MaxAge: -1})
	logoutURL, err := h.provider.LogoutURL(h.config.LogoutReturnURL)
	if err != nil {
		h.problem(w, http.StatusServiceUnavailable, "logout unavailable")
		return
	}
	http.Redirect(w, r, logoutURL, http.StatusSeeOther)
}

func (h *Handler) authenticate(w http.ResponseWriter, r *http.Request, extendIdle bool) (Session, string, bool) {
	cookie, err := r.Cookie(sessionCookie)
	if err != nil || cookie.Value == "" {
		h.problem(w, http.StatusUnauthorized, "authentication required")
		return Session{}, "", false
	}
	ctx, cancel := context.WithTimeout(r.Context(), h.config.DatabaseTimeout)
	defer cancel()
	session, err := h.store.Authenticate(ctx, digest(cookie.Value), h.config.IdleTimeout, extendIdle)
	if err != nil {
		clearCookie(w, h.config.CookieSecure)
		if errors.Is(err, ErrNotFound) {
			h.problem(w, http.StatusUnauthorized, "authentication required")
		} else {
			h.problem(w, http.StatusServiceUnavailable, "authentication unavailable")
		}
		return Session{}, "", false
	}
	csrf, err := r.Cookie(csrfCookie)
	if err != nil || subtle.ConstantTimeCompare(digest(csrf.Value), session.CSRFHash) != 1 {
		clearCookie(w, h.config.CookieSecure)
		h.problem(w, http.StatusUnauthorized, "authentication required")
		return Session{}, "", false
	}
	return session, csrf.Value, true
}

func validOrigin(r *http.Request) bool {
	origin := r.Header.Get("Origin")
	if origin == "" {
		return true
	}
	return origin == "https://"+r.Host || origin == "http://"+r.Host
}

func randomToken(bytes int) (string, error) {
	b := make([]byte, bytes)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

func digest(value string) []byte {
	sum := sha256.Sum256([]byte(value))
	return sum[:]
}

func clearCookie(w http.ResponseWriter, secure bool) {
	http.SetCookie(w, &http.Cookie{Name: sessionCookie, Value: "", Path: "/", HttpOnly: true, Secure: secure, SameSite: http.SameSiteLaxMode, MaxAge: -1})
	http.SetCookie(w, &http.Cookie{Name: csrfCookie, Value: "", Path: "/", HttpOnly: false, Secure: secure, SameSite: http.SameSiteLaxMode, MaxAge: -1})
}

func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		next.ServeHTTP(w, r)
	})
}

func (h *Handler) problem(w http.ResponseWriter, status int, detail string) {
	h.writeJSON(w, status, map[string]string{"error": detail})
}

func (h *Handler) writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
