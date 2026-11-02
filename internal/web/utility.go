package web

import (
	"time"

	"github.com/mrolegmir/cs-demo-coach/internal/match"
)

type utilityView struct {
	Captured        bool
	DamageAvailable bool
	Summary         []match.UtilitySummary
	Evidence        []utilityEvidenceView
}

type utilityEvidenceView struct {
	Round                                 int
	Time                                  time.Duration
	Side, Family, Victim, Place, Relation string
	Known                                 bool
	Damage                                int
}

func utilityFor(m *match.Match, player match.PlayerID) *utilityView {
	rep := m.UtilityFor(player)
	v := &utilityView{Captured: rep.Captured, DamageAvailable: rep.DamageAvailable, Summary: rep.Summary}
	for _, e := range rep.Evidence {
		row := utilityEvidenceView{Round: e.Round, Side: e.AttackerSide.String(), Family: match.UtilityFamily(e.Weapon), Victim: m.PlayerName(e.Victim), Place: match.PlaceName(e.VictimPlace), Relation: e.Relation()}
		if e.HealthDamage != nil && *e.HealthDamage >= 0 {
			row.Known, row.Damage = true, *e.HealthDamage
		}
		for i := range m.Rounds {
			if m.Rounds[i].Number == e.Round {
				row.Time = m.Time(&m.Rounds[i], e.Tick)
				break
			}
		}
		v.Evidence = append(v.Evidence, row)
	}
	return v
}
