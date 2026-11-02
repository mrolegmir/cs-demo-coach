package detect

import (
	"fmt"
	"strings"
	"time"

	"github.com/mrolegmir/cs-demo-coach/internal/match"
)

// UnusedUtility flags deaths where the victim still carried grenades (decoys
// aside): money spent on utility that never affected the round.
type UnusedUtility struct{}

func (UnusedUtility) Detect(m *match.Match) Result {
	var res Result
	for ri := range m.Rounds {
		r := &m.Rounds[ri]
		for _, k := range r.Kills {
			if k.Victim == 0 {
				continue
			}
			res.chance(KindUnusedUtility, k.Victim)
			var nades []string
			for _, u := range k.VictimUtility {
				if !strings.Contains(strings.ToLower(u), "decoy") {
					nades = append(nades, u)
				}
			}
			if len(nades) == 0 {
				continue
			}
			detail := "died holding " + strings.Join(nades, ", ")
			res.Findings = append(res.Findings, placed(finding(m, r, KindUnusedUtility, k.Victim, k.Tick, detail), k.Place(k.Victim)))
		}
	}
	return res
}

// NoUtility flags full-buy rounds where the player stayed alive for at least
// MinLife after freeze time (in a round that lasted that long) and threw no
// grenade at all. Players who rarely
// throw anything never show up in UnusedUtility (they die without nades
// because they never bought them, or survive holding them), so this catches
// the other half of the problem.
type NoUtility struct {
	FullBuy int
	MinLife time.Duration
}

func (d NoUtility) Detect(m *match.Match) Result {
	var res Result
	minLife := ticks(m, d.MinLife)
	for ri := range m.Rounds {
		r := &m.Rounds[ri]
		if r.EndTick-r.FreezeEndTick < minLife {
			continue // nobody had the time
		}
		thrown := map[match.PlayerID]int{}
		for _, g := range r.Grenades {
			thrown[g.Thrower]++
		}
		died := map[match.PlayerID]int{}
		for _, k := range r.Kills {
			died[k.Victim] = k.Tick
		}
		for _, p := range r.Players {
			if p.EquipmentValue < d.FullBuy {
				continue
			}
			if tick, ok := died[p.Player]; ok && tick-r.FreezeEndTick < minLife {
				continue
			}
			res.chance(KindNoUtility, p.Player)
			if thrown[p.Player] > 0 {
				continue
			}
			detail := fmt.Sprintf("threw no grenade all round with $%d of equipment", p.EquipmentValue)
			res.Findings = append(res.Findings, finding(m, r, KindNoUtility, p.Player, r.FreezeEndTick, detail))
		}
	}
	return res
}
