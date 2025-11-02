# Game Stats Exporter - Agent Context

This is a Prometheus metrics exporter for Steam and Old School RuneScape (OSRS) game statistics.

## Architecture Overview

- **Language**: Go
- **Web Framework**: go-chi/chi for routing
- **Metrics**: Prometheus client_golang
- **Caching**: Redis (via go-redis)
- **Logging**: logrus

## API Endpoints

### Steam
- `/metrics/steam/{steam_id}` - Steam player metrics (requires numeric Steam ID, not username)

### OSRS
- `/metrics/osrs/vanilla/{playerid}` - OSRS vanilla player stats (levels, XP, ranks)
- `/metrics/osrs/worlds` - OSRS world player counts (no playerid needed)

All endpoints use metric filtering to ensure only relevant metrics are exposed (Steam endpoints show only `steam_*` metrics, OSRS endpoints show only `osrs_*` metrics).

## Caching Strategy

### Steam Achievements

**Global Achievements**: Cached for **7 days** with 0-12 hours jitter
- Rarely change, so long cache is appropriate
- Jitter prevents thundering herd when caches expire

**User Achievements**: Dynamic TTL based on activity
- **Active players** (playtime increased): **2-5 minutes** with jitter
  - Ensures fresh data when actively playing
  - Prevents refetching on every scrape (if scraping every 5 seconds)
  - Achievements detected within 2-5 minutes
- **Inactive players** (playtime unchanged): **4-6 hours** with jitter
  - Achievements won't change while not playing
  - Longer cache reduces unnecessary API calls

**Owned Games**: 30 minutes TTL

### OSRS Player Stats
- Cached for **15 minutes** TTL
- Cache invalidated if XP increases (active play detection)

### OSRS World Data
- Cached for **5 minutes** TTL
- Note: World data endpoint currently has parsing issues due to server response truncation at 30KB

## Metric Conventions

### Metric Prefixes
- `steam_*` - All Steam metrics
- `osrs_*` - All OSRS metrics

### OSRS Player Metrics
- `osrs_player_level{skill, player, profile, mode}` - Skill levels
- `osrs_player_xp{skill, player, profile, mode}` - Experience points
- `osrs_player_rank{skill, player, profile, mode}` - Highscores ranks (only reported if rank >= 0, -1 means unranked and is excluded)
- The `mode` label allows filtering by game mode (e.g., "vanilla")

### OSRS World Metrics
- `osrs_world_players{id, location, isMembers, type}` - Player count per world

### Steam Metrics
- `steam_owned_games_playtime_seconds{app_id, game_name, steam_id}` - Playtime per game
- `steam_achievements_achieved{app_id, game_name, achievement_name, steam_id, achieved}` - Achievement status (0 or 1)

## Key Design Decisions

### Metrics Isolation
- Metrics are reset before each collection to prevent cross-contamination
- Player stats endpoints reset world metrics
- World endpoints reset player metrics
- Metric filtering ensures endpoints only expose relevant metrics

### Activity Detection

**Steam**: Detected by checking if playtime has increased since last cache
- If playtime increased → active player
- Triggers shorter cache TTL (2-5 minutes) for achievements

**OSRS**: Detected by checking if XP has increased since last check
- If XP increased → active player
- Used for adaptive polling intervals

### Caching Jitter
All caches use random jitter to prevent simultaneous expiration:
- Prevents "thundering herd" problem when many caches expire at once
- Spreads out API requests over time
- Reduces rate limiting risk

## Known Issues

### OSRS World Data
- The server truncates responses at exactly 30KB
- This corrupts the binary format header (world count field)
- Currently returns empty results gracefully rather than errors
- World IDs and player counts may be incorrect due to truncation corruption
- Consider alternative data sources or accepting partial/incomplete data

## Important Notes

### Steam API
- Requires numeric Steam ID (not username)
- Steam API key required (STEAM_KEY environment variable)
- API is heavily rate-limited
- Some games return 403 for achievements (cached to avoid repeated failures)

### OSRS API
- Player stats from: `https://oldschool.runescape.wiki/cors/m=hiscore_oldschool/index_lite.ws?player={rsn}`
- World data from: `https://www.runescape.com/g=oldscape/slr.ws?order=LPWM` (binary format, truncated at 30KB)
- Player ranks are parsed as integers to avoid scientific notation in Prometheus output
- Supports multiple game modes via the `mode` label (currently "vanilla")

### Metric Formatting
- Ranks are integers and should not be shown in scientific notation
- Negative ranks (-1) indicate unranked and are excluded from metrics
- All numeric values are validated within reasonable ranges

## Development Guidelines

- Use structured logging with logrus
- All API clients should handle rate limiting and caching appropriately
- Metrics should be reset between collections to prevent stale data
- Cache keys should be descriptive and consistent
- Error handling should be graceful and informative

