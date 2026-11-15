package platform

import (
	"context"
	"encoding/json"
	"net"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

func TestHealthAndReadinessDuringDatabaseOutage(t *testing.T) {
	// A real TCP endpoint that closes handshakes, not a mock database interface.
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	go func() {
		for {
			c, err := listener.Accept()
			if err != nil {
				return
			}
			_ = c.Close()
		}
	}()
	cfg, err := PoolConfig(Config{DatabaseURL: "postgres://test:password@" + listener.Addr().String() + "/outage_test?sslmode=disable", DatabaseTimeout: 100 * time.Millisecond})
	if err != nil {
		t.Fatal(err)
	}
	pool, err := pgxpool.NewWithConfig(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	h := Handler(pool, 100*time.Millisecond)
	for _, tt := range []struct {
		method, path string
		code         int
		field, value string
	}{
		{"GET", "/healthz", 200, "status", "ok"},
		{"GET", "/readyz", 503, "detail", "database unavailable"},
		{"POST", "/healthz", 405, "", ""},
		{"GET", "/healthz/extra", 404, "", ""},
	} {
		t.Run(tt.method+tt.path, func(t *testing.T) {
			w := httptest.NewRecorder()
			h.ServeHTTP(w, httptest.NewRequest(tt.method, tt.path, nil))
			if w.Code != tt.code {
				t.Fatalf("got %d, want %d", w.Code, tt.code)
			}
			if tt.field != "" {
				var body map[string]string
				if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
					t.Fatal(err)
				}
				if body[tt.field] != tt.value {
					t.Fatalf("unexpected response %v", body)
				}
				if w.Header().Get("Content-Type") != "application/json" {
					t.Fatal("missing JSON content type")
				}
			}
		})
	}
}
