package polling

import (
	"context"
	"fmt"
	"sync"
	"time"
)

type SteamCollector interface {
	Collect(steamId string) error
	IsActive(steamId string) (bool, error)
}

type OSRSCollector interface {
	CollectPlayerStats(rsn string, mode string) error
	CollectWorldData() error
	IsActive(rsn string) (bool, error)
}

type Manager struct {
	steamCollector   SteamCollector
	osrsCollector    OSRSCollector
	normalInterval   time.Duration
	activeInterval   time.Duration

	// Track registered users/players
	steamUsers       map[string]*userState
	osrsPlayers      map[string]*playerState

	mu               sync.RWMutex
	ctx              context.Context
	cancel           context.CancelFunc
	wg               sync.WaitGroup
}

type userState struct {
	lastActive bool
	lastPoll   time.Time
	mu         sync.Mutex
}

type playerState struct {
	lastActive bool
	lastPoll   time.Time
	mu         sync.Mutex
}

func NewManager(steamCollector SteamCollector, osrsCollector OSRSCollector, normalInterval, activeInterval time.Duration) *Manager {
	ctx, cancel := context.WithCancel(context.Background())
	return &Manager{
		steamCollector: steamCollector,
		osrsCollector:  osrsCollector,
		normalInterval: normalInterval,
		activeInterval: activeInterval,
		steamUsers:      make(map[string]*userState),
		osrsPlayers:     make(map[string]*playerState),
		ctx:             ctx,
		cancel:          cancel,
	}
}

// RegisterSteamUser registers a Steam user for background polling
func (m *Manager) RegisterSteamUser(steamId string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, exists := m.steamUsers[steamId]; !exists {
		m.steamUsers[steamId] = &userState{
			lastActive: false,
			lastPoll:   time.Now(),
		}

		// Start polling goroutine for this user
		m.wg.Add(1)
		go m.pollSteamUser(steamId)
	}
}

// RegisterOSRSPlayer registers an OSRS player for background polling
func (m *Manager) RegisterOSRSPlayer(rsn string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, exists := m.osrsPlayers[rsn]; !exists {
		m.osrsPlayers[rsn] = &playerState{
			lastActive: false,
			lastPoll:   time.Now(),
		}

		// Start polling goroutine for this player
		m.wg.Add(1)
		go m.pollOSRSPlayer(rsn)
	}
}

// pollSteamUser polls a Steam user with adaptive interval
func (m *Manager) pollSteamUser(steamId string) {
	defer m.wg.Done()

	m.mu.RLock()
	state, exists := m.steamUsers[steamId]
	m.mu.RUnlock()

	if !exists {
		return
	}

	ticker := time.NewTicker(m.normalInterval)
	defer ticker.Stop()

	for {
		select {
		case <-m.ctx.Done():
			return
		case <-ticker.C:
			// Collect data
			err := m.steamCollector.Collect(steamId)
			if err != nil {
				fmt.Printf("Error collecting Steam data for %s: %v\n", steamId, err)
			}

			// Check if user is active
			active, err := m.steamCollector.IsActive(steamId)
			if err != nil {
				fmt.Printf("Error checking Steam activity for %s: %v\n", steamId, err)
			} else {
				state.mu.Lock()
				state.lastActive = active
				state.lastPoll = time.Now()

				// Adjust polling interval based on activity
				if active {
					ticker.Reset(m.activeInterval)
				} else {
					ticker.Reset(m.normalInterval)
				}
				state.mu.Unlock()
			}
		}
	}
}

// pollOSRSPlayer polls an OSRS player with adaptive interval
func (m *Manager) pollOSRSPlayer(rsn string) {
	defer m.wg.Done()

	m.mu.RLock()
	state, exists := m.osrsPlayers[rsn]
	m.mu.RUnlock()

	if !exists {
		return
	}

	ticker := time.NewTicker(m.normalInterval)
	defer ticker.Stop()

	for {
		select {
		case <-m.ctx.Done():
			return
		case <-ticker.C:
			// Collect data (default to "vanilla" mode for background polling)
			err := m.osrsCollector.CollectPlayerStats(rsn, "vanilla")
			if err != nil {
				fmt.Printf("Error collecting OSRS data for %s: %v\n", rsn, err)
			}

			// Check if player is active
			active, err := m.osrsCollector.IsActive(rsn)
			if err != nil {
				fmt.Printf("Error checking OSRS activity for %s: %v\n", rsn, err)
			} else {
				state.mu.Lock()
				state.lastActive = active
				state.lastPoll = time.Now()

				// Adjust polling interval based on activity
				if active {
					ticker.Reset(m.activeInterval)
				} else {
					ticker.Reset(m.normalInterval)
				}
				state.mu.Unlock()
			}
		}
	}
}

// StartWorldDataPolling starts background polling for OSRS world data
func (m *Manager) StartWorldDataPolling() {
	m.wg.Add(1)
	go func() {
		defer m.wg.Done()

		ticker := time.NewTicker(5 * time.Minute) // World data changes frequently
		defer ticker.Stop()

		for {
			select {
			case <-m.ctx.Done():
				return
			case <-ticker.C:
				err := m.osrsCollector.CollectWorldData()
				if err != nil {
					fmt.Printf("Error collecting OSRS world data: %v\n", err)
				}
			}
		}
	}()
}

// Stop stops all polling
func (m *Manager) Stop() {
	m.cancel()
	m.wg.Wait()
}

