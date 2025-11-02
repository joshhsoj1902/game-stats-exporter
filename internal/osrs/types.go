package osrs

type PlayerProfile string

const (
	PlayerProfileStandard PlayerProfile = "Standard"
)

type SkillInfo struct {
	Rank    string        `json:"rank"`
	Level   string        `json:"level"`
	XP      string        `json:"xp"`
	Name    string        `json:"name"`
	Player  string        `json:"player"`
	Profile PlayerProfile `json:"profile"`
}

type WorldLocation string

const (
	WorldLocationGermany   WorldLocation = "Germany"
	WorldLocationUSA       WorldLocation = "USA"
	WorldLocationUK        WorldLocation = "UK"
	WorldLocationAustralia WorldLocation = "Australia"
	WorldLocationUnknown   WorldLocation = "Unknown"
)

type WorldType string

const (
	WorldTypeFreeToPlay        WorldType = "FreeToPlay"
	WorldTypeMembers           WorldType = "Members"
	WorldTypePVP               WorldType = "PVP"
	WorldTypeBounty            WorldType = "Bounty"
	WorldTypePVPArena          WorldType = "PVPArena"
	WorldTypeSkillTotal        WorldType = "SkillTotal"
	WorldTypeQuestSpeedrunning WorldType = "QuestSpeedrunning"
	WorldTypeHighRisk          WorldType = "HighRisk"
	WorldTypeLastManStanding   WorldType = "LastManStanding"
	WorldTypeNoSaveMode        WorldType = "NoSaveMode"
	WorldTypeTournament        WorldType = "Tournament"
	WorldTypeFreshStartWorld   WorldType = "FreshStartWorld"
	WorldTypeDeadman           WorldType = "Deadman"
	WorldTypeBeta              WorldType = "Beta"
	WorldTypeSoulWars          WorldType = "SoulWars"
	WorldTypeMinigame          WorldType = "Minigame"
	WorldTypeSeasonal          WorldType = "Seasonal"
	WorldTypeUnknown           WorldType = "Unknown"
)

type World struct {
	ID       uint16      `json:"id"`
	Types    []WorldType `json:"types"`
	Address  string      `json:"address"`
	Activity string      `json:"activity"`
	Location WorldLocation `json:"location"`
	Players  int16       `json:"players"`
}

func (w *World) IsMembers() bool {
	for _, t := range w.Types {
		if t == WorldTypeMembers {
			return true
		}
	}
	return false
}

func (w *World) WorldType() WorldType {
	// Priority order from Rust implementation
	types := w.Types
	for _, t := range types {
		switch t {
		case WorldTypeQuestSpeedrunning:
			return WorldTypeQuestSpeedrunning
		case WorldTypeHighRisk:
			return WorldTypeHighRisk
		case WorldTypeLastManStanding:
			return WorldTypeLastManStanding
		case WorldTypeBounty:
			return WorldTypeBounty
		case WorldTypePVP:
			return WorldTypePVP
		case WorldTypePVPArena:
			return WorldTypePVPArena
		case WorldTypeNoSaveMode:
			return WorldTypeNoSaveMode
		case WorldTypeDeadman:
			return WorldTypeDeadman
		case WorldTypeTournament:
			return WorldTypeTournament
		case WorldTypeSkillTotal:
			return WorldTypeSkillTotal
		case WorldTypeFreshStartWorld:
			return WorldTypeFreshStartWorld
		case WorldTypeMinigame:
			return WorldTypeMinigame
		case WorldTypeSoulWars:
			return WorldTypeSoulWars
		case WorldTypeSeasonal:
			return WorldTypeSeasonal
		}
	}

	if w.IsMembers() {
		return WorldTypeMembers
	}

	return WorldTypeFreeToPlay
}

