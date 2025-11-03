package api

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/joshhsoj1902/game-stats-exporter/internal/logger"
	"github.com/sirupsen/logrus"
)

type Handlers struct {
	steamCollector SteamCollector
	osrsCollector  OSRSCollector
}

type SteamCollector interface {
	Collect(steamId string) error
}

type OSRSCollector interface {
	CollectPlayerStats(rsn string, mode string) error
	CollectWorldData() error
}

func NewHandlers(steamCollector SteamCollector, osrsCollector OSRSCollector) *Handlers {
	return &Handlers{
		steamCollector: steamCollector,
		osrsCollector:  osrsCollector,
	}
}

// HandleAllMetrics handles /metrics - serves only system metrics (Go runtime, process, etc.)
func (h *Handlers) HandleAllMetrics(w http.ResponseWriter, r *http.Request) {
	logger.Log.WithFields(logrus.Fields{
		"path":   r.URL.Path,
		"method": r.Method,
		"ip":     r.RemoteAddr,
	}).Info("System metrics request received")

	// Serve only system metrics (excludes steam_* and osrs_* application metrics)
	SystemMetricsHandler().ServeHTTP(w, r)
}

// HandleSteamMetrics handles /metrics/steam/{steam_id}
func (h *Handlers) HandleSteamMetrics(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	steamId := chi.URLParam(r, "steam_id")

	logger.Log.WithFields(logrus.Fields{
		"path":     r.URL.Path,
		"method":   r.Method,
		"steam_id": steamId,
		"ip":       r.RemoteAddr,
	}).Info("Steam metrics request received")

	if steamId == "" {
		logger.Log.Error("Steam metrics request missing steam_id parameter")
		http.Error(w, "steam_id is required", http.StatusBadRequest)
		return
	}

	if h.steamCollector == nil {
		logger.Log.Error("Steam collector not initialized - STEAM_KEY not set")
		http.Error(w, "Steam collector not initialized - STEAM_KEY environment variable is required", http.StatusInternalServerError)
		return
	}

	// Collect metrics for this user
	logger.Log.WithField("steam_id", steamId).Info("Collecting Steam metrics")
	err := h.steamCollector.Collect(steamId)
	if err != nil {
		// If rate limited, serve whatever metrics are already present (from cache)
		if strings.Contains(strings.ToLower(err.Error()), "rate limited") {
			logger.Log.WithFields(logrus.Fields{
				"steam_id": steamId,
				"error":    err.Error(),
				"duration": time.Since(start),
			}).Warn("Rate limited by Steam - serving cached/last reported metrics only")
			SteamHandler().ServeHTTP(w, r)
			return
		}

		logger.Log.WithFields(logrus.Fields{
			"steam_id": steamId,
			"error":    err.Error(),
			"duration": time.Since(start),
		}).Error("Failed to collect Steam metrics")
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	logger.Log.WithFields(logrus.Fields{
		"steam_id": steamId,
		"duration": time.Since(start),
	}).Info("Steam metrics collection completed successfully")

	// Serve Prometheus metrics (Steam only, filtered)
	SteamHandler().ServeHTTP(w, r)
}

// HandleOSRSWorldMetrics handles /metrics/osrs/worlds
func (h *Handlers) HandleOSRSWorldMetrics(w http.ResponseWriter, r *http.Request) {
	start := time.Now()

	logger.Log.WithFields(logrus.Fields{
		"path":   r.URL.Path,
		"method": r.Method,
		"ip":     r.RemoteAddr,
	}).Info("OSRS world metrics request received")

	// Collect world metrics
	logger.Log.Info("Collecting OSRS world data")
	err := h.osrsCollector.CollectWorldData()
	if err != nil {
		logger.Log.WithFields(logrus.Fields{
			"error":    err.Error(),
			"duration": time.Since(start),
		}).Error("Failed to collect OSRS world data")
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	logger.Log.WithFields(logrus.Fields{
		"duration": time.Since(start),
	}).Info("OSRS world metrics collection completed successfully")

	// Serve Prometheus metrics (OSRS only)
	OSRSHandler().ServeHTTP(w, r)
}

// HandleOSRSMetrics handles /metrics/osrs/{mode}/{playerid}
// mode can be "vanilla" (for player stats) or other future modes
func (h *Handlers) HandleOSRSMetrics(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	mode := chi.URLParam(r, "mode")
	playerid := chi.URLParam(r, "playerid")

	logger.Log.WithFields(logrus.Fields{
		"path":     r.URL.Path,
		"method":   r.Method,
		"mode":     mode,
		"playerid": playerid,
		"ip":       r.RemoteAddr,
	}).Info("OSRS metrics request received")

	switch mode {
	case "vanilla", "gridmaster":
		// Collect player stats for vanilla or gridmaster mode
		if playerid == "" {
			logger.Log.WithField("mode", mode).Error("OSRS metrics request missing playerid parameter")
			http.Error(w, fmt.Sprintf("playerid is required for %s mode", mode), http.StatusBadRequest)
			return
		}

		logger.Log.WithFields(logrus.Fields{
			"playerid": playerid,
			"mode":     mode,
		}).Info("Collecting OSRS player metrics")
		err := h.osrsCollector.CollectPlayerStats(playerid, mode)
		if err != nil {
			logger.Log.WithFields(logrus.Fields{
				"playerid": playerid,
				"mode":     mode,
				"error":    err.Error(),
				"duration": time.Since(start),
			}).Error("Failed to collect OSRS player metrics")
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		logger.Log.WithFields(logrus.Fields{
			"playerid": playerid,
			"mode":     mode,
			"duration": time.Since(start),
		}).Info("OSRS player metrics collection completed successfully")

	default:
		logger.Log.WithField("mode", mode).Error("Unknown OSRS mode")
		http.Error(w, "Unknown mode. Supported modes: 'vanilla', 'gridmaster' (use /metrics/osrs/worlds for world data)", http.StatusBadRequest)
		return
	}

	// Serve Prometheus metrics (OSRS only)
	OSRSHandler().ServeHTTP(w, r)
}

// HandleRoot serves a simple front page
func (h *Handlers) HandleRoot(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html")
	w.Write([]byte(`<html>
<head><title>Game Stats Exporter</title></head>
<body>
	<h1>Game Stats Exporter</h1>
	<p>Prometheus metrics exporter for Steam and OSRS stats</p>
	<h2>Endpoints:</h2>
	<ul>
		<li><a href="/metrics">/metrics</a> - System metrics only (Go runtime, process, etc.)</li>
		<li><a href="/metrics/steam/{steam_id}">/metrics/steam/{steam_id}</a> - Steam player metrics (filtered, Steam only)</li>
		<li><a href="/metrics/osrs/vanilla/{playerid}">/metrics/osrs/vanilla/{playerid}</a> - OSRS vanilla player metrics (filtered, OSRS only)</li>
		<li><a href="/metrics/osrs/gridmaster/{playerid}">/metrics/osrs/gridmaster/{playerid}</a> - OSRS gridmaster (tournament) player metrics (filtered, OSRS only)</li>
		<li><a href="/metrics/osrs/worlds">/metrics/osrs/worlds</a> - OSRS world metrics (filtered, OSRS only)</li>
	</ul>
</body>
</html>`))
}

