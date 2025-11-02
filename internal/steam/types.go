package steam

type Achievement struct {
	Name     string `json:"name"`
	Achieved int    `json:"achieved"`
}

type PlayerStats struct {
	SteamID      string        `json:"steamID"`
	GameName     string        `json:"gameName"`
	Achievements []Achievement `json:"achievements"`
}

type AchievementResponse struct {
	PlayerStats PlayerStats `json:"playerstats"`
}

type GlobalAchievement struct {
	Name    string `json:"name"`
	Percent string `json:"percent"`
}

type GlobalAchievementResponse struct {
	AchievementPercentages struct {
		Achievements []GlobalAchievement `json:"achievements"`
	} `json:"achievementpercentages"`
}

type OwnedGame struct {
	AppId           uint64 `json:"appid"`
	Name            string `json:"name"`
	PlaytimeForever int    `json:"playtime_forever"` // This is in minutes
}

type OwnedGamesResponse struct {
	GameCount uint        `json:"game_count"`
	Games     []OwnedGame `json:"games"`
}

type OwnedGamesHttpResponse struct {
	Response OwnedGamesResponse `json:"response"`
}

type PlayerSummary struct {
	SteamID      string `json:"steamid"`
	PersonaName  string `json:"personaname"`
	ProfileURL   string `json:"profileurl"`
	Avatar       string `json:"avatar"`
	AvatarMedium string `json:"avatarmedium"`
	AvatarFull   string `json:"avatarfull"`
}

type PlayerSummariesResponse struct {
	Response struct {
		Players []PlayerSummary `json:"players"`
	} `json:"response"`
}

