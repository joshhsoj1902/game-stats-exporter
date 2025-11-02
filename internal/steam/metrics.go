package steam

import (
	"strconv"

	"github.com/prometheus/client_golang/prometheus"
)

var (
	ownedGamePlaytimeGauge = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: "steam",
		Subsystem:  "owned_games",
		Name:       "playtime_seconds",
		Help:       "Amount of time an owned game has been played (in seconds)",
	}, []string{"app_id", "game_name", "steam_id", "username"})

	achievementGauge = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: "steam",
		Subsystem: "achievements",
		Name:      "achieved",
		Help:      "Whether an achievement has been achieved (1) or not (0)",
	}, []string{"app_id", "game_name", "achievement_name", "steam_id", "username", "achieved"})
)

func init() {
	prometheus.MustRegister(ownedGamePlaytimeGauge)
	prometheus.MustRegister(achievementGauge)
}

// ReportOwnedGame reports playtime metrics for a game
func ReportOwnedGame(game OwnedGame, userId string, username string) {
	// Prometheus prefers seconds rather than minutes
	var playtimeSeconds = float64(60 * game.PlaytimeForever)
	ownedGamePlaytimeGauge.With(prometheus.Labels{
		"game_name": game.Name,
		"app_id":    strconv.FormatUint(game.AppId, 10),
		"steam_id":  userId,
		"username":  username,
	}).Set(playtimeSeconds)
}

// ReportAchievements reports achievement metrics for a game
func ReportAchievements(userAchievements []Achievement, globalAchievements []GlobalAchievement, gameName string, appId uint64, userId string, username string) {
	// Create a map of user achievements for quick lookup
	userAchievementMap := make(map[string]int)
	for _, achievement := range userAchievements {
		userAchievementMap[achievement.Name] = achievement.Achieved
	}

	// Report all achievements, using 0 for unearned ones
	for _, globalAchievement := range globalAchievements {
		achieved := 0
		if earned, exists := userAchievementMap[globalAchievement.Name]; exists {
			achieved = earned
		}

		// Create a more meaningful achieved label
		achievedLabel := "false"
		if achieved == 1 {
			achievedLabel = "true"
		}

		achievementGauge.With(prometheus.Labels{
			"game_name":        gameName,
			"app_id":           strconv.FormatUint(appId, 10),
			"achievement_name": globalAchievement.Name,
			"steam_id":         userId,
			"username":         username,
			"achieved":         achievedLabel,
		}).Set(float64(achieved))
	}
}

