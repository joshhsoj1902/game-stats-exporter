package osrs

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/joshhsoj1902/game-stats-exporter/internal/logger"
	"github.com/sirupsen/logrus"
)

const (
	PlayerStatsURL      = "https://oldschool.runescape.wiki/cors/m=hiscore_oldschool/index_lite.ws"
	PlayerStatsHTMLURL  = "https://secure.runescape.com/m=hiscore_oldschool/hiscorepersonal"
	TournamentStatsURL  = "https://oldschool.runescape.wiki/cors/m=hiscore_oldschool_tournament/index_lite.ws"
	TournamentHTMLURL   = "https://secure.runescape.com/m=hiscore_oldschool_tournament/hiscorepersonal"
	WorldDataURL        = "https://www.runescape.com/g=oldscape/slr.ws?order=LPWM"
)

var Skills = []string{
	"Overall",
	"Attack",
	"Defence",
	"Strength",
	"Hitpoints",
	"Ranged",
	"Prayer",
	"Magic",
	"Cooking",
	"Woodcutting",
	"Fletching",
	"Fishing",
	"Firemaking",
	"Crafting",
	"Smithing",
	"Mining",
	"Herblore",
	"Agility",
	"Thieving",
	"Slayer",
	"Farming",
	"Runecrafting",
	"Hunter",
	"Construction",
	"Stuff",
}

// Known minigame names in order (as they appear in the CSV API)
// This list is based on the OSRS hiscores API order and is kept up-to-date
// Total: 87 minigames in the API
var knownMinigameNames = []string{
	"Clue Scrolls (all)",
	"Clue Scrolls (beginner)",
	"Clue Scrolls (easy)",
	"Clue Scrolls (medium)",
	"Clue Scrolls (hard)",
	"Clue Scrolls (elite)",
	"Clue Scrolls (master)",
	"LMS - Killstreak",
	"LMS - Rank",
	"PvP Arena - Rank",
	"Soul Wars Zeal",
	"Rifts closed",
	"Colosseum Glory",
	"Bounty Hunter - Hunter",
	"Bounty Hunter - Rogue",
	"Bounty Hunter (Legacy) - Hunter",
	"Bounty Hunter (Legacy) - Rogue",
	"Castle Wars Games",
	"Barbarian Assault - Honour Level",
	"BA Attack Level",
	"BA Defence Level",
	"BA Strength Level",
	"BA Hitpoints Level",
	"BA Ranged Level",
	"BA Magic Level",
	"BA Prayer Level",
	"Trouble Brewing",
	"TzTok-Jad",
	"TzKal-Zuk",
	"Wintertodt",
	// Pad to 87 entries - minigames beyond this list will use generic names
	// These will be filled in as we discover the exact order
	"", "", "", "", "", "", "", "", "", "", // 31-40
	"", "", "", "", "", "", "", "", "", "", // 41-50
	"", "", "", "", "", "", "", "", "", "", // 51-60
	"", "", "", "", "", "", "", "", "", "", // 61-70
	"", "", "", "", "", "", "", "", "", "", // 71-80
	"", "", "", "", "", "", "", "", "", "", // 81-87
}

// getMinigameNames fetches and parses minigame names from the HTML highscores page
// Falls back to known list if HTML fetch fails or doesn't return enough names
func getMinigameNames(rsn string, mode string) ([]string, error) {
	var htmlURL string
	switch mode {
	case "gridmaster":
		htmlURL = TournamentHTMLURL
	default:
		htmlURL = PlayerStatsHTMLURL
	}
	url := fmt.Sprintf("%s?user1=%s", htmlURL, rsn)

	resp, err := http.Get(url)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch HTML highscores: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to fetch HTML highscores (status: %d)", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read HTML response: %w", err)
	}

	// Extract minigame names with their table numbers
	// Format: <a href="...table=N...category_type=1...">Name</a>
	type minigameEntry struct {
		table int
		name  string
	}
	var htmlEntries []minigameEntry

	// Try to extract table numbers along with names
	reWithTable := regexp.MustCompile(`<a href="[^"]*table=(\d+)[^"]*category_type=1[^"]*">([^<]+)</a>`)
	matches := reWithTable.FindAllStringSubmatch(string(body), -1)
	for _, match := range matches {
		if len(match) >= 3 {
			tableStr := strings.TrimSpace(match[1])
			name := strings.TrimSpace(match[2])
			if name != "" && tableStr != "" {
				if tableNum, err := strconv.Atoi(tableStr); err == nil {
					htmlEntries = append(htmlEntries, minigameEntry{table: tableNum, name: name})
				}
			}
		}
	}

	// Fallback to simple extraction if table numbers aren't found
	if len(htmlEntries) == 0 {
		re := regexp.MustCompile(`<a href="[^"]*category_type=1[^"]*">([^<]+)</a>`)
		matches := re.FindAllStringSubmatch(string(body), -1)
		for _, match := range matches {
			if len(match) > 1 {
				name := strings.TrimSpace(match[1])
				if name != "" {
					htmlEntries = append(htmlEntries, minigameEntry{table: -1, name: name})
				}
			}
		}
	}

	// Convert to simple string list for compatibility (but keep entries for table mapping)
	var minigameNames []string
	for _, entry := range htmlEntries {
		minigameNames = append(minigameNames, entry.name)
	}

	logger.Log.WithFields(logrus.Fields{
		"minigame_count":      len(minigameNames),
		"entries_with_tables": len(htmlEntries),
	}).Debug("Extracted minigame names from HTML")

	// HTML only shows minigames the player has scores for
	// Return them in the order they appear (which matches the order in CSV for minigames with scores)
	// We don't need to fill in gaps - we'll only output metrics for minigames with scores anyway

	logger.Log.WithFields(logrus.Fields{
		"html_count": len(minigameNames),
	}).Debug("Extracted minigame names from HTML (only for minigames with scores)")

	// Return the HTML names directly - they're in the same order as CSV minigames with scores
	return minigameNames, nil
}

type Client struct {
	httpClient *http.Client
}

func NewClient() *Client {
	return &Client{
		httpClient: &http.Client{
			Timeout: 30 * time.Second, // Longer timeout for world data
		},
	}
}

// GetPlayerStats retrieves player stats from the OSRS hiscores API
func (c *Client) GetPlayerStats(rsn string, mode string) ([]SkillInfo, []MinigameInfo, error) {
	var statsURL string
	switch mode {
	case "gridmaster":
		statsURL = TournamentStatsURL
	default:
		statsURL = PlayerStatsURL
	}
	url := fmt.Sprintf("%s?player=%s", statsURL, rsn)

	resp, err := c.httpClient.Get(url)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to fetch player stats: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, nil, fmt.Errorf("player not found (status: %d)", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to read response: %w", err)
	}

	// Fetch minigame names from HTML page
	minigameNames, err := getMinigameNames(rsn, mode)
	if err != nil {
		logger.Log.WithFields(logrus.Fields{
			"rsn":   rsn,
			"error": err.Error(),
		}).Warn("Failed to fetch minigame names from HTML, using generic names")
		minigameNames = nil // Will fall back to generic names
	} else {
		logger.Log.WithFields(logrus.Fields{
			"rsn":             rsn,
			"minigame_count": len(minigameNames),
		}).Info("Successfully fetched minigame names from HTML")
	}

	// Parse CSV format: rank,level,xp per line for skills, rank,score for minigames
	lines := strings.Split(string(body), "\n")
	var skills []SkillInfo
	var minigames []MinigameInfo

	skillIndex := 0
	minigameIndex := 0

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		parts := strings.Split(line, ",")

		// Skills have 3 values: rank,level,xp
		if len(parts) == 3 && skillIndex < len(Skills) {
			skill := SkillInfo{
				Rank:   parts[0],
				Level:  parts[1],
				XP:     parts[2],
				Name:   Skills[skillIndex],
				Player: rsn,
			}
			skills = append(skills, skill)
			skillIndex++
		} else if len(parts) == 2 {
			// Minigames have 2 values: rank,score
			// Parse dynamically - no hardcoded list needed
			// If we've parsed all expected skills OR we get a 2-part line after parsing at least one skill,
			// then treat it as a minigame (API may return fewer skills than our list)
			if skillIndex >= len(Skills) || (skillIndex > 0 && len(skills) == skillIndex) {
				// Check if this minigame has actual scores (not -1,-1)
				rank := parts[0]
				score := parts[1]
				if rank == "-1" && score == "-1" {
					// Player doesn't have scores for this minigame - skip it
					// Increment index but don't add to the list
					minigameIndex++
					continue
				}

				// Player has scores for this minigame - use real name from HTML if available
				// HTML names are in the same order as CSV minigames with scores
				// Since we skip minigames without scores, len(minigames) gives us the index
				minigameName := fmt.Sprintf("Minigame %d", len(minigames)+1)
				if minigameNames != nil && len(minigames) < len(minigameNames) {
					name := minigameNames[len(minigames)]
					if name != "" {
						minigameName = name
					}
				}

				minigame := MinigameInfo{
					Rank:   rank,
					Score:  score,
					Name:   minigameName,
					Player: rsn,
				}
				minigames = append(minigames, minigame)
				minigameIndex++
			}
			// If we haven't parsed any skills yet, skip 2-part lines (they might be malformed)
		}
	}

	logger.Log.WithFields(logrus.Fields{
		"skills_count":    len(skills),
		"minigames_count": len(minigames),
		"total_lines":     len(lines),
	}).Debug("Parsed player stats from API")

	return skills, minigames, nil
}

// GetWorldData retrieves world data from the OSRS world list API
func (c *Client) GetWorldData() ([]World, error) {
	req, err := http.NewRequest("GET", WorldDataURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Set headers - allow gzip (Go will auto-decompress), set user agent
	req.Header.Set("User-Agent", "game-stats-exporter/1.0")
	req.Header.Set("Accept", "*/*")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch world data: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to fetch world data (status: %d)", resp.StatusCode)
	}

	// Check Content-Length header
	logger.Log.WithFields(logrus.Fields{
		"content_length": resp.ContentLength,
		"content_encoding": resp.Header.Get("Content-Encoding"),
	}).Debug("OSRS world data response headers")

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if len(body) == 0 {
		return nil, fmt.Errorf("received empty response body")
	}

	if resp.ContentLength > 0 && int64(len(body)) < resp.ContentLength {
		logger.Log.WithFields(logrus.Fields{
			"received": len(body),
			"expected": resp.ContentLength,
		}).Warn("Response body shorter than Content-Length header")
	}

	firstBytesLen := 20
	if len(body) < firstBytesLen {
		firstBytesLen = len(body)
	}
	logger.Log.WithFields(logrus.Fields{
		"body_length": len(body),
		"first_bytes": fmt.Sprintf("%x", body[:firstBytesLen]),
	}).Debug("OSRS world data response received")

	return decodeWorldData(body)
}

// decodeWorldData decodes the binary world data format
func decodeWorldData(data []byte) ([]World, error) {
	if len(data) < 6 {
		return nil, fmt.Errorf("response too short: got %d bytes, need at least 6", len(data))
	}

	reader := bytes.NewReader(data)

	// Read buffer size (first 4 bytes) - in Rust: read_i32().await? + 4
	// This is read but the result is ignored - we just need to advance past it
	var bufferSizeRaw int32
	if err := binary.Read(reader, binary.LittleEndian, &bufferSizeRaw); err != nil {
		return nil, fmt.Errorf("failed to read buffer size: %w", err)
	}
	bufferSize := bufferSizeRaw + 4 // Rust code does: read_i32() + 4

	logger.Log.WithFields(logrus.Fields{
		"buffer_size_raw": bufferSizeRaw,
		"buffer_size_calc": bufferSize,
		"data_length":      len(data),
		"remaining_bytes":  reader.Len(),
	}).Debug("Decoding OSRS world data")

	// Read number of worlds (2 bytes)
	var numWorlds int16
	if err := binary.Read(reader, binary.LittleEndian, &numWorlds); err != nil {
		if err == io.EOF {
			return nil, fmt.Errorf("unexpected EOF reading world count - response may be empty or corrupted")
		}
		return nil, fmt.Errorf("failed to read world count: %w", err)
	}

	logger.Log.WithFields(logrus.Fields{
		"num_worlds":     numWorlds,
		"remaining_data": reader.Len(),
	}).Debug("Reading OSRS worlds")

	// The server truncates responses at 30KB, which corrupts the world count field
	// When we see an invalid count, we'll parse worlds iteratively until we hit invalid data
	maxReasonableWorlds := 200
	parseIteratively := false
	if numWorlds <= 0 || numWorlds > int16(maxReasonableWorlds) {
		logger.Log.WithFields(logrus.Fields{
			"num_worlds":           numWorlds,
			"max_reasonable":       maxReasonableWorlds,
			"remaining_data_bytes": reader.Len(),
		}).Warn("Invalid world count detected - response truncated, will parse iteratively until invalid data")
		parseIteratively = true
		// Estimate reasonable number of worlds we can parse from 30KB
		// Each world averages ~40-60 bytes (varies with string lengths)
		// With 30KB, we can fit roughly 500-750 worlds, but OSRS only has ~160 worlds
		// So we'll cap at a reasonable number and parse until we hit invalid IDs
		numWorlds = int16(maxReasonableWorlds) // Cap at reasonable max
	}

	var worlds []World
	maxAttempts := int(numWorlds)
	if parseIteratively {
		maxAttempts = 300 // Try more aggressively when count is corrupted
	}

	for i := 0; i < maxAttempts; i++ {
		// Check if we have enough bytes remaining before attempting to read
		// Minimum bytes needed: 2 (id) + 4 (flags) + at least 2 for strings + 1 (location) + 2 (players) = 11 minimum
		if reader.Len() < 11 {
			logger.Log.WithFields(logrus.Fields{
				"world_num":      i + 1,
				"expected_total": numWorlds,
				"remaining":      reader.Len(),
				"worlds_read":    len(worlds),
			}).Info("Insufficient data remaining - response truncated, returning parsed worlds")
			break
		}

		// Store current position before reading, in case we need to backtrack
		currentPos := reader.Size() - int64(reader.Len())

		// Read world ID (2 bytes)
		var worldID uint16
		if err := binary.Read(reader, binary.LittleEndian, &worldID); err != nil {
			if err == io.EOF {
				logger.Log.WithFields(logrus.Fields{
					"world_num":   i + 1,
					"worlds_read": len(worlds),
				}).Info("EOF reached - response truncated")
				break
			}
			logger.Log.WithFields(logrus.Fields{
				"world_num": i + 1,
				"error":     err.Error(),
			}).Warn("Error reading world ID")
			break
		}

		// Validate world ID - OSRS world IDs are in the range 300-700 (based on webpage)
		// If we see IDs outside this range on the FIRST world, the data might be misaligned
		// due to truncation corruption. Try skipping ahead a few bytes to find alignment.
		if worldID < 300 || worldID > 700 {
			// If this is the first world and it's invalid, try to find a valid world ID
			// by scanning ahead in the data (the truncation might have corrupted the start)
			if i == 0 && parseIteratively {
				logger.Log.WithFields(logrus.Fields{
					"world_id": worldID,
				}).Warn("First world ID is invalid - data may be misaligned, attempting to find valid start")

				// Try scanning forward in 1-byte increments to find a valid world ID
				// This handles cases where truncation corrupted the header/alignment
				foundValid := false
				for offset := 1; offset < 20 && reader.Len() >= 2; offset++ {
					testPos := currentPos + int64(offset)
					reader.Seek(testPos, 0)

					var testID uint16
					if err := binary.Read(reader, binary.LittleEndian, &testID); err != nil {
						break
					}

					if testID >= 300 && testID <= 700 {
						logger.Log.WithFields(logrus.Fields{
							"offset":   offset,
							"world_id": testID,
						}).Info("Found valid world ID at offset, adjusting alignment")
						worldID = testID
						foundValid = true
						break
					}

					// Reset to test position for next iteration
					reader.Seek(testPos, 0)
				}

				if !foundValid {
					logger.Log.Warn("Could not find valid world ID alignment in corrupted data")
					return []World{}, nil
				}
			} else {
				// Not first world, just stop
				logger.Log.WithFields(logrus.Fields{
					"world_id":  worldID,
					"world_num": i + 1,
				}).Info("Invalid world ID detected - reached corrupted/truncated data, stopping")
				reader.Seek(currentPos, 0)
				break
			}
		}

		// Read world type flags (4 bytes)
		var worldTypeFlags int32
		if err := binary.Read(reader, binary.LittleEndian, &worldTypeFlags); err != nil {
			return nil, fmt.Errorf("failed to read world type flags: %w", err)
		}

		// Read address string (null-terminated)
		address, err := readNullTerminatedString(reader)
		if err != nil {
			return nil, fmt.Errorf("failed to read address: %w", err)
		}

		// Read activity string (null-terminated)
		activity, err := readNullTerminatedString(reader)
		if err != nil {
			return nil, fmt.Errorf("failed to read activity: %w", err)
		}

		// Read location (1 byte)
		var locationByte int8
		if err := binary.Read(reader, binary.LittleEndian, &locationByte); err != nil {
			return nil, fmt.Errorf("failed to read location: %w", err)
		}

		// Read player count (2 bytes)
		var playerCount int16
		if err := binary.Read(reader, binary.LittleEndian, &playerCount); err != nil {
			return nil, fmt.Errorf("failed to read player count: %w", err)
		}

		// Convert location
		location := locationFromByte(locationByte)

		// Parse world types from flags
		types := parseWorldTypes(worldTypeFlags)

		world := World{
			ID:       worldID,
			Types:    types,
			Address:  address,
			Activity: activity,
			Location: location,
			Players:  playerCount,
		}

		worlds = append(worlds, world)
	}

	return worlds, nil
}

// readNullTerminatedString reads a null-terminated string from the reader
func readNullTerminatedString(reader *bytes.Reader) (string, error) {
	var bytes []byte
	for {
		b, err := reader.ReadByte()
		if err != nil {
			return "", err
		}
		if b == 0 {
			break
		}
		bytes = append(bytes, b)
	}
	return string(bytes), nil
}

// locationFromByte converts a location byte to WorldLocation
func locationFromByte(b int8) WorldLocation {
	switch b {
	case 0:
		return WorldLocationUSA
	case 1:
		return WorldLocationUK
	case 3:
		return WorldLocationAustralia
	case 7:
		return WorldLocationGermany
	default:
		return WorldLocationUnknown
	}
}

// parseWorldTypes parses world type flags into a slice of WorldType
func parseWorldTypes(flags int32) []WorldType {
	var types []WorldType

	// Check each flag bit
	if flags&1 != 0 {
		types = append(types, WorldTypeMembers)
	}
	if flags&(1<<2) != 0 {
		types = append(types, WorldTypePVP)
	}
	if flags&(1<<5) != 0 {
		types = append(types, WorldTypeBounty)
	}
	if flags&(1<<6) != 0 {
		types = append(types, WorldTypePVPArena)
	}
	if flags&(1<<7) != 0 {
		types = append(types, WorldTypeSkillTotal)
	}
	if flags&(1<<8) != 0 {
		types = append(types, WorldTypeQuestSpeedrunning)
	}
	if flags&(1<<10) != 0 {
		types = append(types, WorldTypeHighRisk)
	}
	if flags&(1<<14) != 0 {
		types = append(types, WorldTypeLastManStanding)
	}
	if flags&(1<<22) != 0 {
		types = append(types, WorldTypeSoulWars)
	}
	if flags&(1<<23) != 0 {
		types = append(types, WorldTypeBeta)
	}
	if flags&(1<<25) != 0 {
		types = append(types, WorldTypeNoSaveMode)
	}
	if flags&(1<<26) != 0 {
		types = append(types, WorldTypeTournament)
	}
	if flags&(1<<27) != 0 {
		types = append(types, WorldTypeFreshStartWorld)
	}
	if flags&(1<<28) != 0 {
		types = append(types, WorldTypeMinigame)
	}
	if flags&(1<<29) != 0 {
		types = append(types, WorldTypeDeadman)
	}
	if flags&(1<<30) != 0 {
		types = append(types, WorldTypeSeasonal)
	}

	// If no types found, default to FreeToPlay
	if len(types) == 0 {
		types = append(types, WorldTypeFreeToPlay)
	}

	return types
}

