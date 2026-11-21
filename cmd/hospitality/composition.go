package main

import (
	"net/http"
	"time"

	pmsapi "github.com/chuxorg/chux-hosp-platform/internal/modules/pms/api"
	pmsapp "github.com/chuxorg/chux-hosp-platform/internal/modules/pms/application"
	pmsdb "github.com/chuxorg/chux-hosp-platform/internal/modules/pms/infrastructure"
	"github.com/chuxorg/chux-hosp-platform/internal/platform"
	"github.com/jackc/pgx/v5/pgxpool"
)

// composeHandler is the application composition root: Platform does not import
// PMS, while PMS never imports other optional business modules.
// PostgreSQL infrastructure stays behind the PMS application's typed Store port.
func composeHandler(pool *pgxpool.Pool, databaseTimeout time.Duration, authHandlers ...http.Handler) http.Handler {
	mux := http.NewServeMux()
	mux.Handle("/", platform.Handler(pool, databaseTimeout))
	if len(authHandlers) > 0 && authHandlers[0] != nil {
		mux.Handle("/auth/v1/", http.StripPrefix("/auth/v1", authHandlers[0]))
	}
	store := pmsdb.NewStore(pool)
	pmsHandler := http.StripPrefix("/pms/v1", pmsapi.Handler(pmsapp.NewService(store), pmsapp.NewReservationService(store)))
	if len(authHandlers) > 0 && authHandlers[0] != nil {
		// Authentication proves identity only. Until the separate authorization
		// slice enforces permission and property grants, fail closed for PMS.
		pmsHandler = http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.Header().Set("Cache-Control", "no-store")
			w.WriteHeader(http.StatusForbidden)
			_, _ = w.Write([]byte("{\"error\":\"authorization not configured\"}\n"))
		})
	}
	mux.Handle("/pms/v1/", pmsHandler)
	return mux
}
