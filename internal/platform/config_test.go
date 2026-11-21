package platform

import (
	"strings"
	"testing"
	"time"
)

func TestLoadConfig(t *testing.T) {
	for _, tt := range []struct {
		name string
		env  map[string]string
		bad  bool
	}{
		{"defaults", map[string]string{"DATABASE_URL": "postgres://user:secret@localhost/hospitality"}, false},
		{"explicit", map[string]string{"DATABASE_URL": "postgres://localhost/hospitality", "HTTP_ADDR": "0.0.0.0:9000", "DATABASE_TIMEOUT": "500ms"}, false},
		{"missing database", map[string]string{}, true},
		{"SQLite refused", map[string]string{"DATABASE_URL": "sqlite:///tmp/test"}, true},
		{"implicit database refused", map[string]string{"DATABASE_URL": "postgres://localhost/"}, true},
		{"bad database option", map[string]string{"DATABASE_URL": "postgres://user:secret@localhost/db?sslmode=invalid"}, true},
		{"missing port", map[string]string{"DATABASE_URL": "postgres://localhost/db", "HTTP_ADDR": "localhost"}, true},
		{"invalid port", map[string]string{"DATABASE_URL": "postgres://localhost/db", "HTTP_ADDR": "localhost:65536"}, true},
		{"negative timeout", map[string]string{"DATABASE_URL": "postgres://localhost/db", "DATABASE_TIMEOUT": "-1s"}, true},
		{"unbounded timeout", map[string]string{"DATABASE_URL": "postgres://localhost/db", "DATABASE_TIMEOUT": "31s"}, true},
		{"zero timeout", map[string]string{"DATABASE_URL": "postgres://localhost/db", "DATABASE_TIMEOUT": "0s"}, true},
		{"invalid timeout", map[string]string{"DATABASE_URL": "postgres://localhost/db", "DATABASE_TIMEOUT": "forever"}, true},
		{"auth defaults", map[string]string{"DATABASE_URL": "postgres://localhost/db", "AUTH_ENABLED": "true", "OIDC_ISSUER_URL": "https://tenant.example/", "OIDC_CLIENT_ID": "client", "OIDC_CLIENT_SECRET": "secret", "OIDC_REDIRECT_URL": "https://app.example/auth/v1/callback", "OIDC_LOGOUT_RETURN_URL": "https://app.example/"}, false},
		{"auth missing secret", map[string]string{"DATABASE_URL": "postgres://localhost/db", "AUTH_ENABLED": "true", "OIDC_ISSUER_URL": "https://tenant.example/", "OIDC_CLIENT_ID": "client", "OIDC_REDIRECT_URL": "https://app.example/auth/v1/callback", "OIDC_LOGOUT_RETURN_URL": "https://app.example/"}, true},
		{"auth insecure deployed redirect", map[string]string{"DATABASE_URL": "postgres://localhost/db", "AUTH_ENABLED": "true", "OIDC_ISSUER_URL": "https://tenant.example/", "OIDC_CLIENT_ID": "client", "OIDC_CLIENT_SECRET": "secret", "OIDC_REDIRECT_URL": "http://localhost:8000/auth/v1/callback", "OIDC_LOGOUT_RETURN_URL": "http://localhost:8000/"}, true},
		{"auth explicit local HTTP", map[string]string{"DATABASE_URL": "postgres://localhost/db", "AUTH_ENABLED": "true", "AUTH_ALLOW_INSECURE_HTTP": "true", "OIDC_ISSUER_URL": "https://tenant.example/", "OIDC_CLIENT_ID": "client", "OIDC_CLIENT_SECRET": "secret", "OIDC_REDIRECT_URL": "http://localhost:8000/auth/v1/callback", "OIDC_LOGOUT_RETURN_URL": "http://localhost:8000/"}, false},
		{"auth invalid lifetime", map[string]string{"DATABASE_URL": "postgres://localhost/db", "AUTH_ENABLED": "true", "OIDC_ISSUER_URL": "https://tenant.example/", "OIDC_CLIENT_ID": "client", "OIDC_CLIENT_SECRET": "secret", "OIDC_REDIRECT_URL": "https://app.example/auth/v1/callback", "OIDC_LOGOUT_RETURN_URL": "https://app.example/", "AUTH_SESSION_IDLE_TIMEOUT": "13h", "AUTH_SESSION_ABSOLUTE_LIFETIME": "12h"}, true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			c, err := LoadConfig(func(k string) string { return tt.env[k] })
			if (err != nil) != tt.bad {
				t.Fatalf("unexpected config result: %v", err)
			}
			if err != nil {
				if strings.Contains(err.Error(), "secret") {
					t.Fatal("configuration leaked credentials")
				}
				return
			}
			pc, err := PoolConfig(c)
			if err != nil {
				t.Fatal(err)
			}
			if pc.MaxConns != 10 || pc.ConnConfig.ConnectTimeout != c.DatabaseTimeout {
				t.Fatal("pool limits/timeouts not applied")
			}
			if tt.name == "defaults" && (c.HTTPAddr != "127.0.0.1:8000" || c.DatabaseTimeout != 2*time.Second) {
				t.Fatal("unexpected defaults")
			}
			if tt.name == "auth defaults" && (!c.Auth.Enabled || !c.Auth.CookieSecure || c.Auth.SessionIdleTimeout != 8*time.Hour || c.Auth.SessionAbsoluteLife != 12*time.Hour) {
				t.Fatal("unexpected authentication defaults")
			}
		})
	}
}
