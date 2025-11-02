package steam

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/joshhsoj1902/game-stats-exporter/internal/logger"
	"github.com/sirupsen/logrus"
)

const (
	APIOrigin                     = "https://api.steampowered.com"
	OwnedGamesEndpoint            = "/IPlayerService/GetOwnedGames/v0001/"
	AchievementsEndpoint          = "/ISteamUserStats/GetUserStatsForGame/v0002/"
	GlobalAchievementsEndpoint    = "/ISteamUserStats/GetGlobalAchievementPercentagesForApp/v0002/"
	PlayerSummariesEndpoint       = "/ISteamUser/GetPlayerSummaries/v0002/"
)

type Client struct {
	apiKey    string
	httpClient *http.Client
}

func NewClient(apiKey string) *Client {
	return &Client{
		apiKey: apiKey,
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

func (c *Client) getJSON(url string, params map[string]string, target interface{}) error {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	q := req.URL.Query()
	for k, v := range params {
		q.Add(k, v)
	}
	q.Add("key", c.apiKey)
	q.Add("format", "json")
	req.URL.RawQuery = q.Encode()

	// Log the request URL (without the API key)
	debugParams := make(map[string]string)
	for k, v := range params {
		debugParams[k] = v
	}
	debugParams["key"] = "[HIDDEN]"
	debugParams["format"] = "json"
	debugQuery := make([]string, 0, len(debugParams))
	for k, v := range debugParams {
		debugQuery = append(debugQuery, fmt.Sprintf("%s=%s", k, v))
	}
	logger.Log.WithFields(logrus.Fields{
		"url":    url,
		"params": strings.Join(debugQuery, "&"),
	}).Debug("Making Steam API request")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		logger.Log.WithError(err).Error("Steam API request failed")
		return fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		logger.Log.WithError(err).Error("Failed to read Steam API response body")
		return fmt.Errorf("failed to read response body: %w", err)
	}

	logger.Log.WithFields(logrus.Fields{
		"status_code": resp.StatusCode,
		"body_length": len(body),
	}).Debug("Steam API response received")

	switch resp.StatusCode {
	case http.StatusOK:
		// Continue with JSON parsing
		logger.Log.Debug("Steam API request successful")
	case http.StatusTooManyRequests:
		logger.Log.Error("Steam API rate limit exceeded (429)")
		return fmt.Errorf("rate limited by Steam API (429)")
	case http.StatusUnauthorized:
		logger.Log.Error("Steam API unauthorized (401) - check API key")
		return fmt.Errorf("unauthorized (401) - check your Steam API key")
	case http.StatusForbidden:
		logger.Log.Error("Steam API forbidden (403) - check API key and permissions")
		return fmt.Errorf("forbidden (403) - check your Steam API key and permissions")
	case http.StatusBadRequest:
		logger.Log.WithFields(logrus.Fields{
			"status_code": resp.StatusCode,
			"body":        string(body),
		}).Error("Steam API bad request (400)")
		return fmt.Errorf("bad request (400): %s", string(body))
	default:
		logger.Log.WithFields(logrus.Fields{
			"status_code": resp.StatusCode,
			"body":        string(body),
		}).Error("Unexpected Steam API response")
		return fmt.Errorf("unexpected status code %d: %s", resp.StatusCode, string(body))
	}

	// Check if the response starts with HTML (common error case)
	if len(body) > 0 && body[0] == '<' {
		logger.Log.WithField("body", string(body)).Error("Received HTML instead of JSON from Steam API")
		return fmt.Errorf("received HTML instead of JSON. Response: %s", string(body))
	}

	err = json.NewDecoder(bytes.NewReader(body)).Decode(target)
	if err != nil {
		bodyPreview := string(body)
		if len(bodyPreview) > 200 {
			bodyPreview = bodyPreview[:200] + "..."
		}
		logger.Log.WithError(err).WithField("body_preview", bodyPreview).Error("Failed to decode Steam API JSON response")
		return fmt.Errorf("failed to decode JSON: %w, body: %s", err, string(body))
	}

	return nil
}

// GetOwnedGames retrieves the list of games owned by a Steam user
func (c *Client) GetOwnedGames(steamId string) (OwnedGamesResponse, error) {
	logger.Log.WithField("steam_id", steamId).Info("Fetching owned games from Steam API")

	// Validate Steam ID format (should be numeric)
	if steamId == "" {
		logger.Log.Error("Steam ID is empty")
		return OwnedGamesResponse{}, fmt.Errorf("steam ID cannot be empty")
	}

	// Check if it looks like a Steam ID (should be numeric, typically 17 digits)
	if _, err := strconv.ParseUint(steamId, 10, 64); err != nil {
		logger.Log.WithFields(logrus.Fields{
			"steam_id": steamId,
			"error":    err.Error(),
		}).Error("Invalid Steam ID format - must be numeric")
		return OwnedGamesResponse{}, fmt.Errorf("invalid Steam ID format: '%s' - Steam IDs must be numeric (e.g., 76561197987123908). You may have used a username instead", steamId)
	}

	if c.apiKey == "" {
		logger.Log.Error("Steam API key not configured")
		return OwnedGamesResponse{}, fmt.Errorf("Steam API key is not configured - set STEAM_KEY environment variable")
	}

	url := APIOrigin + OwnedGamesEndpoint

	params := map[string]string{
		"steamid":                  steamId,
		"include_appinfo":          "true",
		"include_played_free_games": "true",
	}

	var httpResp OwnedGamesHttpResponse
	err := c.getJSON(url, params, &httpResp)
	if err != nil {
		logger.Log.WithFields(logrus.Fields{
			"steam_id": steamId,
			"error":    err.Error(),
		}).Error("Failed to get owned games from Steam API")
		return OwnedGamesResponse{}, fmt.Errorf("GetOwnedGames failed for steamid=%s: %w", steamId, err)
	}

	logger.Log.WithFields(logrus.Fields{
		"steam_id":   steamId,
		"game_count": httpResp.Response.GameCount,
	}).Info("Successfully fetched owned games from Steam API")

	return httpResp.Response, nil
}

// GetUserStatsForGame retrieves achievement data for a specific game and user
func (c *Client) GetUserStatsForGame(steamId string, appId uint64) (AchievementResponse, error) {
	url := APIOrigin + AchievementsEndpoint

	params := map[string]string{
		"steamid": steamId,
		"appid":   strconv.FormatUint(appId, 10),
	}

	var achievementResp AchievementResponse
	err := c.getJSON(url, params, &achievementResp)
	if err != nil {
		return AchievementResponse{}, err
	}

	return achievementResp, nil
}

// GetGlobalAchievementPercentages retrieves the list of all achievements for a game
func (c *Client) GetGlobalAchievementPercentages(appId uint64) (GlobalAchievementResponse, error) {
	url := APIOrigin + GlobalAchievementsEndpoint

	params := map[string]string{
		"gameid": strconv.FormatUint(appId, 10),
	}

	var globalResp GlobalAchievementResponse
	err := c.getJSON(url, params, &globalResp)
	if err != nil {
		return GlobalAchievementResponse{}, err
	}

	return globalResp, nil
}

// GetPlayerSummaries retrieves player information including username (personaname) from Steam IDs
func (c *Client) GetPlayerSummaries(steamIds []string) ([]PlayerSummary, error) {
	if len(steamIds) == 0 {
		return nil, fmt.Errorf("steamIds cannot be empty")
	}

	url := APIOrigin + PlayerSummariesEndpoint

	// Steam API accepts comma-separated list of Steam IDs (up to 100)
	params := map[string]string{
		"steamids": strings.Join(steamIds, ","),
	}

	var resp PlayerSummariesResponse
	err := c.getJSON(url, params, &resp)
	if err != nil {
		return nil, err
	}

	return resp.Response.Players, nil
}

