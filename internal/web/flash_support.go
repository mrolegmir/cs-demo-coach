package web

import (
	"time"

	"github.com/mrolegmir/cs-demo-coach/internal/match"
)

type flashSupportView struct {
	TimingAvailable                                                  bool
	TeammateKills, SupportedKills, AmbiguousKills, UnclassifiedKills int
	Flashes                                                          []flashReviewView
	Kills                                                            []flashKillView
}

type flashReviewView struct {
	Round                               int
	Time                                time.Duration
	TimeKnown                           bool
	Place                               string
	EnemyTargets, UnclassifiedBlindings int
	Blinds                              []flashBlindView
}

type flashBlindView struct {
	Player, Relation           string
	PlayerKnown, DurationKnown bool
	Seconds                    float64
}

type flashKillView struct {
	Round                 int
	Time                  time.Duration
	Killer, Victim, State string
	Sources               []flashSourceView
	DelayKnown            bool
	Seconds               float64
}

type flashSourceView struct {
	Player string
	Known  bool
	Time   time.Duration
}

func flashSupportFor(m *match.Match, player match.PlayerID) *flashSupportView {
	rep := m.FlashSupportFor(player)
	v := &flashSupportView{TimingAvailable: rep.TimingAvailable, TeammateKills: rep.TeammateKills, SupportedKills: rep.SupportedKills, AmbiguousKills: rep.AmbiguousKills, UnclassifiedKills: rep.UnclassifiedKills}
	for _, f := range rep.Flashes {
		row := flashReviewView{Round: f.Round, Time: roundTime(m, f.Round, f.Tick), TimeKnown: rep.TimingAvailable, Place: match.PlaceName(f.Place), EnemyTargets: f.EnemyTargets, UnclassifiedBlindings: f.UnclassifiedBlindings}
		for _, b := range f.Blinded {
			target := flashBlindView{Relation: "Unknown", PlayerKnown: b.Player != 0, DurationKnown: b.Duration >= 0, Seconds: b.Duration.Seconds()}
			if target.PlayerKnown {
				target.Player = m.PlayerName(b.Player)
			}
			switch {
			case b.Player != 0 && b.Player == f.Thrower:
				target.Relation = "Self"
			case b.Player != 0 && (f.Side == match.SideT || f.Side == match.SideCT) && (b.Side == match.SideT || b.Side == match.SideCT):
				if b.Side == f.Side {
					target.Relation = "Teammate"
				} else {
					target.Relation = "Enemy"
				}
			}
			row.Blinds = append(row.Blinds, target)
		}
		v.Flashes = append(v.Flashes, row)
	}
	for _, k := range rep.Kills {
		row := flashKillView{Round: k.Round, Time: roundTime(m, k.Round, k.Tick), Killer: m.PlayerName(k.Killer), Victim: m.PlayerName(k.Victim), State: "No eligible attribution", DelayKnown: k.State == match.FlashUnique, Seconds: k.Delay.Seconds()}
		switch k.State {
		case match.FlashUnique:
			row.State = "One eligible flash"
		case match.FlashAmbiguous:
			row.State = "Multiple possible flashes"
		}
		for _, s := range k.Sources {
			source := flashSourceView{Known: s.Thrower != 0, Time: roundTime(m, k.Round, s.Tick)}
			if source.Known {
				source.Player = m.PlayerName(s.Thrower)
			}
			row.Sources = append(row.Sources, source)
		}
		v.Kills = append(v.Kills, row)
	}
	return v
}
