package api

import (
	"github.com/go-chi/chi/v5"
)

func NewRouter(handlers *Handlers) *chi.Mux {
	r := chi.NewRouter()

	r.Get("/", handlers.HandleRoot)

	// Generic metrics endpoint - serves all metrics (including Go runtime metrics)
	r.Get("/metrics", handlers.HandleAllMetrics)

	// Service-specific filtered endpoints
	r.Get("/metrics/steam/{steam_id}", handlers.HandleSteamMetrics)

	// Worlds endpoint (no playerid needed)
	r.Get("/metrics/osrs/worlds", handlers.HandleOSRSWorldMetrics)

	// Mode-based endpoints: /metrics/osrs/{mode}/{playerid}
	// mode can be "vanilla" (for player stats) or other future modes
	r.Get("/metrics/osrs/{mode}/{playerid}", handlers.HandleOSRSMetrics)

	return r
}

