package platform

import (
	"errors"
	"net"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Config contains process settings. DatabaseURL is sensitive: never log it.
type Config struct {
	HTTPAddr        string
	DatabaseURL     string
	DatabaseTimeout time.Duration
	Auth            AuthConfig
}

// AuthConfig is disabled unless AUTH_ENABLED is explicitly true. Secrets must
// never be logged or returned by diagnostics.
type AuthConfig struct {
	Enabled             bool
	IssuerURL           string
	ClientID            string
	ClientSecret        string
	RedirectURL         string
	LogoutReturnURL     string
	CookieSecure        bool
	SessionIdleTimeout  time.Duration
	SessionAbsoluteLife time.Duration
}

func LoadConfig(getenv func(string) string) (Config, error) {
	c := Config{HTTPAddr: getenv("HTTP_ADDR"), DatabaseURL: getenv("DATABASE_URL"), DatabaseTimeout: 2 * time.Second}
	if c.HTTPAddr == "" {
		c.HTTPAddr = "127.0.0.1:8000"
	}
	_, port, err := net.SplitHostPort(c.HTTPAddr)
	if err != nil {
		return Config{}, errors.New("HTTP_ADDR must be host:port")
	}
	n, err := strconv.Atoi(port)
	if err != nil || n < 1 || n > 65535 {
		return Config{}, errors.New("HTTP_ADDR port must be 1–65535")
	}
	if raw := getenv("DATABASE_TIMEOUT"); raw != "" {
		c.DatabaseTimeout, err = time.ParseDuration(raw)
		if err != nil || c.DatabaseTimeout <= 0 || c.DatabaseTimeout > 30*time.Second {
			return Config{}, errors.New("DATABASE_TIMEOUT must be greater than zero and at most 30s")
		}
	}
	if err := loadAuthConfig(&c.Auth, getenv); err != nil {
		return Config{}, err
	}
	if _, err := PoolConfig(c); err != nil {
		return Config{}, err
	}
	return c, nil
}

func loadAuthConfig(c *AuthConfig, getenv func(string) string) error {
	rawEnabled := strings.TrimSpace(getenv("AUTH_ENABLED"))
	if rawEnabled == "" || rawEnabled == "false" {
		return nil
	}
	if rawEnabled != "true" {
		return errors.New("AUTH_ENABLED must be true or false")
	}
	c.Enabled = true
	c.CookieSecure = true
	c.SessionIdleTimeout = 8 * time.Hour
	c.SessionAbsoluteLife = 12 * time.Hour
	c.IssuerURL = strings.TrimSpace(getenv("OIDC_ISSUER_URL"))
	c.ClientID = strings.TrimSpace(getenv("OIDC_CLIENT_ID"))
	c.ClientSecret = getenv("OIDC_CLIENT_SECRET")
	c.RedirectURL = strings.TrimSpace(getenv("OIDC_REDIRECT_URL"))
	c.LogoutReturnURL = strings.TrimSpace(getenv("OIDC_LOGOUT_RETURN_URL"))
	if c.IssuerURL == "" || c.ClientID == "" || c.ClientSecret == "" || c.RedirectURL == "" || c.LogoutReturnURL == "" {
		return errors.New("authentication settings are incomplete")
	}
	allowHTTP := strings.TrimSpace(getenv("AUTH_ALLOW_INSECURE_HTTP"))
	if allowHTTP != "" && allowHTTP != "true" && allowHTTP != "false" {
		return errors.New("AUTH_ALLOW_INSECURE_HTTP must be true or false")
	}
	if allowHTTP == "true" {
		c.CookieSecure = false
	}
	issuer, err := url.Parse(c.IssuerURL)
	if err != nil || issuer.Scheme != "https" || issuer.Host == "" || issuer.RawQuery != "" || issuer.Fragment != "" {
		return errors.New("OIDC_ISSUER_URL must be an HTTPS URL")
	}
	for name, raw := range map[string]string{"OIDC_REDIRECT_URL": c.RedirectURL, "OIDC_LOGOUT_RETURN_URL": c.LogoutReturnURL} {
		u, parseErr := url.Parse(raw)
		if parseErr != nil || u.Host == "" || (u.Scheme != "https" && !(allowHTTP == "true" && u.Scheme == "http" && (u.Hostname() == "localhost" || u.Hostname() == "127.0.0.1"))) {
			return errors.New(name + " must be HTTPS (or localhost HTTP when explicitly enabled)")
		}
	}
	if err := parseAuthDuration(getenv("AUTH_SESSION_IDLE_TIMEOUT"), &c.SessionIdleTimeout, 5*time.Minute, 12*time.Hour, "AUTH_SESSION_IDLE_TIMEOUT"); err != nil {
		return err
	}
	if err := parseAuthDuration(getenv("AUTH_SESSION_ABSOLUTE_LIFETIME"), &c.SessionAbsoluteLife, 5*time.Minute, 24*time.Hour, "AUTH_SESSION_ABSOLUTE_LIFETIME"); err != nil {
		return err
	}
	if c.SessionIdleTimeout > c.SessionAbsoluteLife {
		return errors.New("AUTH_SESSION_IDLE_TIMEOUT must not exceed AUTH_SESSION_ABSOLUTE_LIFETIME")
	}
	return nil
}

func parseAuthDuration(raw string, target *time.Duration, min, max time.Duration, name string) error {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	d, err := time.ParseDuration(raw)
	if err != nil || d < min || d > max {
		return errors.New(name + " is outside the allowed range")
	}
	*target = d
	return nil
}

func PoolConfig(c Config) (*pgxpool.Config, error) {
	u, err := url.Parse(c.DatabaseURL)
	if err != nil || (u.Scheme != "postgres" && u.Scheme != "postgresql") || u.Hostname() == "" || u.Path == "" || u.Path == "/" {
		return nil, errors.New("DATABASE_URL must be a PostgreSQL URL with explicit host and database")
	}
	pc, err := pgxpool.ParseConfig(c.DatabaseURL)
	if err != nil {
		return nil, errors.New("invalid PostgreSQL configuration")
	}
	pc.ConnConfig.ConnectTimeout = c.DatabaseTimeout
	pc.MaxConns = 10
	pc.MinConns = 0
	pc.MaxConnLifetime = 30 * time.Minute
	pc.MaxConnIdleTime = 5 * time.Minute
	return pc, nil
}
