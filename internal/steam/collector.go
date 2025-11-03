package steam

import (
	"encoding/json"
	"fmt"
	"math/rand"
	"strings"
	"time"

	"github.com/joshhsoj1902/game-stats-exporter/internal/cache"
	"github.com/joshhsoj1902/game-stats-exporter/internal/logger"
	"github.com/sirupsen/logrus"
)

type Collector struct {
	client    *Client
	cache     *cache.Cache
	rateLimit *RateLimitState
}

func NewCollector(apiKey string, cache *cache.Cache) *Collector {
	rateLimit := NewRateLimitState(cache)
	return &Collector{
		client:    NewClient(apiKey, rateLimit),
		cache:     cache,
		rateLimit: rateLimit,
	}
}

// Collect collects and reports all Steam metrics for a user
func (c *Collector) Collect(steamId string) error {
	logger.Log.WithField("steam_id", steamId).Info("Starting Steam metrics collection")

	// Get username (from cache or API)
	username, err := c.getUsername(steamId)
	if err != nil {
		logger.Log.WithFields(logrus.Fields{
			"steam_id": steamId,
			"error":    err.Error(),
		}).Warn("Failed to get username, continuing without username label")
		username = "" // Fallback to empty string if username lookup fails
	} else {
		logger.Log.WithFields(logrus.Fields{
			"steam_id": steamId,
			"username": username,
		}).Debug("Retrieved username for Steam user")
	}

    // Get owned games (from cache or API)
    ownedGamesResp, err := c.getOwnedGames(steamId)
    if err != nil {
        // If rate limited, attempt to serve from cache instead of failing
        if strings.Contains(strings.ToLower(err.Error()), "rate limited") {
            cacheKey := fmt.Sprintf("steam:owned_games:%s", steamId)
            if cachedData, exists := c.cache.Get(cacheKey); exists {
                var cachedResp OwnedGamesResponse
                if uerr := json.Unmarshal(cachedData, &cachedResp); uerr == nil && len(cachedResp.Games) > 0 {
                    logger.Log.WithFields(logrus.Fields{
                        "steam_id": steamId,
                        "game_count": len(cachedResp.Games),
                    }).Warn("Rate limited: using cached owned games to serve metrics")
                    ownedGamesResp = cachedResp
                } else {
                    logger.Log.WithFields(logrus.Fields{
                        "steam_id": steamId,
                        "error":    err.Error(),
                    }).Error("Rate limited and no cached owned games available")
                    return fmt.Errorf("failed to get owned games: %w", err)
                }
            } else {
                logger.Log.WithFields(logrus.Fields{
                    "steam_id": steamId,
                    "error":    err.Error(),
                }).Error("Rate limited and owned games cache miss")
                return fmt.Errorf("failed to get owned games: %w", err)
            }
        } else {
            logger.Log.WithFields(logrus.Fields{
                "steam_id": steamId,
                "error":    err.Error(),
            }).Error("Failed to get owned games")
            return fmt.Errorf("failed to get owned games: %w", err)
        }
    }

	logger.Log.WithFields(logrus.Fields{
		"steam_id":   steamId,
		"game_count": len(ownedGamesResp.Games),
	}).Info("Processing owned games")

	// Check if we're rate limited at the start - if so, we'll use cache-only mode
	isRateLimited := c.rateLimit != nil && c.rateLimit.CheckAndBlock()

	// Report playtime for all games
	for _, game := range ownedGamesResp.Games {
		ReportOwnedGame(game, steamId, username)

		// If rate limited, skip achievement collection entirely (will use cache in collectAchievements if available)
		if isRateLimited {
			logger.Log.WithFields(logrus.Fields{
				"steam_id": steamId,
				"game":     game.Name,
				"app_id":   game.AppId,
			}).Debug("Rate limited - skipping achievement collection, will use cache if available")
			// Still try to collect achievements (will use cache only)
			_ = c.collectAchievements(steamId, game, username)
			continue
		}

		// Skip achievement fetching for games with zero playtime
		if game.PlaytimeForever == 0 {
			logger.Log.WithFields(logrus.Fields{
				"steam_id": steamId,
				"game":     game.Name,
				"app_id":   game.AppId,
			}).Debug("Skipping achievements for game with zero playtime")
			continue
		}

		// Get and report achievements
        err := c.collectAchievements(steamId, game, username)
		if err != nil {
            // On rate limit, we already attempted cache inside collectAchievements; just continue
			logger.Log.WithFields(logrus.Fields{
				"steam_id": steamId,
				"game":     game.Name,
				"app_id":   game.AppId,
				"error":    err.Error(),
			}).Warn("Error collecting achievements for game, continuing")
			continue
		}
	}

	logger.Log.WithField("steam_id", steamId).Info("Completed Steam metrics collection")
	return nil
}

// getOwnedGames retrieves owned games, using cache if available
func (c *Collector) getOwnedGames(steamId string) (OwnedGamesResponse, error) {
	// Check cache first
	cacheKey := fmt.Sprintf("steam:owned_games:%s", steamId)
	if cachedData, exists := c.cache.Get(cacheKey); exists {
		var resp OwnedGamesResponse
		if err := json.Unmarshal(cachedData, &resp); err == nil {
			logger.Log.WithFields(logrus.Fields{
				"steam_id": steamId,
				"cache":    "hit",
			}).Info("Retrieved owned games from cache")
			return resp, nil
		}
		logger.Log.WithFields(logrus.Fields{
			"steam_id": steamId,
		}).Warn("Cache hit but failed to unmarshal, fetching fresh")
	}

	logger.Log.WithFields(logrus.Fields{
		"steam_id": steamId,
		"cache":    "miss",
	}).Info("Fetching owned games from API")

	// Fetch from API
	resp, err := c.client.GetOwnedGames(steamId)
	if err != nil {
		return OwnedGamesResponse{}, err
	}

	// Cache with default TTL (30 minutes)
	if data, err := json.Marshal(resp); err == nil {
		c.cache.Set(cacheKey, data, 30*time.Minute)
		logger.Log.WithFields(logrus.Fields{
			"steam_id": steamId,
			"ttl":      "30m",
		}).Debug("Cached owned games")
	}

	return resp, nil
}

// getUsername retrieves username for a Steam ID, using cache if available
func (c *Collector) getUsername(steamId string) (string, error) {
	// Check cache first
	cacheKey := fmt.Sprintf("steam:username:%s", steamId)
	if cachedData, exists := c.cache.Get(cacheKey); exists {
		var username string
		if err := json.Unmarshal(cachedData, &username); err == nil && username != "" {
			logger.Log.WithFields(logrus.Fields{
				"steam_id": steamId,
				"username": username,
				"cache":    "hit",
			}).Debug("Retrieved username from cache")
			return username, nil
		}
	}

	logger.Log.WithFields(logrus.Fields{
		"steam_id": steamId,
		"cache":    "miss",
	}).Debug("Fetching username from API")

	// Fetch from API
	summaries, err := c.client.GetPlayerSummaries([]string{steamId})
	if err != nil {
		return "", fmt.Errorf("failed to get player summary: %w", err)
	}

	if len(summaries) == 0 {
		return "", fmt.Errorf("no player summary found for Steam ID %s", steamId)
	}

	username := summaries[0].PersonaName
	if username == "" {
		return "", fmt.Errorf("username (personaname) is empty for Steam ID %s", steamId)
	}

	// Cache username for 24 hours with jitter (usernames can change but not frequently)
	if data, err := json.Marshal(username); err == nil {
		ttl := 24*time.Hour + time.Duration(rand.Intn(120))*time.Minute // 24 hours + 0-2 hours jitter
		c.cache.Set(cacheKey, data, ttl)
		logger.Log.WithFields(logrus.Fields{
			"steam_id": steamId,
			"username": username,
			"ttl":      ttl.String(),
		}).Debug("Cached username")
	}

	return username, nil
}

// collectAchievements collects achievements for a specific game
func (c *Collector) collectAchievements(steamId string, game OwnedGame, username string) error {
	// Get global achievements from cache or fetch them
	var globalAchievements []GlobalAchievement
	globalCacheKey := fmt.Sprintf("steam:global_achievements:%d", game.AppId)
	cached := false
	if cachedData, exists := c.cache.Get(globalCacheKey); exists {
		if err := json.Unmarshal(cachedData, &globalAchievements); err == nil && len(globalAchievements) > 0 {
			cached = true
		}
	}

		if !cached {
		// Fetch global achievements
		globalResp, err := c.client.GetGlobalAchievementPercentages(game.AppId)
		if err != nil {
			// Check if this is a rate limit error - if so, return early and let the rate limiter handle it
			if err.Error() == "steam API rate limited - backoff period active" ||
			   err.Error() == "forbidden (403) - Steam API rate limit detected, backing off" {
				return fmt.Errorf("steam API rate limited: %w", err)
			}
			// Note: We no longer cache "empty achievements" for 403s because 403 now means rate limiting
			// If a game legitimately has no achievements, it would typically return 200 with empty array
			return fmt.Errorf("error fetching global achievements: %w", err)
		}
		globalAchievements = globalResp.AchievementPercentages.Achievements
		if data, err := json.Marshal(globalAchievements); err == nil {
			// Global achievements change rarely, cache for 7 days with jitter to avoid thundering herd
			ttl := 7*24*time.Hour + time.Duration(rand.Intn(720))*time.Minute // 7 days + 0-12 hours jitter
			c.cache.Set(globalCacheKey, data, ttl)
			logger.Log.WithFields(logrus.Fields{
				"app_id": game.AppId,
				"ttl":    ttl.String(),
			}).Debug("Cached global achievements with jitter")
		}
	}

	// Skip if no achievements available
	if len(globalAchievements) == 0 {
		return nil
	}

	// Check if playtime increased (active player detection)
	userCacheKey := fmt.Sprintf("steam:user_achievements:%s:%d", steamId, game.AppId)
	playtimeIncreased := c.hasPlaytimeIncreased(game.AppId, steamId, game.PlaytimeForever)

	var userAchievements []Achievement
	// Try to use cached user achievements if playtime hasn't increased
	if !playtimeIncreased {
		if cachedData, exists := c.cache.Get(userCacheKey); exists {
			type cacheEntry struct {
				UserAchievements []Achievement `json:"user_achievements"`
				Playtime        int           `json:"playtime"`
			}
			var entry cacheEntry
			if err := json.Unmarshal(cachedData, &entry); err == nil {
				userAchievements = entry.UserAchievements
			}
		}
	}

    // If we don't have cached user achievements, fetch them
    if userAchievements == nil {
		// Only sleep if we're not rate limited (sleep is to avoid rate limiting, but if we're already rate limited, we won't make the call anyway)
		if c.rateLimit == nil || !c.rateLimit.CheckAndBlock() {
			// Add a small delay between achievement requests to avoid rate limiting
			time.Sleep(5 * time.Second)
		}

		// Fetch user achievements
		achievementResp, err := c.client.GetUserStatsForGame(steamId, game.AppId)
        if err != nil {
            // If rate limited, try to serve from cache instead of failing
            if strings.Contains(strings.ToLower(err.Error()), "rate limited") {
                if cachedData, exists := c.cache.Get(userCacheKey); exists {
                    type cacheEntry struct {
                        UserAchievements []Achievement `json:"user_achievements"`
                        Playtime        int           `json:"playtime"`
                    }
                    var entry cacheEntry
                    if uerr := json.Unmarshal(cachedData, &entry); uerr == nil && len(entry.UserAchievements) > 0 {
                        userAchievements = entry.UserAchievements
                        logger.Log.WithFields(logrus.Fields{
                            "steam_id": steamId,
                            "app_id":   game.AppId,
                        }).Warn("Rate limited: using cached user achievements to serve metrics")
                    } else {
                        return fmt.Errorf("error fetching user achievements: %w", err)
                    }
                } else {
                    return fmt.Errorf("error fetching user achievements: %w", err)
                }
            } else {
                return fmt.Errorf("error fetching user achievements: %w", err)
            }
        }
        if userAchievements == nil {
            userAchievements = achievementResp.PlayerStats.Achievements
        }

		// Cache user achievements with different TTLs based on activity
		type cacheEntry struct {
			UserAchievements []Achievement `json:"user_achievements"`
			Playtime        int           `json:"playtime"`
		}
		entry := cacheEntry{
			UserAchievements: userAchievements,
			Playtime:         game.PlaytimeForever,
		}
		if data, err := json.Marshal(entry); err == nil {
			var ttl time.Duration
			if playtimeIncreased {
				// Active player: Cache for 2-5 minutes to avoid refetching every scrape while still detecting achievements quickly
				ttl = 2*time.Minute + time.Duration(rand.Intn(180))*time.Second // 2-5 minutes with jitter
				logger.Log.WithFields(logrus.Fields{
					"app_id":   game.AppId,
					"steam_id": steamId,
					"ttl":      ttl.String(),
					"reason":   "playtime_increased",
				}).Debug("Cached user achievements for active player")
			} else {
				// Inactive player: Cache for 4-6 hours since achievements won't change while not playing
				ttl = 4*time.Hour + time.Duration(rand.Intn(120))*time.Minute // 4-6 hours with jitter
				logger.Log.WithFields(logrus.Fields{
					"app_id":   game.AppId,
					"steam_id": steamId,
					"ttl":      ttl.String(),
					"reason":   "inactive_player",
				}).Debug("Cached user achievements for inactive player")
			}
			c.cache.Set(userCacheKey, data, ttl)
		}
	}

	// Report achievements
	ReportAchievements(
		userAchievements,
		globalAchievements,
		game.Name,
		game.AppId,
		steamId,
		username,
	)

	return nil
}

// hasPlaytimeIncreased checks if playtime has increased since last cache
// Returns true if playtime increased, false if same or cache doesn't exist
func (c *Collector) hasPlaytimeIncreased(appId uint64, steamId string, currentPlaytime int) bool {
	userCacheKey := fmt.Sprintf("steam:user_achievements:%s:%d", steamId, appId)
	if cachedData, exists := c.cache.Get(userCacheKey); exists {
		type cacheEntry struct {
			UserAchievements []Achievement `json:"user_achievements"`
			Playtime        int           `json:"playtime"`
		}
		var entry cacheEntry
		if err := json.Unmarshal(cachedData, &entry); err == nil {
			return currentPlaytime > entry.Playtime
		}
	}
	// No cache exists, treat as playtime increased (need to fetch)
	return true
}

// shouldInvalidateUserCache checks if cache should be invalidated based on playtime
// This is kept for backward compatibility with IsActive detection
func (c *Collector) shouldInvalidateUserCache(appId uint64, steamId string, currentPlaytime int) bool {
	return c.hasPlaytimeIncreased(appId, steamId, currentPlaytime)
}

// IsActive detects if a user is actively playing by checking playtime increases
func (c *Collector) IsActive(steamId string) (bool, error) {
	// Get current owned games
	resp, err := c.client.GetOwnedGames(steamId)
	if err != nil {
		return false, err
	}

	// Check cache for last known playtimes
	for _, game := range resp.Games {
		if game.PlaytimeForever == 0 {
			continue
		}

		// Check if playtime increased (activity detected)
		if c.shouldInvalidateUserCache(game.AppId, steamId, game.PlaytimeForever) {
			return true, nil
		}
	}

	return false, nil
}

