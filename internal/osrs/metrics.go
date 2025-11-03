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
	}, []string{"skill", "player", "mode"})

	playerXPGauge = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: "osrs",
		Subsystem: "player",
		Name:      "xp",
		Help:      "Player experience points",
	}, []string{"skill", "player", "mode"})

	playerRankGauge = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: "osrs",
		Subsystem: "player",
		Name:      "rank",
		Help:      "Player highscores rank",
	}, []string{"skill", "player", "mode"})

	worldPlayersGauge = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: "osrs",
		Subsystem: "world",
		Name:      "players",
		Help:      "Number of players in a world",
	}, []string{"id", "location", "isMembers", "type"})

	minigameRankGauge = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: "osrs",
		Subsystem: "minigame",
		Name:      "rank",
		Help:      "Player minigame highscores rank",
	}, []string{"minigame", "player", "mode"})

	minigameScoreGauge = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: "osrs",
		Subsystem: "minigame",
		Name:      "score",
		Help:      "Player minigame score",
	}, []string{"minigame", "player", "mode"})
)

func init() {
	prometheus.MustRegister(playerLevelGauge)
	prometheus.MustRegister(playerXPGauge)
	prometheus.MustRegister(playerRankGauge)
	prometheus.MustRegister(worldPlayersGauge)
	prometheus.MustRegister(minigameRankGauge)
	prometheus.MustRegister(minigameScoreGauge)
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
	minigameRankGauge.Reset()
	minigameScoreGauge.Reset()
}

// ResetPlayerMetrics resets all player metrics (removes all labels)
// This is the public API, the actual implementation is resetPlayerMetrics
func ResetPlayerMetrics() {
	resetPlayerMetrics()
}

// reportPlayerStatsWithoutReset reports player skill metrics without resetting
// This is used when accumulating metrics from multiple modes
func reportPlayerStatsWithoutReset(stats []SkillInfo, mode string) {
	for _, stat := range stats {
		level, _ := strconv.ParseFloat(stat.Level, 64)
		xp, _ := strconv.ParseFloat(stat.XP, 64)
		// Parse rank as integer to avoid scientific notation (ranks are always whole numbers)
		rankInt, _ := strconv.ParseInt(stat.Rank, 10, 64)
		rank := float64(rankInt)

		playerLevelGauge.With(prometheus.Labels{
			"skill":  stat.Name,
			"player": stat.Player,
			"mode":   mode,
		}).Set(level)

		playerXPGauge.With(prometheus.Labels{
			"skill":  stat.Name,
			"player": stat.Player,
			"mode":   mode,
		}).Set(xp)

		// Only report rank if it's valid (not -1, which means unranked)
		if rankInt >= 0 {
			playerRankGauge.With(prometheus.Labels{
				"skill":  stat.Name,
				"player": stat.Player,
				"mode":   mode,
			}).Set(rank)
		}
	}
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
			"skill":  stat.Name,
			"player": stat.Player,
			"mode":   mode,
		}).Set(level)

		playerXPGauge.With(prometheus.Labels{
			"skill":  stat.Name,
			"player": stat.Player,
			"mode":   mode,
		}).Set(xp)

		// Only report rank if it's valid (not -1, which means unranked)
		if rankInt >= 0 {
			playerRankGauge.With(prometheus.Labels{
				"skill":  stat.Name,
				"player": stat.Player,
				"mode":   mode,
			}).Set(rank)
		}
	}
}

// ResetWorldMetrics resets all world metrics (removes all labels)
// This is the public API, the actual implementation is resetWorldMetrics
func ResetWorldMetrics() {
	resetWorldMetrics()
}

// reportMinigamesWithoutReset reports minigame metrics without resetting
// This is used when accumulating metrics from multiple modes
func reportMinigamesWithoutReset(minigames []MinigameInfo, mode string) {
	for _, minigame := range minigames {
		// Parse rank as integer to avoid scientific notation
		rankInt, _ := strconv.ParseInt(minigame.Rank, 10, 64)
		// Parse score as integer (minigames only increase)
		scoreInt, _ := strconv.ParseInt(minigame.Score, 10, 64)

		// Only report rank if it's valid (not -1, which means unranked)
		if rankInt >= 0 {
			minigameRankGauge.With(prometheus.Labels{
				"minigame": minigame.Name,
				"player":   minigame.Player,
				"mode":     mode,
			}).Set(float64(rankInt))
		}

		// Only report score if it's valid (not -1, which means unranked/not played)
		if scoreInt >= 0 {
			minigameScoreGauge.With(prometheus.Labels{
				"minigame": minigame.Name,
				"player":   minigame.Player,
				"mode":     mode,
			}).Set(float64(scoreInt))
		}
	}
}

// ReportMinigames reports minigame metrics (rank and score)
func ReportMinigames(minigames []MinigameInfo, mode string) {
	for _, minigame := range minigames {
		// Parse rank as integer to avoid scientific notation
		rankInt, _ := strconv.ParseInt(minigame.Rank, 10, 64)
		// Parse score as integer (minigames only increase)
		scoreInt, _ := strconv.ParseInt(minigame.Score, 10, 64)

		// Only report rank if it's valid (not -1, which means unranked)
		if rankInt >= 0 {
			minigameRankGauge.With(prometheus.Labels{
				"minigame": minigame.Name,
				"player":   minigame.Player,
				"mode":     mode,
			}).Set(float64(rankInt))
		}

		// Only report score if it's valid (not -1, which means unranked/not played)
		if scoreInt >= 0 {
			minigameScoreGauge.With(prometheus.Labels{
				"minigame": minigame.Name,
				"player":   minigame.Player,
				"mode":     mode,
			}).Set(float64(scoreInt))
		}
	}
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

