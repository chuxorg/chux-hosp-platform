package platform

import (
	"errors"
	"net"
	"net/url"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Config contains process settings. DatabaseURL is sensitive: never log it.
type Config struct {
	HTTPAddr        string
	DatabaseURL     string
	DatabaseTimeout time.Duration
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
	if _, err := PoolConfig(c); err != nil {
		return Config{}, err
	}
	return c, nil
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
