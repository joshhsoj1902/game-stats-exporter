package osrs

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/joshhsoj1902/game-stats-exporter/internal/logger"
	"github.com/sirupsen/logrus"
)

const (
	PlayerStatsURL = "https://oldschool.runescape.wiki/cors/m=hiscore_oldschool/index_lite.ws"
	WorldDataURL   = "https://www.runescape.com/g=oldscape/slr.ws?order=LPWM"
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
func (c *Client) GetPlayerStats(rsn string) ([]SkillInfo, error) {
	url := fmt.Sprintf("%s?player=%s", PlayerStatsURL, rsn)

	resp, err := c.httpClient.Get(url)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch player stats: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("player not found (status: %d)", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	// Parse CSV format: rank,level,xp per line
	lines := strings.Split(string(body), "\n")
	var skills []SkillInfo

	for i, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		parts := strings.Split(line, ",")
		if len(parts) != 3 {
			continue
		}

		if i >= len(Skills) {
			break
		}

		skill := SkillInfo{
			Rank:    parts[0],
			Level:   parts[1],
			XP:      parts[2],
			Name:    Skills[i],
			Player:  rsn,
			Profile: PlayerProfileStandard,
		}

		skills = append(skills, skill)
	}

	return skills, nil
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

