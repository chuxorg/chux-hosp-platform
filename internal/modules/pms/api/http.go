// Package api exposes the PMS module's own HTTP endpoints.
// Business endpoints are added only when approved PMS stories implement them.
package api

import "net/http"

// Handler is mounted by the application's composition root at /pms/v1/.
// This bootstrap-only endpoint proves PMS can be composed without any other module.
func Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /status", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-store")
		_, _ = w.Write([]byte("{\"module\":\"pms\",\"status\":\"bootstrap\"}\n"))
	})
	return mux
}
