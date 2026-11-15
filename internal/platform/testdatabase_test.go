package platform

import (
	"errors"
	"net/url"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
)

// Only used by tests. The supplied DB is a provisioning endpoint, never reset.
func testDatabaseConfig(raw, appURL string) (*pgx.ConnConfig, error) {
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "postgres" && u.Scheme != "postgresql") || u.Hostname() == "" {
		return nil, errors.New("TEST_DATABASE_URL must be an explicit PostgreSQL URL")
	}
	cfg, err := pgx.ParseConfig(raw)
	if err != nil {
		return nil, errors.New("invalid test database configuration")
	}
	if !strings.HasSuffix(cfg.Database, "_test") || cfg.Database == "_test" {
		return nil, errors.New("TEST_DATABASE_URL database must end in _test")
	}
	if appURL != "" {
		app, err := pgx.ParseConfig(appURL)
		if err != nil {
			return nil, errors.New("invalid application database configuration")
		}
		if cfg.Database == app.Database && cfg.Host == app.Host && cfg.Port == app.Port {
			return nil, errors.New("test and application databases must differ")
		}
	}
	return cfg, nil
}

func TestIntegrationDatabaseSafeguards(t *testing.T) {
	for _, tt := range []struct {
		name, url, app string
		bad            bool
	}{
		{"dedicated", "postgres://localhost/hospitality_test", "", false},
		{"missing", "", "", true},
		{"production", "postgres://localhost/hospitality", "", true},
		{"override", "postgres://localhost/hospitality_test?dbname=production", "", true},
		{"same application", "postgres://localhost/hospitality_test", "postgresql://localhost/hospitality_test", true},
		{"distinct database", "postgres://localhost/hospitality_test", "postgres://localhost/hospitality", false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			_, err := testDatabaseConfig(tt.url, tt.app)
			if (err != nil) != tt.bad {
				t.Fatalf("unexpected guard result: %v", err)
			}
		})
	}
}
