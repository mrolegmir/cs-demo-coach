package detect

import (
	"cmp"
	"slices"
	"strings"
	"time"

	"github.com/mrolegmir/cs-demo-coach/internal/match"
)

// Role is what one player did for the team in a match, beyond mistakes: how
// often they took the round's first fight, how many of their flashes
// blinded an enemy, and how much they played the AWP (ADR 17).
type Role struct {
	Player match.PlayerID `json:"player"`
	Rounds int            `json:"rounds"`
	// Openings are rounds whose first duel the player was in; OpeningWins
	// the ones they won.
	Openings    int `json:"openings"`
	OpeningWins int `json:"opening_wins"`
	Kills       int `json:"kills"`
	AWPKills    int `json:"awp_kills"`
	Flashes     int `json:"flashes"`
	// GoodFlashes are flashes that blinded an enemy for at least MinBlind.
	GoodFlashes int `json:"good_flashes"`
}

// Roles holds the thresholds for counting roles.
type Roles struct {
	// MinBlind is how long an enemy must be blinded for a flash to count.
	MinBlind time.Duration
}

// DefaultRoles returns the thresholds used by the service.
func DefaultRoles() Roles { return Roles{MinBlind: MinEnemyBlind} }

// Count returns every player's role, sorted by player.
func (d Roles) Count(m *match.Match) []Role {
	roles := map[match.PlayerID]*Role{}
	get := func(p match.PlayerID) *Role {
		if roles[p] == nil {
			roles[p] = &Role{Player: p}
		}
		return roles[p]
	}
	for ri := range m.Rounds {
		r := &m.Rounds[ri]
		for _, p := range r.Players {
			get(p.Player).Rounds++
		}
		if first, ok := openingKill(r); ok {
			get(first.Killer).Openings++
			get(first.Killer).OpeningWins++
			get(first.Victim).Openings++
		}
		for _, k := range r.Kills {
			if k.Killer == 0 || k.TeamKill || k.Killer == k.Victim {
				continue
			}
			get(k.Killer).Kills++
			if strings.EqualFold(k.Weapon, "AWP") {
				get(k.Killer).AWPKills++
			}
		}
		for _, f := range r.Flashes {
			if f.Thrower == 0 || f.Side == match.SideNone {
				continue // not linked to a player, as in the flash detector
			}
			get(f.Thrower).Flashes++
			if slices.ContainsFunc(f.Blinded, func(b match.Blinding) bool { return b.Side != f.Side && b.Duration >= d.MinBlind }) {
				get(f.Thrower).GoodFlashes++
			}
		}
	}
	out := []Role{}
	for _, p := range roles {
		out = append(out, *p)
	}
	slices.SortFunc(out, func(a, b Role) int { return cmp.Compare(a.Player, b.Player) })
	return out
}

// Derived recomputes what a report holds besides findings and stats, for
// reports made before those parts existed.
func Derived(m *match.Match) (pairs []Pair, roles []Role) {
	return TradePairs(m), DefaultRoles().Count(m)
}
