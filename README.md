# Game Stats Exporter

A unified Prometheus metrics exporter for Steam and Old School RuneScape (OSRS) game statistics.

## About

This project combines features from two existing exporters:
- **[steam-exporter](https://github.com/mitchtech/steam-exporter)** - Steam game statistics exporter
- **[osrs-prometheus-exporter](https://woodpecker.boerlage.me/daan/osrs-prometheus-exporter)** - Old School RuneScape statistics exporter

This unified exporter was created primarily with AI assistance to consolidate functionality, improve caching strategies, and provide a more streamlined experience for monitoring both Steam and OSRS game statistics.

## Features

- **Steam Integration**: Tracks owned games, playtime, and achievements
- **OSRS Integration**: Tracks player skill levels, XP, ranks, and world player counts
- **Dynamic Endpoints**: Metrics available at `/metrics/steam/{steam_id}` and `/metrics/osrs/{mode}/{playerid}`
- **Redis Caching**: Aggressive caching to minimize API rate limit issues
- **Intelligent Polling**: Adaptive polling intervals based on player activity

## Quick Start with Docker Compose

1. Create a `.env` file (optional):
```bash
STEAM_KEY=your_steam_api_key_here
```

2. Start the services:
```bash
docker-compose up -d
```

3. Access the exporter:
- Root page: http://localhost:8000
- Steam metrics: http://localhost:8000/metrics/steam/{steam_id}
- OSRS player metrics: http://localhost:8000/metrics/osrs/vanilla/{playerid}
- OSRS world metrics: http://localhost:8000/metrics/osrs/worlds

## Configuration

### Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `STEAM_KEY` | - | Steam API key (required for Steam features) |
| `REDIS_ADDR` | `localhost:6379` | Redis server address |
| `REDIS_PASSWORD` | - | Redis password (if required) |
| `REDIS_DB` | `0` | Redis database number |
| `POLL_INTERVAL_NORMAL` | `15m` | Normal polling interval |
| `POLL_INTERVAL_ACTIVE` | `5m` | Active play polling interval |
| `PORT` | `8000` | HTTP server port |

### Getting a Steam API Key

Sign up for a Steam API key at: https://steamcommunity.com/dev

### Getting Your Steam User ID

1. Login to Steam and go to your profile
2. The Steam User ID is in the URL: `https://steamcommunity.com/profiles/XXXXXX`
   Where `XXXXXX` is your Steam User ID (decimal format)

## Prometheus Configuration

Example Prometheus scrape config:

```yaml
scrape_configs:
  - job_name: steam-user
    scrape_interval: 5m
    metrics_path: /metrics/steam/YOUR_STEAM_ID
    static_configs:
      - targets:
          - localhost:8000

  - job_name: osrs-player
    scrape_interval: 15m
    metrics_path: /metrics/osrs/vanilla/YOUR_RSN
    static_configs:
      - targets:
          - localhost:8000

  - job_name: osrs-worlds
    scrape_interval: 5m
    metrics_path: /metrics/osrs/worlds
    static_configs:
      - targets:
          - localhost:8000
```

## Metrics

### Steam Metrics

- `steam_owned_games_playtime_seconds{app_id, game_name, steam_id}` - Total playtime per game (in seconds)
- `steam_achievements_achieved{app_id, game_name, achievement_name, steam_id, achieved}` - Achievement status (0 or 1)

### OSRS Metrics

- `osrs_player_level{skill, player, profile}` - Player skill level
- `osrs_player_xp{skill, player, profile}` - Player experience points
- `osrs_player_rank{skill, player, profile}` - Player highscores rank
- `osrs_world_players{id, location, isMembers, type}` - Number of players in a world

## Building from Source

```bash
go build
```

## Docker Build

```bash
docker build -t game-stats-exporter .
```
