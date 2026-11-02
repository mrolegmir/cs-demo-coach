package detect

import (
	"cmp"
	"fmt"
	"slices"
	"time"

	"github.com/mrolegmir/cs-demo-coach/internal/match"
)

// Trade looks at deaths the victim's team did not avenge: the killer
// survived Window after the kill. Such a death is one of two mistakes:
//
//   - isolated death: no teammate was within Distance, so nobody could trade.
//     The victim gets the finding.
//   - missed trade: a teammate was within Distance and did not trade. The
//     nearest such teammate gets the finding.
//
// Deaths with no living teammate are ignored, since nobody could trade them.
// Distance is a straight line and ignores walls, so it marks a teammate as
// "close", not as "had line of sight".
type Trade struct {
	Window   time.Duration
	Distance float64
}

func (d Trade) Detect(m *match.Match) Result {
	var res Result
	pairs := map[[2]match.PlayerID]*Pair{}
	window := ticks(m, d.Window)
	for ri := range m.Rounds {
		r := &m.Rounds[ri]
		for ki, k := range r.Kills {
			if k.Killer == 0 || k.TeamKill || k.Victim == 0 || k.Killer == k.Victim {
				continue
			}
			victim, ok := state(k.Alive, k.Victim)
			if !ok {
				continue
			}
			nearest, dist, ok := nearestMate(k.Alive, victim)
			if !ok {
				continue
			}
			close := dist <= d.Distance
			res.chance(KindIsolatedDeath, k.Victim)
			wasTraded := traded(r.Kills[ki+1:], k, victim.Side, window)
			if close {
				res.chance(KindMissedTrade, nearest)
				key := [2]match.PlayerID{k.Victim, nearest}
				if pairs[key] == nil {
					pairs[key] = &Pair{Player: k.Victim, Partner: nearest}
				}
				pairs[key].Chances++
				if wasTraded {
					pairs[key].Traded++
				}
			}
			if wasTraded {
				continue
			}

			if close {
				detail := fmt.Sprintf("%s was killed by %s %.0fu away; no trade within %s",
					m.PlayerName(k.Victim), m.PlayerName(k.Killer), dist, d.Window)
				res.Findings = append(res.Findings, placed(finding(m, r, KindMissedTrade, nearest, k.Tick, detail, k.Victim), k.Place(nearest)))
				continue
			}
			detail := fmt.Sprintf("killed by %s with no teammate within %.0fu (nearest: %s, %.0fu)",
				m.PlayerName(k.Killer), d.Distance, m.PlayerName(nearest), dist)
			res.Findings = append(res.Findings, placed(finding(m, r, KindIsolatedDeath, k.Victim, k.Tick, detail, nearest), k.Place(k.Victim)))
		}
	}
	for _, p := range pairs {
		res.Pairs = append(res.Pairs, *p)
	}
	slices.SortFunc(res.Pairs, func(a, b Pair) int {
		return cmp.Or(cmp.Compare(a.Player, b.Player), cmp.Compare(a.Partner, b.Partner))
	})
	return res
}

// TradePairs recomputes the pairs of a stored match, for reports made
// before pairs were counted.
func TradePairs(m *match.Match) []Pair {
	if ps := DefaultTrade().Detect(m).Pairs; ps != nil {
		return ps
	}
	return []Pair{}
}

// nearestMate returns the living teammate closest to the victim.
func nearestMate(alive []match.PlayerState, victim match.PlayerState) (match.PlayerID, float64, bool) {
	var best match.PlayerID
	bestDist, found := 0.0, false
	for _, a := range alive {
		if a.Side != victim.Side || a.Player == victim.Player || a.Player == 0 {
			continue
		}
		if dist := a.Pos.Dist(victim.Pos); !found || dist < bestDist {
			best, bestDist, found = a.Player, dist, true
		}
	}
	return best, bestDist, found
}

// traded reports whether a later kill avenged k: the killer died to the
// victim's side within window ticks.
func traded(later []match.Kill, k match.Kill, side match.Side, window int) bool {
	for _, l := range later {
		if l.Tick-k.Tick > window {
			return false
		}
		if l.Victim != k.Killer || l.TeamKill {
			continue
		}
		if s, ok := state(l.Alive, l.Killer); ok && s.Side == side {
			return true
		}
	}
	return false
}

func state(alive []match.PlayerState, id match.PlayerID) (match.PlayerState, bool) {
	for _, a := range alive {
		if a.Player == id {
			return a, true
		}
	}
	return match.PlayerState{}, false
}
