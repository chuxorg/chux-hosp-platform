package auth

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

type fakeProvider struct {
	identity       Identity
	exchangeErr    error
	code, verifier string
}

func (p *fakeProvider) AuthorizationURL(state, nonce, challenge string) string {
	return "https://tenant.example/authorize?state=" + url.QueryEscape(state) + "&nonce=" + url.QueryEscape(nonce) + "&code_challenge=" + url.QueryEscape(challenge)
}
func (p *fakeProvider) Exchange(_ context.Context, code, verifier string) (Identity, error) {
	p.code, p.verifier = code, verifier
	return p.identity, p.exchangeErr
}
func (p *fakeProvider) LogoutURL(returnTo string) (string, error) {
	return "https://tenant.example/logout?returnTo=" + url.QueryEscape(returnTo), nil
}

type fakeStore struct {
	login                               LoginTransaction
	loginState, nonceHash               []byte
	user                                User
	findErr, authErr                    error
	session                             Session
	createdSessionHash, createdCSRFHash []byte
	revoked                             [][]byte
}

func (s *fakeStore) CreateLogin(_ context.Context, state, nonce []byte, verifier string, _ time.Time) error {
	s.loginState, s.nonceHash = state, nonce
	s.login = LoginTransaction{NonceHash: nonce, PKCEVerifier: verifier}
	return nil
}
func (s *fakeStore) ConsumeLogin(_ context.Context, state []byte) (LoginTransaction, error) {
	if len(s.loginState) == 0 || string(state) != string(s.loginState) {
		return LoginTransaction{}, ErrNotFound
	}
	login := s.login
	s.loginState = nil
	return login, nil
}
func (s *fakeStore) FindUser(_ context.Context, _, _ string) (User, error) { return s.user, s.findErr }
func (s *fakeStore) CreateSession(_ context.Context, _ int64, session, csrf []byte, _, _ time.Time) error {
	s.createdSessionHash, s.createdCSRFHash = session, csrf
	s.session = Session{User: s.user, CSRFHash: csrf}
	return nil
}
func (s *fakeStore) Authenticate(_ context.Context, session []byte, _ time.Duration, _ bool) (Session, error) {
	if s.authErr != nil || len(s.createdSessionHash) == 0 || string(session) != string(s.createdSessionHash) {
		if s.authErr != nil {
			return Session{}, s.authErr
		}
		return Session{}, ErrNotFound
	}
	return s.session, nil
}
func (s *fakeStore) RevokeSession(_ context.Context, hash []byte) error {
	s.revoked = append(s.revoked, hash)
	return nil
}

func authTestHandler(store *fakeStore, provider *fakeProvider) http.Handler {
	return NewHandler(store, provider, Config{IssuerURL: "https://tenant.example/", LogoutReturnURL: "https://app.example/", CookieSecure: true, IdleTimeout: 8 * time.Hour, AbsoluteLifetime: 12 * time.Hour, DatabaseTimeout: time.Second})
}

func TestLoginUsesStateNonceAndPKCES256(t *testing.T) {
	store, provider := &fakeStore{}, &fakeProvider{}
	w := httptest.NewRecorder()
	authTestHandler(store, provider).ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/login", nil))
	if w.Code != http.StatusFound || len(store.loginState) != sha256.Size || len(store.nonceHash) != sha256.Size {
		t.Fatalf("login did not persist bounded transaction: %d", w.Code)
	}
	location, err := url.Parse(w.Header().Get("Location"))
	if err != nil || location.Query().Get("state") == "" || location.Query().Get("nonce") == "" || location.Query().Get("code_challenge") == "" {
		t.Fatal("authorization redirect omitted state, nonce or PKCE challenge")
	}
	if string(digest(location.Query().Get("state"))) != string(store.loginState) || string(digest(location.Query().Get("nonce"))) != string(store.nonceHash) {
		t.Fatal("redirect values do not match persisted hashes")
	}
}

func TestCallbackRequiresProvisioningRotatesSessionAndRejectsReplay(t *testing.T) {
	store, provider := &fakeStore{user: User{ID: 7, Issuer: "https://tenant.example/", Subject: "auth0|synthetic", DisplayName: "Demo Operator"}}, &fakeProvider{}
	handler := authTestHandler(store, provider)
	login := httptest.NewRecorder()
	handler.ServeHTTP(login, httptest.NewRequest(http.MethodGet, "/login", nil))
	redirect, _ := url.Parse(login.Header().Get("Location"))
	provider.identity = Identity{Issuer: "https://tenant.example/", Subject: "auth0|synthetic", Nonce: redirect.Query().Get("nonce")}
	callbackPath := "/callback?state=" + url.QueryEscape(redirect.Query().Get("state")) + "&code=one-time"
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, httptest.NewRequest(http.MethodGet, callbackPath, nil))
	if w.Code != http.StatusSeeOther || provider.code != "one-time" || provider.verifier == "" || len(w.Result().Cookies()) != 2 {
		t.Fatalf("callback failed: %d", w.Code)
	}
	cookies := w.Result().Cookies()
	if cookies[0].Name != sessionCookie || !cookies[0].HttpOnly || !cookies[0].Secure || cookies[0].SameSite != http.SameSiteLaxMode || cookies[0].Domain != "" || cookies[0].Path != "/" {
		t.Fatalf("unsafe session cookie: %#v", cookies[0])
	}
	if cookies[1].Name != csrfCookie || cookies[1].HttpOnly || !cookies[1].Secure || cookies[1].SameSite != http.SameSiteLaxMode || cookies[1].Domain != "" || cookies[1].Path != "/" {
		t.Fatalf("unsafe CSRF cookie: %#v", cookies[1])
	}
	if len(store.createdSessionHash) != sha256.Size || len(store.createdCSRFHash) != sha256.Size {
		t.Fatal("opaque session and CSRF hashes were not stored")
	}
	replay := httptest.NewRecorder()
	handler.ServeHTTP(replay, httptest.NewRequest(http.MethodGet, callbackPath, nil))
	if replay.Code != http.StatusBadRequest {
		t.Fatal("replayed callback was accepted")
	}

	unprovisionedStore := &fakeStore{findErr: ErrNotFound}
	unprovisionedProvider := &fakeProvider{}
	unprovisioned := authTestHandler(unprovisionedStore, unprovisionedProvider)
	l := httptest.NewRecorder()
	unprovisioned.ServeHTTP(l, httptest.NewRequest(http.MethodGet, "/login", nil))
	u, _ := url.Parse(l.Header().Get("Location"))
	unprovisionedProvider.identity = Identity{Issuer: "https://tenant.example/", Subject: "unknown", Nonce: u.Query().Get("nonce")}
	denied := httptest.NewRecorder()
	unprovisioned.ServeHTTP(denied, httptest.NewRequest(http.MethodGet, "/callback?state="+url.QueryEscape(u.Query().Get("state"))+"&code=x", nil))
	if denied.Code != http.StatusForbidden {
		t.Fatal("unprovisioned identity was accepted")
	}
}

func TestSessionAndLogoutRequireValidCookiesCSRFAndOrigin(t *testing.T) {
	sessionRaw := base64.RawURLEncoding.EncodeToString([]byte("synthetic-session-material-32bytes"))
	csrfRaw := base64.RawURLEncoding.EncodeToString([]byte("synthetic-csrf-material-32-bytes"))
	store := &fakeStore{user: User{ID: 9, DisplayName: "Front Desk"}, createdSessionHash: digest(sessionRaw), createdCSRFHash: digest(csrfRaw)}
	store.session = Session{User: store.user, CSRFHash: digest(csrfRaw)}
	handler := authTestHandler(store, &fakeProvider{})

	request := httptest.NewRequest(http.MethodGet, "/session", nil)
	request.AddCookie(&http.Cookie{Name: sessionCookie, Value: sessionRaw})
	request.AddCookie(&http.Cookie{Name: csrfCookie, Value: csrfRaw})
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, request)
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), "Front Desk") || !strings.Contains(w.Body.String(), csrfRaw) {
		t.Fatalf("session lookup failed: %d %s", w.Code, w.Body.String())
	}

	bad := httptest.NewRequest(http.MethodPost, "/logout", nil)
	bad.Host = "app.example"
	bad.Header.Set("Origin", "https://evil.example")
	bad.Header.Set("X-CSRF-Token", csrfRaw)
	bad.AddCookie(&http.Cookie{Name: sessionCookie, Value: sessionRaw})
	bad.AddCookie(&http.Cookie{Name: csrfCookie, Value: csrfRaw})
	denied := httptest.NewRecorder()
	handler.ServeHTTP(denied, bad)
	if denied.Code != http.StatusForbidden {
		t.Fatal("cross-origin logout was accepted")
	}

	good := httptest.NewRequest(http.MethodPost, "/logout", nil)
	good.Host = "app.example"
	good.Header.Set("Origin", "https://app.example")
	good.Header.Set("X-CSRF-Token", csrfRaw)
	good.AddCookie(&http.Cookie{Name: sessionCookie, Value: sessionRaw})
	good.AddCookie(&http.Cookie{Name: csrfCookie, Value: csrfRaw})
	loggedOut := httptest.NewRecorder()
	handler.ServeHTTP(loggedOut, good)
	if loggedOut.Code != http.StatusSeeOther || len(store.revoked) != 1 || !strings.HasPrefix(loggedOut.Header().Get("Location"), "https://tenant.example/logout") {
		t.Fatal("valid logout failed")
	}
}

func TestSessionStoreFailureFailsClosed(t *testing.T) {
	store := &fakeStore{authErr: errors.New("database offline")}
	r := httptest.NewRequest(http.MethodGet, "/session", nil)
	r.AddCookie(&http.Cookie{Name: sessionCookie, Value: "opaque"})
	r.AddCookie(&http.Cookie{Name: csrfCookie, Value: "csrf"})
	w := httptest.NewRecorder()
	authTestHandler(store, &fakeProvider{}).ServeHTTP(w, r)
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("database failure returned %d", w.Code)
	}
}
