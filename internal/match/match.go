// Package match defines the parser-independent model of a CS2 match that
// detectors work on. It is produced by package demo and is plain data, so
// detector tests can build matches by hand or load them from JSON fixtures.
package match

import (
	"fmt"
	"math"
	"strings"
	"time"
	"unicode"
)

// PlayerID is a player's SteamID64.
type PlayerID uint64

// Side is the team a player is on in a given round.
type Side uint8

const (
	SideNone Side = iota
	SideT
	SideCT
)

func (s Side) String() string {
	switch s {
	case SideT:
		return "T"
	case SideCT:
		return "CT"
	default:
		return "none"
	}
}

// Vec is a position in world units.
type Vec struct {
	X, Y, Z float64
}

// Dist returns the euclidean distance between two positions.
func (v Vec) Dist(o Vec) float64 {
	dx, dy, dz := v.X-o.X, v.Y-o.Y, v.Z-o.Z
	return math.Sqrt(dx*dx + dy*dy + dz*dz)
}

// Match is one parsed demo.
type Match struct {
	// MechanicsVersion is zero for reports parsed before shot capture existed.
	MechanicsVersion int      `json:"mechanics_version,omitzero"`
	UtilityVersion   int      `json:"utility_version,omitzero"`
	ObjectiveVersion int      `json:"objective_version,omitzero"`
	ID               string   `json:"id"`
	Map              string   `json:"map"`
	TickRate         float64  `json:"tick_rate"`
	Players          []Player `json:"players"`
	Rounds           []Round  `json:"rounds"`
}

// Player is a participant of the match.
type Player struct {
	ID   PlayerID `json:"id"`
	Name string   `json:"name"`
}

// PlayerName returns the name of the player with the given ID, or the ID
// itself if the player is unknown.
func (m *Match) PlayerName(id PlayerID) string {
	for _, p := range m.Players {
		if p.ID == id {
			return p.Name
		}
	}
	return fmt.Sprint(uint64(id))
}

// Round is one played round. All event slices are sorted by tick.
type Round struct {
	Number int `json:"number"`
	// StartTick is the round start, FreezeEndTick the moment players can
	// move, EndTick the moment the winner is decided.
	StartTick     int    `json:"start_tick"`
	FreezeEndTick int    `json:"freeze_end_tick"`
	EndTick       int    `json:"end_tick"`
	Winner        Side   `json:"winner"`
	EndReason     string `json:"end_reason,omitempty"`

	Players       []PlayerRound   `json:"players"`
	Kills         []Kill          `json:"kills,omitempty"`
	Flashes       []Flash         `json:"flashes,omitempty"`
	Grenades      []Grenade       `json:"grenades,omitempty"`
	Shots         []Shot          `json:"shots,omitempty"`
	Damage        []Damage        `json:"damage,omitempty"`
	UtilityDamage []UtilityDamage `json:"utility_damage,omitempty"`
	BombEvents    []BombEvent     `json:"bomb_events,omitempty"`
}

// PlayerRound is a player's state at the end of freeze time, after buying.
type PlayerRound struct {
	Player PlayerID `json:"player"`
	Side   Side     `json:"side"`
	// Money is what the player has left after buying.
	Money int `json:"money"`
	// Spent is what the player spent during freeze time.
	Spent int `json:"spent"`
	// EquipmentValue is the value of everything the player carries.
	EquipmentValue int `json:"equipment_value"`
}

// Kill is a player death. Killer is zero for world damage and suicides.
type Kill struct {
	Tick     int      `json:"tick"`
	Killer   PlayerID `json:"killer,omitzero"`
	Victim   PlayerID `json:"victim"`
	Assister PlayerID `json:"assister,omitzero"`
	Weapon   string   `json:"weapon"`
	Headshot bool     `json:"headshot,omitzero"`
	Wallbang bool     `json:"wallbang,omitzero"`
	TeamKill bool     `json:"team_kill,omitzero"`
	// VictimUtility lists grenades the victim still carried.
	VictimUtility []string `json:"victim_utility,omitempty"`
	// Alive is every player alive right before the kill, victim included.
	Alive []PlayerState `json:"alive"`
}

// Place returns where a player alive at the kill stood, or "".
func (k Kill) Place(p PlayerID) string {
	for _, a := range k.Alive {
		if a.Player == p {
			return a.Place
		}
	}
	return ""
}

// PlayerState is a snapshot of one player at a point in time.
type PlayerState struct {
	Player PlayerID `json:"player"`
	Side   Side     `json:"side"`
	Pos    Vec      `json:"pos"`
	// Place is the map's name for where the player stands ("BombsiteB",
	// "Apartments"), as the game reports it; empty on maps without names.
	Place string `json:"place,omitzero"`
}

// Flash is a detonated flashbang and everyone it blinded.
type Flash struct {
	Tick    int      `json:"tick"`
	Thrower PlayerID `json:"thrower"`
	Side    Side     `json:"side"`
	Pos     Vec      `json:"pos"`
	// Place is where the thrower stood when they threw it (see PlayerState).
	Place   string     `json:"place,omitzero"`
	Blinded []Blinding `json:"blinded,omitempty"`
}

// Blinding is one player blinded by a flash.
type Blinding struct {
	Player   PlayerID      `json:"player"`
	Side     Side          `json:"side"`
	Duration time.Duration `json:"duration"`
}

// Grenade is a thrown grenade of any type.
type Grenade struct {
	Tick    int      `json:"tick"`
	Thrower PlayerID `json:"thrower"`
	Type    string   `json:"type"`
}

// Time converts a tick to time elapsed since the round went live.
func (m *Match) Time(r *Round, tick int) time.Duration {
	if m.TickRate <= 0 {
		return 0
	}
	return time.Duration(float64(tick-r.FreezeEndTick) / m.TickRate * float64(time.Second))
}

// StartSide returns the side a player had in the first round they played,
// or SideNone if they never played.
func (m *Match) StartSide(id PlayerID) Side {
	for i := range m.Rounds {
		if s := m.Rounds[i].Side(id); s != SideNone {
			return s
		}
	}
	return SideNone
}

// Clock formats a duration as m:ss, rounded to the second.
func Clock(d time.Duration) string {
	d = max(d, 0).Round(time.Second)
	return fmt.Sprintf("%d:%02d", int(d.Minutes()), int(d.Seconds())%60)
}

// Side returns the side of a player in the round, or SideNone.
func (r *Round) Side(id PlayerID) Side {
	for _, p := range r.Players {
		if p.Player == id {
			return p.Side
		}
	}
	return SideNone
}

// Anonymize replaces player IDs and names with stable placeholders (1, 2, ...
// and player01, player02, ...) assigned in the order of Players. Use it before
// committing a match as a test fixture.
func (m *Match) Anonymize() {
	ids := make(map[PlayerID]PlayerID, len(m.Players))
	for i := range m.Players {
		n := PlayerID(i + 1)
		ids[m.Players[i].ID] = n
		m.Players[i] = Player{ID: n, Name: fmt.Sprintf("player%02d", n)}
	}
	sub := func(id *PlayerID) {
		if *id != 0 {
			*id = ids[*id]
		}
	}
	for ri := range m.Rounds {
		r := &m.Rounds[ri]
		for i := range r.Players {
			sub(&r.Players[i].Player)
		}
		for ki := range r.Kills {
			k := &r.Kills[ki]
			sub(&k.Killer)
			sub(&k.Victim)
			sub(&k.Assister)
			for i := range k.Alive {
				sub(&k.Alive[i].Player)
			}
		}
		for fi := range r.Flashes {
			f := &r.Flashes[fi]
			sub(&f.Thrower)
			for i := range f.Blinded {
				sub(&f.Blinded[i].Player)
			}
		}
		for gi := range r.Grenades {
			sub(&r.Grenades[gi].Thrower)
		}
		for si := range r.Shots {
			sub(&r.Shots[si].Player)
		}
		for di := range r.Damage {
			sub(&r.Damage[di].Attacker)
			sub(&r.Damage[di].Victim)
		}
		for di := range r.UtilityDamage {
			sub(&r.UtilityDamage[di].Attacker)
			sub(&r.UtilityDamage[di].Victim)
		}
		for bi := range r.BombEvents {
			sub(&r.BombEvents[bi].Player)
			for i := range r.BombEvents[bi].Alive {
				sub(&r.BombEvents[bi].Alive[i].Player)
			}
		}
	}
}

// placeNames are place names that splitting the words gets wrong.
var placeNames = map[string]string{
	"BombsiteA": "A site",
	"BombsiteB": "B site",
	"TopofMid":  "Top of Mid",
}

// PlaceName turns the game's place name ("LongA", "CTSpawn") into words
// ("Long A", "CT Spawn").
func PlaceName(raw string) string {
	if n, ok := placeNames[raw]; ok {
		return n
	}
	rs := []rune(raw)
	var b strings.Builder
	for i, r := range rs {
		if i > 0 && unicode.IsUpper(r) &&
			(unicode.IsLower(rs[i-1]) || unicode.IsDigit(rs[i-1]) || i+1 < len(rs) && unicode.IsLower(rs[i+1])) {
			b.WriteByte(' ')
		}
		b.WriteRune(r)
	}
	return b.String()
}

// MapName turns a map's file name into its name: "de_mirage" is "Mirage".
func MapName(raw string) string {
	_, name, ok := strings.Cut(raw, "_")
	if !ok || name == "" {
		return raw
	}
	return strings.ToUpper(name[:1]) + name[1:]
}
