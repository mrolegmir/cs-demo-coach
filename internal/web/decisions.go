package web

import (
	"cmp"
	"slices"
	"time"

	"github.com/mrolegmir/cs-demo-coach/internal/match"
)

type decisionsView struct {
	ObjectiveCaptured bool
	Advantages        []advantageView
	Rounds            []objectiveRoundView
}

type advantageView struct {
	match.AdvantageReview
	Time       time.Duration
	TimeKnown  bool
	Own, Enemy []string
	Deaths     []decisionDeathView
}

type decisionDeathView struct {
	Time                  time.Duration
	TimeKnown             bool
	Victim, Killer, Place string
	KillerKnown           bool
}

type objectiveRoundView struct {
	Round              int
	Side, Result, Site string
	PlantKnown         bool
	TAlive, CTAlive    int
	ViewerAlive        bool
	Events             []bombEventView
}

type bombEventView struct {
	Tick                             int
	Time, SincePlant                 time.Duration
	AfterPlant                       bool
	TimeKnown                        bool
	Action, Player, Site             string
	KitKnown, HasKit                 bool
	ActorKnown, NoActor, KitRelevant bool
}

func decisionsFor(m *match.Match, player match.PlayerID) *decisionsView {
	v := &decisionsView{ObjectiveCaptured: m.ObjectiveVersion == 1}
	for _, a := range m.LostAdvantages(player) {
		row := advantageView{AdvantageReview: a, Time: roundTime(m, a.Round, a.Tick), TimeKnown: m.TickRate > 0}
		for _, s := range a.Alive {
			if s.Side == a.Side {
				row.Own = append(row.Own, m.PlayerName(s.Player))
			} else {
				row.Enemy = append(row.Enemy, m.PlayerName(s.Player))
			}
		}
		for _, k := range a.After {
			death := decisionDeathView{Time: roundTime(m, a.Round, k.Tick), TimeKnown: m.TickRate > 0, Victim: m.PlayerName(k.Victim), Place: match.PlaceName(k.Place(k.Victim))}
			if k.Killer != 0 {
				death.KillerKnown = true
				death.Killer = m.PlayerName(k.Killer)
			}
			row.Deaths = append(row.Deaths, death)
		}
		v.Advantages = append(v.Advantages, row)
	}
	for _, o := range m.ObjectiveRounds(player) {
		row := objectiveRoundView{Round: o.Round, Side: o.Side.String(), Result: "Round result unavailable"}
		switch o.Winner {
		case o.Side:
			row.Result = "Won"
		case match.SideT, match.SideCT:
			row.Result = "Lost"
		}
		if o.Plant != nil {
			row.Site = o.Plant.Site
			row.PlantKnown = o.Plant.AliveComplete && len(o.Plant.Alive) > 0
			seen := map[match.PlayerID]bool{}
			for _, a := range o.Plant.Alive {
				if a.Player == 0 || seen[a.Player] {
					row.PlantKnown = false
				}
				seen[a.Player] = true
				switch a.Side {
				case match.SideT:
					row.TAlive++
				case match.SideCT:
					row.CTAlive++
				default:
					row.PlantKnown = false
				}
				if a.Player == player {
					row.ViewerAlive = true
				}
			}
		}
		for _, e := range o.Events {
			event := bombEventView{Tick: e.Tick, Time: roundTime(m, o.Round, e.Tick), TimeKnown: m.TickRate > 0, Action: bombLabel(e.Kind), Site: e.Site, KitRelevant: e.Kind == match.BombDefuseBegin}
			if e.Kind == match.BombExploded {
				event.NoActor = true
			} else if e.Player != 0 {
				event.Player = m.PlayerName(e.Player)
				event.ActorKnown = true
			}
			if o.Plant != nil && e.Tick >= o.Plant.Tick && m.TickRate > 0 {
				event.AfterPlant = true
				event.SincePlant = time.Duration(float64(e.Tick-o.Plant.Tick) / m.TickRate * float64(time.Second))
			}
			if e.HasKit != nil {
				event.KitKnown = true
				event.HasKit = *e.HasKit
			}
			row.Events = append(row.Events, event)
		}
		for _, k := range o.Deaths {
			event := bombEventView{Tick: k.Tick, Time: roundTime(m, o.Round, k.Tick), TimeKnown: m.TickRate > 0, Action: "Player death", Player: m.PlayerName(k.Victim), ActorKnown: true}
			if m.TickRate > 0 {
				event.AfterPlant = true
				event.SincePlant = time.Duration(float64(k.Tick-o.Plant.Tick) / m.TickRate * float64(time.Second))
			}
			if k.Tick == o.Plant.Tick {
				row.PlantKnown = false
			}
			row.Events = append(row.Events, event)
		}
		slices.SortStableFunc(row.Events, func(a, b bombEventView) int { return cmp.Compare(a.Tick, b.Tick) })
		v.Rounds = append(v.Rounds, row)
	}
	return v
}

func roundTime(m *match.Match, round, tick int) time.Duration {
	for i := range m.Rounds {
		if m.Rounds[i].Number == round {
			return m.Time(&m.Rounds[i], tick)
		}
	}
	return 0
}

func bombLabel(kind string) string {
	switch kind {
	case match.BombPlantBegin:
		return "Plant started"
	case match.BombPlantAborted:
		return "Plant interrupted"
	case match.BombPlanted:
		return "Bomb planted"
	case match.BombDefuseBegin:
		return "Defuse started"
	case match.BombDefuseAborted:
		return "Defuse interrupted"
	case match.BombDefused:
		return "Bomb defused"
	case match.BombExploded:
		return "Bomb exploded"
	default:
		return "Unrecognized bomb event"
	}
}
