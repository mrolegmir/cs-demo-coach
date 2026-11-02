package web

import (
	"fmt"
	"slices"
	"time"

	"github.com/mrolegmir/cs-demo-coach/internal/coach"
	"github.com/mrolegmir/cs-demo-coach/internal/detect"
	"github.com/mrolegmir/cs-demo-coach/internal/drills"
	"github.com/mrolegmir/cs-demo-coach/internal/history"
	"github.com/mrolegmir/cs-demo-coach/internal/match"
	"github.com/mrolegmir/cs-demo-coach/internal/store"
)

var kindLabels = map[detect.Kind]string{
	detect.KindIsolatedDeath: "Isolated death",
	detect.KindMissedTrade:   "Missed trade",
	detect.KindFlashNoEnemy:  "Wasted flash",
	detect.KindTeamFlash:     "Team flash",
	detect.KindUnusedUtility: "Unused utility",
	detect.KindBuyDesync:     "Buy desync",
	detect.KindMissedDrop:    "Missed drop",
	detect.KindNoUtility:     "No utility thrown",
	detect.KindOpeningDeath:  "Dry opening death",
}

// matchPage is the view model of the match page.
type matchPage struct {
	Viewer
	Match *store.Match
	// Notice is a message about the last action, e.g. an invalid party.
	Notice string
	// CanCoach shows the party picker; only the uploader may coach.
	CanCoach bool
	// Teams lists players by starting side for the party picker.
	Teams          []teamView
	Result         resultView
	Scoreboard     []scoreboardView
	Diagnostics    bool
	Categories     []skillCategory
	SkillPlan      []skillCheck
	ActivePractice []activePracticeView
	CalloutURL     string
	SuggestedParty []match.PlayerID
	PartyNames     []string

	Players                    []playerView
	Team                       []teamAdviceView
	Notes                      []string
	Findings                   []findingView
	MechanicsAvailable         bool
	Mechanics                  []mechanicsView
	DamageAvailable            bool
	DamageCaptured             bool
	DamageEvents, LinkedDamage int
	Shooting                   []shootingView
	MovementCaptured           bool
}

type shootingView struct {
	Name     string
	Round    int
	Time     time.Duration
	Movement []movementView
	match.ShootingSequence
}

type movementView struct {
	BeforeMS, WindowMS int
	Speed              float64
	Known              bool
	Airborne           bool
}

type mechanicsView struct {
	Name                                                                string
	Shots, SpeedSamples, MovingSamples, Sequences, Singles, Short, Long int
}

type teamView struct {
	Side    string
	Players []pickView
}

type pickView struct {
	ID      match.PlayerID
	Name    string
	InParty bool
}

type playerView struct {
	Name  string
	Focus []focusView
	Self  bool
}

type focusView struct {
	Title, Why, Drill, Priority string
	// Lib is the library drill, nil for advice made before the library.
	Lib *drills.Drill
	// Where lists the places this mistake happened most, e.g. "B site 4".
	Where    []string
	Evidence []findingView
}

// newFocusView converts a focus area; evidence and places are converted by
// the caller.
func newFocusView(f coach.Focus, evidence []findingView, places []coach.Place) focusView {
	v := focusView{Title: f.Title, Why: f.Why, Drill: f.Drill, Priority: f.Priority, Evidence: evidence, Where: whereText(places)}
	if d, ok := drills.ByID(f.DrillID); ok {
		v.Lib = &d
	}
	return v
}

type teamAdviceView struct {
	Title, Why, Suggestion string
	Evidence               []findingView
}

type findingView struct {
	// Link points at the finding in its match; empty on the match page itself.
	Link      string
	ID        string
	Round     int
	Time      time.Duration
	Player    string
	Kind      string
	Place     string // in words; empty for whole-round findings
	PlaceHint string
	Side      string
	Detail    string
}

// Pending reports whether the page should refresh itself.
func (p matchPage) Pending() bool {
	return p.Match.Status == store.StatusUploaded || p.Match.Status == store.StatusCoaching
}

func (p *matchPage) build(an *store.Analysis) {
	m := an.Match
	p.CalloutURL = calloutGuide(m.Map)
	p.buildSkills(an)
	p.Result = resultFor(m.Rounds, userID(p.User))
	p.Scoreboard = scoreboard(m, userID(p.User))
	p.MechanicsAvailable = m.MechanicsVersion >= 1 && m.MechanicsVersion <= 4
	p.DamageCaptured = m.MechanicsVersion >= 2 && m.MechanicsVersion <= 4
	p.MovementCaptured = m.MechanicsVersion >= 3 && m.MechanicsVersion <= 4
	for _, r := range m.Rounds {
		if p.DamageCaptured && len(r.Damage) > 0 {
			p.DamageAvailable = true
		}
		if !p.DamageCaptured {
			continue
		}
		for i, link := range r.DamageLinks() {
			d := r.Damage[i]
			if len(p.Match.Party) > 0 && !slices.Contains(p.Match.Party, d.Attacker) {
				continue
			}
			if d.Friendly || d.Attacker == 0 || d.Victim == 0 || d.Attacker == d.Victim || d.HealthDamage <= 0 {
				continue
			}
			p.DamageEvents++
			if link >= 0 {
				p.LinkedDamage++
			}
		}
	}
	sequences := m.ShootingSequences()
	for _, s := range sequences {
		if len(p.Match.Party) > 0 && !slices.Contains(p.Match.Party, s.Player) {
			continue
		}
		for i := range m.Rounds {
			if m.Rounds[i].Number == s.Round {
				v := shootingView{Name: m.PlayerName(s.Player), Round: s.Round, Time: m.Time(&m.Rounds[i], s.Tick), ShootingSequence: s}
				for _, sample := range s.Preparation {
					mv := movementView{BeforeMS: int(float64(s.Tick-sample.Tick) * 1000 / m.TickRate), WindowMS: int(float64(sample.WindowTicks) * 1000 / m.TickRate), Airborne: sample.Airborne}
					if sample.Speed != nil {
						mv.Speed, mv.Known = *sample.Speed, true
					}
					v.Movement = append(v.Movement, mv)
				}
				p.Shooting = append(p.Shooting, v)
				break
			}
		}
	}
	for _, pl := range m.Players {
		if len(p.Match.Party) > 0 && !slices.Contains(p.Match.Party, pl.ID) {
			continue
		}
		v := mechanicsView{Name: pl.Name}
		for _, s := range sequences {
			if s.Player != pl.ID {
				continue
			}
			v.Shots += s.Shots
			v.SpeedSamples += s.SpeedSamples
			v.MovingSamples += s.MovingSamples
			v.Sequences++
			switch {
			case s.Shots == 1:
				v.Singles++
			case s.Shots <= 5:
				v.Short++
			default:
				v.Long++
			}
		}
		if v.Shots > 0 {
			p.Mechanics = append(p.Mechanics, v)
		}
	}
	inParty := func(id match.PlayerID) bool { return slices.Contains(p.Match.Party, id) }
	// Before a party is chosen, pre-select the signed-in player.
	preselect := func(id match.PlayerID) bool {
		if len(p.Match.Party) > 0 {
			return inParty(id)
		}
		if len(p.SuggestedParty) > 0 {
			return slices.Contains(p.SuggestedParty, id)
		}
		return p.User != nil && p.User.SteamID == id
	}

	for _, side := range []match.Side{match.SideT, match.SideCT} {
		tv := teamView{Side: "Started as " + side.String()}
		for _, pl := range m.Players {
			if m.StartSide(pl.ID) == side {
				if preselect(pl.ID) {
					p.PartyNames = append(p.PartyNames, pl.Name)
				}
				tv.Players = append(tv.Players, pickView{ID: pl.ID, Name: pl.Name, InParty: preselect(pl.ID)})
			}
		}
		p.Teams = append(p.Teams, tv)
	}

	byID := map[string]findingView{}
	for _, f := range an.Report.Findings {
		fv := findingView{
			ID:     f.ID,
			Round:  f.Round,
			Time:   f.Time,
			Player: m.PlayerName(f.Player),
			Kind:   kindLabels[f.Kind],
			Place:  match.PlaceName(f.Place),
			Detail: f.Detail,
			Side:   f.Side.String(),
		}
		byID[f.ID] = fv
		involved := inParty(f.Player) || slices.ContainsFunc(f.Related, inParty) || (p.User != nil && f.Player == p.User.SteamID)
		if len(p.Match.Party) == 0 || involved {
			p.Findings = append(p.Findings, fv)
		}
	}
	evidence := func(ids []string) []findingView {
		var out []findingView
		for _, id := range ids {
			if fv, ok := byID[id]; ok {
				out = append(out, fv)
			}
		}
		return out
	}

	if an.Advice == nil {
		return
	}
	// New advice carries identity; legacy names are resolved only if unique.
	for _, pa := range an.Advice.Players {
		id := coach.ResolvePlayer(pa, m.Players, p.Match.Party)
		isMember := id != 0
		pv := playerView{Name: pa.Player, Self: p.User != nil && id == p.User.SteamID}
		for _, f := range pa.Focus {
			var places []coach.Place
			if isMember {
				places, _ = coach.TopPlaces(history.CountPlaces(m.Map, an.Report.Findings, id, f.Kind), false)
			}
			pv.Focus = append(pv.Focus, newFocusView(f, evidence(f.Evidence), places))
		}
		p.Players = append(p.Players, pv)
	}
	for _, t := range an.Advice.Team {
		p.Team = append(p.Team, teamAdviceView{Title: t.Title, Why: t.Why, Suggestion: t.Suggestion, Evidence: evidence(t.Evidence)})
	}
	p.Notes = an.Advice.Notes
	if p.User != nil {
		// Put the signed-in player's recommendations before the party's.
		slices.SortStableFunc(p.Players, func(a, b playerView) int {
			if a.Self && !b.Self {
				return -1
			}
			if b.Self && !a.Self {
				return 1
			}
			return 0
		})
	}
}

// whereText lists the places worth showing, e.g. "B site 4".
func whereText(places []coach.Place) []string {
	var out []string
	for _, p := range coach.Shown(places) {
		out = append(out, fmt.Sprintf("%s %d", p.Name, p.Count))
	}
	return out
}
