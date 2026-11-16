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
func composeHandler(pool *pgxpool.Pool, databaseTimeout time.Duration) http.Handler {
	mux := http.NewServeMux()
	mux.Handle("/", platform.Handler(pool, databaseTimeout))
	store := pmsdb.NewStore(pool)
	mux.Handle("/pms/v1/", http.StripPrefix("/pms/v1", pmsapi.Handler(pmsapp.NewService(store), pmsapp.NewReservationService(store))))
	return mux
}
