package platform

import (
	"context"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Handler checks the real PostgreSQL pool. Liveness deliberately does not touch it.
func Handler(pool *pgxpool.Pool, timeout time.Duration) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		respond(w, http.StatusOK, `{"status":"ok"}`)
	})
	mux.HandleFunc("GET /readyz", func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), timeout)
		defer cancel()
		if err := pool.Ping(ctx); err != nil {
			respond(w, http.StatusServiceUnavailable, `{"detail":"database unavailable"}`)
			return
		}
		respond(w, http.StatusOK, `{"status":"ready"}`)
	})
	return mux
}

func respond(w http.ResponseWriter, code int, body string) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(code)
	_, _ = w.Write([]byte(body + "\n"))
}
