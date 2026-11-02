package detect

import (
	"fmt"
	"strings"
	"time"

	"github.com/mrolegmir/cs-demo-coach/internal/match"
)

// Flash flags flashbangs that did not blind any enemy for at least
// MinEnemyBlind, and flashbangs that blinded a teammate (or the thrower) for
// at least MinTeamBlind.
type Flash struct {
	MinEnemyBlind time.Duration
	MinTeamBlind  time.Duration
}

func (d Flash) Detect(m *match.Match) Result {
	var res Result
	for ri := range m.Rounds {
		r := &m.Rounds[ri]
		for _, f := range r.Flashes {
			if f.Thrower == 0 || f.Side == match.SideNone {
				continue
			}
			res.chance(KindFlashNoEnemy, f.Thrower)
			res.chance(KindTeamFlash, f.Thrower)

			var enemy time.Duration
			var team []match.PlayerID
			for _, b := range f.Blinded {
				switch {
				case b.Side != f.Side:
					enemy = max(enemy, b.Duration)
				case b.Duration >= d.MinTeamBlind:
					team = append(team, b.Player)
				}
			}
			if enemy < d.MinEnemyBlind {
				detail := "flash blinded no enemy"
				if enemy > 0 {
					detail = fmt.Sprintf("flash blinded enemies for only %.1fs", enemy.Seconds())
				}
				res.Findings = append(res.Findings, placed(finding(m, r, KindFlashNoEnemy, f.Thrower, f.Tick, detail), f.Place))
			}
			if len(team) > 0 {
				names := make([]string, 0, len(team))
				for _, p := range team {
					if p == f.Thrower {
						names = append(names, "self")
					} else {
						names = append(names, m.PlayerName(p))
					}
				}
				detail := "flash blinded own team: " + strings.Join(names, ", ")
				res.Findings = append(res.Findings, placed(finding(m, r, KindTeamFlash, f.Thrower, f.Tick, detail, team...), f.Place))
			}
		}
	}
	return res
}
