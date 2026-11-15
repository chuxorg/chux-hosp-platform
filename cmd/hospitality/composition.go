package main

import (
	"net/http"
	"time"

	pmsapi "github.com/chuxorg/chux-hosp-platform/internal/modules/pms/api"
	"github.com/chuxorg/chux-hosp-platform/internal/platform"
	"github.com/jackc/pgx/v5/pgxpool"
)

// composeHandler is the single application composition root for HTTP modules.
// Platform infrastructure does not import business modules, and PMS does not
// import optional Experiences, Loyalty, Procurement or Operations packages.
func composeHandler(pool *pgxpool.Pool, databaseTimeout time.Duration) http.Handler {
	mux := http.NewServeMux()
	mux.Handle("/", platform.Handler(pool, databaseTimeout))
	mux.Handle("/pms/v1/", http.StripPrefix("/pms/v1", pmsapi.Handler()))
	return mux
}
