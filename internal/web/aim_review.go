package web

import (
	"time"

	"github.com/mrolegmir/cs-demo-coach/internal/match"
)

type mechanicsReviewView struct {
	Captured, TrajectoryCaptured bool
	Episodes                     []rifleEpisodeView
}

type rifleEpisodeView struct {
	Side                           string
	Round, Shots, Damage, FirstHit int
	Time                           time.Duration
	Weapon, Place, Target          string
	TargetKnown, Ambiguous         bool
	Distance                       float64
	Preparation                    []movementView
	Fires                          []fireView
	Contact                        []contactView
}

type fireView struct {
	Ordinal, AfterMS, WindowMS, Damage int
	Speed                              float64
	Known, Airborne                    bool
}

type contactView struct {
	BeforeMS                         int
	Speed, Angle                     float64
	SpeedKnown, AngleKnown, Airborne bool
}

func mechanicsReviewFor(m *match.Match, player match.PlayerID) *mechanicsReviewView {
	v := &mechanicsReviewView{Captured: m.MechanicsVersion >= 1 && m.MechanicsVersion <= 4, TrajectoryCaptured: m.MechanicsVersion == 4}
	for _, e := range m.RifleEpisodes(player) {
		row := rifleEpisodeView{Round: e.Round, Shots: e.Shots, Damage: e.HealthDamage, FirstHit: e.FirstHitShot, Time: roundTime(m, e.Round, e.Tick), Weapon: e.Weapon, Place: match.PlaceName(e.Place), Ambiguous: e.ContactAmbiguous, Side: e.Side.String()}
		for _, s := range e.Preparation {
			mv := movementView{BeforeMS: int(float64(e.Tick-s.Tick) * 1000 / m.TickRate), WindowMS: int(float64(s.WindowTicks) * 1000 / m.TickRate), Airborne: s.Airborne}
			if s.Speed != nil {
				mv.Known = true
				mv.Speed = *s.Speed
			}
			row.Preparation = append(row.Preparation, mv)
		}
		for _, s := range e.Fires {
			f := fireView{Ordinal: s.Ordinal, AfterMS: int(float64(s.Tick-e.Tick) * 1000 / m.TickRate), WindowMS: int(float64(s.WindowTicks) * 1000 / m.TickRate), Airborne: s.Airborne, Damage: s.HealthDamage}
			if s.Speed != nil {
				f.Known = true
				f.Speed = *s.Speed
			}
			row.Fires = append(row.Fires, f)
		}
		if e.Contact != nil {
			row.TargetKnown = true
			row.Target = m.PlayerName(e.Contact.Victim)
			row.Distance = e.Contact.Distance
			if v.TrajectoryCaptured {
				for _, s := range e.Contact.Preparation {
					c := contactView{BeforeMS: int(float64(e.Contact.Tick-s.Tick) * 1000 / m.TickRate), Airborne: s.Airborne}
					if s.Speed != nil {
						c.SpeedKnown = true
						c.Speed = *s.Speed
					}
					if angle := match.ViewToEyeAngle(s.View, s.TargetEye); angle != nil {
						c.AngleKnown = true
						c.Angle = *angle
					}
					row.Contact = append(row.Contact, c)
				}
			}
		}
		v.Episodes = append(v.Episodes, row)
	}
	return v
}
