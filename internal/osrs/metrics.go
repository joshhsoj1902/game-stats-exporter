package osrs

import (
	"strconv"

	"github.com/prometheus/client_golang/prometheus"
)

var (
	playerLevelGauge = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: "osrs",
		Subsystem: "player",
		Name:      "level",
		Help:      "Player skill level",
	}, []string{"skill", "player", "profile", "mode"})

	playerXPGauge = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: "osrs",
		Subsystem: "player",
		Name:      "xp",
		Help:      "Player experience points",
	}, []string{"skill", "player", "profile", "mode"})

	playerRankGauge = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: "osrs",
		Subsystem: "player",
		Name:      "rank",
		Help:      "Player highscores rank",
	}, []string{"skill", "player", "profile", "mode"})

	worldPlayersGauge = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: "osrs",
		Subsystem: "world",
		Name:      "players",
		Help:      "Number of players in a world",
	}, []string{"id", "location", "isMembers", "type"})
)

func init() {
	prometheus.MustRegister(playerLevelGauge)
	prometheus.MustRegister(playerXPGauge)
	prometheus.MustRegister(playerRankGauge)
	prometheus.MustRegister(worldPlayersGauge)
}

// resetWorldMetrics (lowercase) is the actual implementation
func resetWorldMetrics() {
	worldPlayersGauge.Reset()
}

// resetPlayerMetrics (lowercase) is the actual implementation
func resetPlayerMetrics() {
	playerLevelGauge.Reset()
	playerXPGauge.Reset()
	playerRankGauge.Reset()
}

// ResetPlayerMetrics resets all player metrics (removes all labels)
// This is the public API, the actual implementation is resetPlayerMetrics
func ResetPlayerMetrics() {
	resetPlayerMetrics()
}

// ReportPlayerStats reports player skill metrics
func ReportPlayerStats(stats []SkillInfo, mode string) {
	// Reset all player metrics first to avoid stale data from previous requests
	ResetPlayerMetrics()

	for _, stat := range stats {
		level, _ := strconv.ParseFloat(stat.Level, 64)
		xp, _ := strconv.ParseFloat(stat.XP, 64)
		// Parse rank as integer to avoid scientific notation (ranks are always whole numbers)
		rankInt, _ := strconv.ParseInt(stat.Rank, 10, 64)
		rank := float64(rankInt)

		playerLevelGauge.With(prometheus.Labels{
			"skill":   stat.Name,
			"player":  stat.Player,
			"profile": string(stat.Profile),
			"mode":    mode,
		}).Set(level)

		playerXPGauge.With(prometheus.Labels{
			"skill":   stat.Name,
			"player":  stat.Player,
			"profile": string(stat.Profile),
			"mode":    mode,
		}).Set(xp)

		// Only report rank if it's valid (not -1, which means unranked)
		if rankInt >= 0 {
			playerRankGauge.With(prometheus.Labels{
				"skill":   stat.Name,
				"player":  stat.Player,
				"profile": string(stat.Profile),
				"mode":    mode,
			}).Set(rank)
		}
	}
}

// ResetWorldMetrics resets all world metrics (removes all labels)
// This is the public API, the actual implementation is resetWorldMetrics
func ResetWorldMetrics() {
	resetWorldMetrics()
}

// ReportWorldData reports world player count metrics
func ReportWorldData(worlds []World) {
	// Reset all world metrics first to avoid stale data from previous requests
	ResetWorldMetrics()

	for _, world := range worlds {
		worldType := world.WorldType()
		isMembers := strconv.FormatBool(world.IsMembers())

		// Ensure player count is non-negative (OSRS player counts should be 0-2000)
		playerCount := world.Players
		if playerCount < 0 {
			playerCount = 0
		}
		if playerCount > 2000 {
			// Cap at 2000 if somehow we get a value higher than max
			playerCount = 2000
		}

		worldPlayersGauge.With(prometheus.Labels{
			"id":         strconv.FormatUint(uint64(world.ID), 10),
			"location":   string(world.Location),
			"isMembers":  isMembers,
			"type":       string(worldType),
		}).Set(float64(playerCount))
	}
}

