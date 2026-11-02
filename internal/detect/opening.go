package detect

import (
	"fmt"
	"time"

	"github.com/mrolegmir/cs-demo-coach/internal/match"
)

// OpeningDeath flags the first death of a round when the victim's team had
// not flashed an enemy in the SupportWindow before it. Dying first without
// utility support hands the enemy a 5v4.
type OpeningDeath struct {
	SupportWindow time.Duration
}

func (d OpeningDeath) Detect(m *match.Match) Result {
	var res Result
	window := ticks(m, d.SupportWindow)
	for ri := range m.Rounds {
		r := &m.Rounds[ri]
		for _, p := range r.Players {
			res.chance(KindOpeningDeath, p.Player)
		}
		first, ok := openingKill(r)
		if !ok {
			continue
		}
		side := r.Side(first.Victim)
		if supported(r, side, first.Tick, window) {
			continue
		}
		detail := fmt.Sprintf("died first to %s (%s) without a teammate flash on the enemy",
			m.PlayerName(first.Killer), first.Weapon)
		res.Findings = append(res.Findings, placed(finding(m, r, KindOpeningDeath, first.Victim, first.Tick, detail), first.Place(first.Victim)))
	}
	return res
}

// openingKill returns the round's first kill if it was an opening duel. A
// round opened by a suicide, team kill or world damage has no opening duel.
func openingKill(r *match.Round) (match.Kill, bool) {
	if len(r.Kills) == 0 {
		return match.Kill{}, false
	}
	k := r.Kills[0]
	ok := k.Killer != 0 && k.Victim != 0 && !k.TeamKill && k.Killer != k.Victim
	return k, ok
}

// supported reports whether side flashed an enemy shortly before tick.
func supported(r *match.Round, side match.Side, tick, window int) bool {
	for _, f := range r.Flashes {
		if f.Side != side || f.Tick > tick || tick-f.Tick > window {
			continue
		}
		for _, b := range f.Blinded {
			if b.Side != side {
				return true
			}
		}
	}
	return false
}
