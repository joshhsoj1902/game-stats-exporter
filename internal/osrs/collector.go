package osrs

import (
	"encoding/json"
	"fmt"
	"strconv"
	"time"

	"github.com/joshhsoj1902/game-stats-exporter/internal/cache"
	"github.com/joshhsoj1902/game-stats-exporter/internal/logger"
	"github.com/sirupsen/logrus"
)

type Collector struct {
	client *Client
	cache  *cache.Cache
}

func NewCollector(cache *cache.Cache) *Collector {
	return &Collector{
		client: NewClient(),
		cache:  cache,
	}
}

// CollectPlayerStats collects and reports player stats
func (c *Collector) CollectPlayerStats(rsn string, mode string) error {
	logger.Log.WithFields(logrus.Fields{
		"rsn":  rsn,
		"mode": mode,
	}).Info("Starting OSRS player stats collection")

	// Check cache first
	var stats []SkillInfo
	cacheKey := fmt.Sprintf("osrs:player_stats:%s", rsn)
	if cachedData, exists := c.cache.Get(cacheKey); exists {
		type cacheEntry struct {
			Stats      []SkillInfo `json:"stats"`
			LastUpdate time.Time   `json:"last_update"`
		}
		var entry cacheEntry
		if err := json.Unmarshal(cachedData, &entry); err == nil {
			stats = entry.Stats
			logger.Log.WithFields(logrus.Fields{
				"rsn":   rsn,
				"cache": "hit",
			}).Info("Retrieved player stats from cache")
		} else {
			logger.Log.WithFields(logrus.Fields{
				"rsn": rsn,
			}).Warn("Cache hit but failed to unmarshal, fetching fresh")
			stats = nil
		}
	}

	// Fetch fresh data if not cached
	if stats == nil {
		logger.Log.WithFields(logrus.Fields{
			"rsn":   rsn,
			"cache": "miss",
		}).Info("Fetching player stats from API")

		freshStats, err := c.client.GetPlayerStats(rsn)
		if err != nil {
			logger.Log.WithFields(logrus.Fields{
				"rsn":   rsn,
				"error": err.Error(),
			}).Error("Failed to get player stats from API")
			return fmt.Errorf("failed to get player stats: %w", err)
		}
		stats = freshStats

		// Cache with default TTL (15 minutes)
		type cacheEntry struct {
			Stats      []SkillInfo `json:"stats"`
			LastUpdate time.Time   `json:"last_update"`
		}
		entry := cacheEntry{
			Stats:      stats,
			LastUpdate: time.Now(),
		}
		if data, err := json.Marshal(entry); err == nil {
			c.cache.Set(cacheKey, data, 15*time.Minute)
			logger.Log.WithFields(logrus.Fields{
				"rsn": rsn,
				"ttl": "15m",
			}).Debug("Cached player stats")
		}
	}

	// Reset world metrics first to ensure they don't leak into player endpoint
	ResetWorldMetrics()

	// Report metrics - this will reset player metrics
	ReportPlayerStats(stats, mode)

	logger.Log.WithFields(logrus.Fields{
		"rsn":         rsn,
		"skills_count": len(stats),
	}).Info("Completed OSRS player stats collection")

	return nil
}

// CollectWorldData collects and reports world data
func (c *Collector) CollectWorldData() error {
	logger.Log.Info("Starting OSRS world data collection")

	// Check cache first
	var worlds []World
	cacheKey := "osrs:world_data"
	if cachedData, exists := c.cache.Get(cacheKey); exists {
		if err := json.Unmarshal(cachedData, &worlds); err == nil {
			logger.Log.WithFields(logrus.Fields{
				"cache":      "hit",
				"worlds_num": len(worlds),
			}).Info("Retrieved world data from cache")
			// Use cached data
		} else {
			logger.Log.WithFields(logrus.Fields{
				"error": err.Error(),
			}).Warn("Cache hit but failed to unmarshal, fetching fresh")
			worlds = nil
		}
	}

	// Fetch fresh data if not cached
	if worlds == nil {
		logger.Log.WithField("cache", "miss").Info("Fetching world data from API")

		freshWorlds, err := c.client.GetWorldData()
		if err != nil {
			logger.Log.WithFields(logrus.Fields{
				"error": err.Error(),
			}).Error("Failed to get world data from API")
			return fmt.Errorf("failed to get world data: %w", err)
		}
		worlds = freshWorlds

		logger.Log.WithField("worlds_num", len(worlds)).Info("Successfully fetched world data from API")

		// Cache with 5 minute TTL
		if data, err := json.Marshal(worlds); err == nil {
			c.cache.Set(cacheKey, data, 5*time.Minute)
			logger.Log.WithField("ttl", "5m").Debug("Cached world data")
		}
	}

	// Reset player metrics first to ensure they don't leak into world endpoint
	ResetPlayerMetrics()

	// Report metrics - this will reset world metrics
	ReportWorldData(worlds)

	logger.Log.WithField("worlds_num", len(worlds)).Info("Completed OSRS world data collection")

	return nil
}

// IsActive detects if a player is actively playing by checking XP increases
func (c *Collector) IsActive(rsn string) (bool, error) {
	// Get current stats
	stats, err := c.client.GetPlayerStats(rsn)
	if err != nil {
		return false, err
	}

	// Get last known XP values from cache
	cacheKey := fmt.Sprintf("osrs:last_xp:%s", rsn)
	lastXP := make(map[string]int64)
	if cachedData, exists := c.cache.Get(cacheKey); exists {
		if err := json.Unmarshal(cachedData, &lastXP); err != nil {
			lastXP = make(map[string]int64)
		}
	}

	if len(lastXP) == 0 {
		// No previous data, can't determine activity
		// Store current XP for next time
		currentXP := make(map[string]int64)
		for _, stat := range stats {
			xp, _ := strconv.ParseInt(stat.XP, 10, 64)
			currentXP[stat.Name] = xp
		}
		if data, err := json.Marshal(currentXP); err == nil {
			c.cache.Set(cacheKey, data, 24*time.Hour)
		}
		return false, nil
	}

	// Check if any XP has increased
	active := false
	currentXP := make(map[string]int64)
	for _, stat := range stats {
		xp, _ := strconv.ParseInt(stat.XP, 10, 64)
		currentXP[stat.Name] = xp

		if lastXPValue, exists := lastXP[stat.Name]; exists {
			if xp > lastXPValue {
				active = true
			}
		}
	}

	// Update cached XP values
	if data, err := json.Marshal(currentXP); err == nil {
		c.cache.Set(cacheKey, data, 24*time.Hour)
	}

	return active, nil
}

