package detect

import (
	"fmt"
	"strings"

	"github.com/mrolegmir/cs-demo-coach/internal/match"
)

// BuyDesync looks at rounds where most of a team full-bought or most saved,
// and flags whoever was out of line. Pistol rounds are skipped. A buy is
// "full" from FullBuy equipment value, "eco" below Eco, and a player counts
// as left without a real buy below Under (no rifle with armor).
//
//   - buy_desync: the player bought less than Under while at least majority
//     teammates full-bought although they had the money for a full buy, or
//     full-bought while at least majority teammates saved.
//   - missed_drop: in a round the team full-bought, the player kept at least
//     DropMoney after buying (a chance to drop), and a teammate who could not
//     afford a buy stayed below Under. Every teammate who could have dropped
//     shares the finding; whoever had the money had the chance.
//
// Matchmaking parties rarely split their buys outright; missing drops are the
// common economy mistake, which is why both are tracked.
type BuyDesync struct {
	FullBuy   int
	Eco       int
	Under     int
	DropMoney int
}

// majority is how many players must agree on a buy for the rest to count as
// out of line.
const majority = 3

func (d BuyDesync) Detect(m *match.Match) Result {
	var res Result
	for ri := range m.Rounds {
		r := &m.Rounds[ri]
		if pistolRound(m, ri) {
			continue
		}
		for _, side := range []match.Side{match.SideT, match.SideCT} {
			var team, full, eco []match.PlayerRound
			for _, p := range r.Players {
				if p.Side != side {
					continue
				}
				team = append(team, p)
				switch {
				case p.EquipmentValue >= d.FullBuy:
					full = append(full, p)
				case p.EquipmentValue < d.Eco:
					eco = append(eco, p)
				}
			}
			for _, p := range team {
				res.chance(KindBuyDesync, p.Player)
			}
			switch {
			case len(full) >= majority:
				d.teamBought(m, r, team, full, &res)
			case len(eco) >= majority:
				for _, p := range full {
					detail := fmt.Sprintf("full-bought ($%d of equipment) while %d teammates saved", p.EquipmentValue, len(eco))
					res.Findings = append(res.Findings, finding(m, r, KindBuyDesync, p.Player, r.FreezeEndTick, detail))
				}
			}
		}
	}
	return res
}

// teamBought handles a round where most of the team full-bought.
func (d BuyDesync) teamBought(m *match.Match, r *match.Round, team, full []match.PlayerRound, res *Result) {
	var broke []match.PlayerRound
	for _, p := range team {
		if p.EquipmentValue >= d.Under {
			continue
		}
		if available := p.Money + p.Spent; available >= d.FullBuy {
			detail := fmt.Sprintf("bought only $%d of equipment with $%d available while %d teammates full-bought",
				p.EquipmentValue, available, len(full))
			res.Findings = append(res.Findings, finding(m, r, KindBuyDesync, p.Player, r.FreezeEndTick, detail))
			continue
		}
		broke = append(broke, p)
	}

	for _, f := range full {
		if f.Money < d.DropMoney {
			continue
		}
		res.chance(KindMissedDrop, f.Player)
		if len(broke) == 0 {
			continue
		}
		names := make([]string, 0, len(broke))
		related := make([]match.PlayerID, 0, len(broke))
		for _, b := range broke {
			names = append(names, fmt.Sprintf("%s ($%d)", m.PlayerName(b.Player), b.EquipmentValue))
			related = append(related, b.Player)
		}
		detail := fmt.Sprintf("kept $%d after buying while %s stayed without a rifle buy", f.Money, strings.Join(names, ", "))
		res.Findings = append(res.Findings, finding(m, r, KindMissedDrop, f.Player, r.FreezeEndTick, detail, related...))
	}
}

// pistolRound reports whether round i opens a half: the first round, or a
// round where players switched sides.
func pistolRound(m *match.Match, i int) bool {
	if i == 0 {
		return true
	}
	prev := &m.Rounds[i-1]
	for _, p := range m.Rounds[i].Players {
		if s := prev.Side(p.Player); s != match.SideNone && s != p.Side {
			return true
		}
	}
	return false
}
