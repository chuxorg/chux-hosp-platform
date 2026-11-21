package auth

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/go-jose/go-jose/v4"
)

func TestOIDCProviderVerifiesTokenAndPKCEExchange(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	wrongKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	claims := map[string]any{}
	useWrongKey := false
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/.well-known/openid-configuration":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"issuer": server.URL, "authorization_endpoint": server.URL + "/authorize",
				"token_endpoint": server.URL + "/token", "jwks_uri": server.URL + "/keys",
				"end_session_endpoint":                  server.URL + "/logout",
				"id_token_signing_alg_values_supported": []string{"RS256"},
			})
		case "/keys":
			_ = json.NewEncoder(w).Encode(map[string]any{"keys": []jose.JSONWebKey{{Key: &key.PublicKey, KeyID: "test", Algorithm: "RS256", Use: "sig"}}})
		case "/token":
			if err := r.ParseForm(); err != nil || r.Form.Get("code") != "valid-code" || r.Form.Get("code_verifier") != "valid-verifier" {
				http.Error(w, "invalid exchange", http.StatusBadRequest)
				return
			}
			signingKey := key
			if useWrongKey {
				signingKey = wrongKey
			}
			signer, err := jose.NewSigner(jose.SigningKey{Algorithm: jose.RS256, Key: jose.JSONWebKey{Key: signingKey, KeyID: "test"}}, nil)
			if err != nil {
				t.Error(err)
				return
			}
			payload, _ := json.Marshal(claims)
			signed, err := signer.Sign(payload)
			if err != nil {
				t.Error(err)
				return
			}
			raw, err := signed.CompactSerialize()
			if err != nil {
				t.Error(err)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"access_token": "unused", "token_type": "Bearer", "id_token": raw})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	provider, err := NewOIDCProvider(context.Background(), server.URL, "hospitality-client", "synthetic-secret", server.URL+"/callback")
	if err != nil {
		t.Fatal(err)
	}
	valid := map[string]any{"iss": server.URL, "sub": "auth0|synthetic", "aud": "hospitality-client", "exp": time.Now().Add(time.Minute).Unix(), "iat": time.Now().Add(-time.Minute).Unix(), "nonce": "nonce-value"}
	for k, v := range valid {
		claims[k] = v
	}
	identity, err := provider.Exchange(context.Background(), "valid-code", "valid-verifier")
	if err != nil || identity.Subject != "auth0|synthetic" || identity.Nonce != "nonce-value" {
		t.Fatalf("valid token rejected: %#v %v", identity, err)
	}

	tests := []struct {
		name   string
		mutate func()
	}{
		{"wrong audience", func() { claims["aud"] = "different-client" }},
		{"expired", func() { claims["exp"] = time.Now().Add(-time.Minute).Unix() }},
		{"wrong issuer", func() { claims["iss"] = "https://wrong.example/" }},
		{"invalid signature", func() { useWrongKey = true }},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			for k := range claims {
				delete(claims, k)
			}
			for k, v := range valid {
				claims[k] = v
			}
			useWrongKey = false
			tc.mutate()
			if _, err := provider.Exchange(context.Background(), "valid-code", "valid-verifier"); err == nil {
				t.Fatal("invalid ID token accepted")
			}
		})
	}
	if _, err := provider.Exchange(context.Background(), "valid-code", "wrong-verifier"); err == nil {
		t.Fatal("invalid PKCE verifier accepted")
	}
}

// ADR-0005 / #54: discovery uses OIDC logout semantics even at a custom path;
// the missing-metadata Auth0 fallback alone uses legacy returnTo.
func TestOIDCLogoutReturnParameters(t *testing.T) {
	for _, tc := range []struct{ name, endpointPath, wantPath, redirectKey, absentKey string }{
		{"discovered Auth0 OIDC", "/oidc/logout?preserved=value&returnTo=stale", "/oidc/logout", "post_logout_redirect_uri", "returnTo"},
		{"discovered custom endpoint", "/custom/end-session", "/custom/end-session", "post_logout_redirect_uri", "returnTo"},
		{"missing endpoint legacy fallback", "", "/v2/logout", "returnTo", "post_logout_redirect_uri"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var server *httptest.Server
			server = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/.well-known/openid-configuration" {
					http.NotFound(w, r)
					return
				}
				metadata := map[string]any{"issuer": server.URL, "authorization_endpoint": server.URL + "/authorize", "token_endpoint": server.URL + "/token", "jwks_uri": server.URL + "/keys"}
				if tc.endpointPath != "" {
					metadata["end_session_endpoint"] = server.URL + tc.endpointPath
				}
				_ = json.NewEncoder(w).Encode(metadata)
			}))
			defer server.Close()
			ctx := oidc.ClientContext(context.Background(), server.Client())
			p, err := NewOIDCProvider(ctx, server.URL, "synthetic-client", "synthetic-secret", server.URL+"/callback")
			if err != nil {
				t.Fatal(err)
			}
			target := "http://localhost:8000/?source=logout&next=%2F"
			raw, err := p.LogoutURL(target)
			if err != nil {
				t.Fatal(err)
			}
			got, err := url.Parse(raw)
			if err != nil {
				t.Fatal(err)
			}
			if got.Path != tc.wantPath || got.Query().Get("client_id") != "synthetic-client" || got.Query().Get(tc.redirectKey) != target || got.Query().Has(tc.absentKey) {
				t.Fatalf("incorrect logout parameters: %s", raw)
			}
			if tc.name == "discovered Auth0 OIDC" && got.Query().Get("preserved") != "value" {
				t.Fatal("lost existing endpoint parameter")
			}
		})
	}
}
