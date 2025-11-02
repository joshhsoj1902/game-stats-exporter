package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/joshhsoj1902/game-stats-exporter/internal/api"
	"github.com/joshhsoj1902/game-stats-exporter/internal/cache"
	"github.com/joshhsoj1902/game-stats-exporter/internal/logger"
	"github.com/joshhsoj1902/game-stats-exporter/internal/osrs"
	"github.com/joshhsoj1902/game-stats-exporter/internal/polling"
	"github.com/joshhsoj1902/game-stats-exporter/internal/steam"
	"github.com/sirupsen/logrus"
)

func main() {
	// Initialize logger first
	logger.Log.Info("Starting game-stats-exporter")

	// Load configuration from environment variables
	config := loadConfig()

	logger.Log.WithFields(logrus.Fields{
		"port":               config.Port,
		"redis_addr":         config.RedisAddr,
		"poll_interval":      config.PollIntervalNormal,
		"poll_interval_active": config.PollIntervalActive,
		"steam_key_set":      config.SteamKey != "",
	}).Info("Configuration loaded")

	// Initialize Redis cache
	redisCache := cache.New(config.RedisAddr, config.RedisPassword, config.RedisDB)
	defer redisCache.Close()

	// Initialize collectors
	var steamCollector *steam.Collector
	if config.SteamKey != "" {
		steamCollector = steam.NewCollector(config.SteamKey, redisCache)
	}

	osrsCollector := osrs.NewCollector(redisCache)

	// Initialize polling manager (optional - for background polling if needed)
	// Note: Currently collection is on-demand via HTTP endpoints
	// The polling manager can be used for background polling if desired
	var pollingManager *polling.Manager
	if steamCollector != nil {
		pollingManager = polling.NewManager(
			steamCollector,
			osrsCollector,
			config.PollIntervalNormal,
			config.PollIntervalActive,
		)
		// Start background polling for world data
		pollingManager.StartWorldDataPolling()
	}

	// Initialize handlers with polling manager
	handlers := api.NewHandlers(steamCollector, osrsCollector)

	// Create router
	router := api.NewRouter(handlers)

	// Create HTTP server
	server := &http.Server{
		Addr:    fmt.Sprintf(":%d", config.Port),
		Handler: router,
	}

	// Start server in a goroutine
	go func() {
		logger.Log.WithField("port", config.Port).Info("Starting HTTP server")
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Log.WithError(err).Fatal("Failed to start server")
		}
	}()

	// Wait for interrupt signal to gracefully shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	logger.Log.Info("Shutting down server...")

	// Stop polling manager if it exists
	if pollingManager != nil {
		logger.Log.Info("Stopping polling manager")
		pollingManager.Stop()
	}

	// Shutdown HTTP server with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := server.Shutdown(ctx); err != nil {
		logger.Log.WithError(err).Fatal("Server forced to shutdown")
	}

	logger.Log.Info("Server exited")
}

type Config struct {
	SteamKey          string
	RedisAddr         string
	RedisPassword     string
	RedisDB           int
	PollIntervalNormal time.Duration
	PollIntervalActive time.Duration
	Port               int
}

func loadConfig() Config {
	config := Config{}

	// Steam API key
	config.SteamKey = os.Getenv("STEAM_KEY")

	// Redis configuration
	config.RedisAddr = getEnv("REDIS_ADDR", "localhost:6379")
	config.RedisPassword = os.Getenv("REDIS_PASSWORD")

	redisDBStr := os.Getenv("REDIS_DB")
	if redisDBStr != "" {
		if db, err := strconv.Atoi(redisDBStr); err == nil {
			config.RedisDB = db
		}
	}

	// Polling intervals
	pollNormalStr := getEnv("POLL_INTERVAL_NORMAL", "15m")
	if interval, err := time.ParseDuration(pollNormalStr); err == nil {
		config.PollIntervalNormal = interval
	} else {
		config.PollIntervalNormal = 15 * time.Minute // Default
	}

	pollActiveStr := getEnv("POLL_INTERVAL_ACTIVE", "5m")
	if interval, err := time.ParseDuration(pollActiveStr); err == nil {
		config.PollIntervalActive = interval
	} else {
		config.PollIntervalActive = 5 * time.Minute // Default
	}

	// Port
	portStr := getEnv("PORT", "8000")
	if port, err := strconv.Atoi(portStr); err == nil {
		config.Port = port
	} else {
		config.Port = 8000 // Default
	}

	return config
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

