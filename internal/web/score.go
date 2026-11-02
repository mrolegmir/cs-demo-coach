package web

import (
	"github.com/mrolegmir/cs-demo-coach/internal/match"
	"github.com/mrolegmir/cs-demo-coach/internal/store"
)

type resultView struct {
	Known                  bool
	Won, Lost, TWon, CTWon int
	Outcome, Symbol        string
	Rounds                 []roundView
}

type roundView struct {
	Number, Kills         int
	Side, Outcome, Symbol string
	Switch                bool
}

// resultFor follows the player's side each round, including overtime switches.
// Without a participant's perspective, no win/loss result is invented.
func resultFor(rounds []match.Round, id match.PlayerID) resultView {
	v := resultView{Known: id != 0 && len(rounds) > 0}
	for _, r := range rounds {
		if r.Winner != match.SideT && r.Winner != match.SideCT {
			v.Known = false
			continue
		}
		side := r.Side(id)
		if side != match.SideT && side != match.SideCT {
			v.Known = false
			continue
		}
		rv := roundView{Number: r.Number, Side: side.String(), Outcome: "Defeat", Symbol: "−"}
		if len(v.Rounds) > 0 {
			rv.Switch = v.Rounds[len(v.Rounds)-1].Side != rv.Side
		}
		for _, k := range r.Kills {
			if k.Killer == id && k.Victim != id && !k.TeamKill {
				rv.Kills++
			}
		}
		if side == r.Winner {
			v.Won++
			rv.Outcome, rv.Symbol = "Victory", "+"
			if side == match.SideT {
				v.TWon++
			} else {
				v.CTWon++
			}
		} else {
			v.Lost++
		}
		v.Rounds = append(v.Rounds, rv)
	}
	if v.Known {
		switch {
		case v.Won > v.Lost:
			v.Outcome, v.Symbol = "Victory", "✓"
		case v.Won < v.Lost:
			v.Outcome, v.Symbol = "Defeat", "✕"
		default:
			v.Outcome, v.Symbol = "Draw", "="
		}
	}
	return v
}

type matchListView struct {
	store.Match
	Result resultView
}

type scoreboardView struct {
	Name                                       string
	Group                                      string
	Self                                       bool
	Kills, Deaths, Assists, Headshots, Flashes int
}

func scoreboard(m *match.Match, viewer match.PlayerID) []scoreboardView {
	var out []scoreboardView
	for _, pl := range m.Players {
		v := scoreboardView{Name: pl.Name, Self: pl.ID == viewer}
		v.Group = "Players"
		if own := m.StartSide(viewer); own != match.SideNone {
			if m.StartSide(pl.ID) == own {
				v.Group = "Your team"
			} else {
				v.Group = "Opponents"
			}
		}
		for _, r := range m.Rounds {
			for _, k := range r.Kills {
				if k.Victim == pl.ID {
					v.Deaths++
				}
				if k.TeamKill || k.Killer == k.Victim {
					continue
				}
				if k.Killer == pl.ID {
					v.Kills++
					if k.Headshot {
						v.Headshots++
					}
				}
				if k.Assister == pl.ID {
					v.Assists++
				}
			}
			for _, g := range r.Grenades {
				if g.Thrower == pl.ID && g.Type == "Flashbang" {
					v.Flashes++
				}
			}
		}
		out = append(out, v)
	}
	sortScoreboard(out)
	return out
}
