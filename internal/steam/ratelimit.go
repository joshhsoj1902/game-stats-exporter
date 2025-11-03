package steam

import (
	"encoding/json"
	"sync"
	"time"

	"github.com/joshhsoj1902/game-stats-exporter/internal/cache"
	"github.com/joshhsoj1902/game-stats-exporter/internal/logger"
	"github.com/sirupsen/logrus"
)

// RateLimitState tracks Steam API rate limiting status
type RateLimitState struct {
	IsRateLimited  bool          `json:"is_rate_limited"`
	BlockedUntil   time.Time     `json:"blocked_until"`
	Consecutive403 int           `json:"consecutive_403"`
	BackoffHours   int           `json:"backoff_hours"` // Current backoff duration in hours
	mu             sync.RWMutex  `json:"-"`
	cache          *cache.Cache  `json:"-"`
}

const (
	rateLimitCacheKey = "steam:rate_limit_state"
	initialBackoff    = 1 * time.Hour   // Start with 1 hour backoff
	maxBackoff        = 24 * time.Hour  // Max 24 hours backoff
	backoffMultiplier = 2               // Double each time
)

// NewRateLimitState creates a new rate limiter
func NewRateLimitState(cache *cache.Cache) *RateLimitState {
	rl := &RateLimitState{
		cache:        cache,
		BackoffHours: 1, // Start at 1 hour
	}

	// Load state from cache
	rl.loadState()
	return rl
}

// CheckAndBlock checks if we're currently rate limited and blocks if needed
// Returns true if blocked (should not make API calls), false if OK to proceed
func (rl *RateLimitState) CheckAndBlock() bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	if !rl.IsRateLimited {
		return false
	}

	if time.Now().Before(rl.BlockedUntil) {
		remaining := time.Until(rl.BlockedUntil)
		logger.Log.WithFields(logrus.Fields{
			"blocked_until":     rl.BlockedUntil,
			"remaining_seconds": int(remaining.Seconds()),
			"backoff_hours":     rl.BackoffHours,
		}).Warn("Steam API is rate limited - blocking all API calls until backoff period expires")
		return true
	}

	// Block period has expired, clear rate limit state
	rl.IsRateLimited = false
	rl.Consecutive403 = 0
	rl.BackoffHours = 1 // Reset to initial backoff
	rl.saveState()

	logger.Log.Info("Steam API rate limit backoff period expired - resuming API calls")
	return false
}

// Record403 records a 403 response and applies exponential backoff
func (rl *RateLimitState) Record403() {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	rl.Consecutive403++
	
	// Calculate exponential backoff: 1 hour, 2 hours, 4 hours, 8 hours, 16 hours, 24 hours (max)
	backoffDuration := initialBackoff
	for i := 0; i < rl.Consecutive403-1 && backoffDuration < maxBackoff; i++ {
		backoffDuration *= backoffMultiplier
		if backoffDuration > maxBackoff {
			backoffDuration = maxBackoff
			break
		}
	}

	rl.IsRateLimited = true
	rl.BlockedUntil = time.Now().Add(backoffDuration)
	rl.BackoffHours = int(backoffDuration.Hours())

	logger.Log.WithFields(logrus.Fields{
		"consecutive_403": rl.Consecutive403,
		"blocked_until":   rl.BlockedUntil,
		"backoff_hours":   rl.BackoffHours,
	}).Error("Steam API rate limit detected (403) - applying aggressive backoff")

	rl.saveState()
}

// RecordSuccess resets the consecutive 403 counter (but doesn't immediately clear rate limit if still in backoff)
func (rl *RateLimitState) RecordSuccess() {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	// Only reset if we're not currently in a backoff period
	if !rl.IsRateLimited || time.Now().After(rl.BlockedUntil) {
		rl.Consecutive403 = 0
		rl.IsRateLimited = false
		rl.BackoffHours = 1
		rl.saveState()
	}
}

func (rl *RateLimitState) loadState() {
	if cachedData, exists := rl.cache.Get(rateLimitCacheKey); exists {
		var state struct {
			IsRateLimited  bool      `json:"is_rate_limited"`
			BlockedUntil   time.Time `json:"blocked_until"`
			Consecutive403 int       `json:"consecutive_403"`
			BackoffHours   int       `json:"backoff_hours"`
		}
		if err := json.Unmarshal(cachedData, &state); err == nil {
			rl.mu.Lock()
			rl.IsRateLimited = state.IsRateLimited
			rl.BlockedUntil = state.BlockedUntil
			rl.Consecutive403 = state.Consecutive403
			rl.BackoffHours = state.BackoffHours
			rl.mu.Unlock()

			logger.Log.WithFields(logrus.Fields{
				"is_rate_limited": rl.IsRateLimited,
				"blocked_until":    rl.BlockedUntil,
				"consecutive_403":  rl.Consecutive403,
			}).Info("Loaded Steam rate limit state from cache")
		}
	}
}

func (rl *RateLimitState) saveState() {
	state := struct {
		IsRateLimited  bool      `json:"is_rate_limited"`
		BlockedUntil   time.Time `json:"blocked_until"`
		Consecutive403 int       `json:"consecutive_403"`
		BackoffHours   int       `json:"backoff_hours"`
	}{
		IsRateLimited:  rl.IsRateLimited,
		BlockedUntil:   rl.BlockedUntil,
		Consecutive403: rl.Consecutive403,
		BackoffHours:   rl.BackoffHours,
	}

	if data, err := json.Marshal(state); err == nil {
		// Cache for the duration of the backoff + 1 hour as safety margin
		ttl := 24 * time.Hour // Cache state for up to 24 hours
		if rl.IsRateLimited && time.Now().Before(rl.BlockedUntil) {
			remaining := time.Until(rl.BlockedUntil)
			ttl = remaining + 1*time.Hour // Cache until backoff expires + 1 hour safety
		}
		rl.cache.Set(rateLimitCacheKey, data, ttl)
	}
}

