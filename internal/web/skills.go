package web

import (
	"cmp"
	"slices"
	"strings"

	"github.com/mrolegmir/cs-demo-coach/internal/coach"
	"github.com/mrolegmir/cs-demo-coach/internal/detect"
	"github.com/mrolegmir/cs-demo-coach/internal/drills"
	"github.com/mrolegmir/cs-demo-coach/internal/match"
	"github.com/mrolegmir/cs-demo-coach/internal/store"
)

type skillCheck struct {
	Kind                              detect.Kind
	Title, Observation, Action, Limit string
	Count, Chances, TCount, CTCount   int
	State                             string
	Priority                          int
	PlaceMeaning                      string
	Evidence                          []findingView
	Drills                            []drills.Drill
}

type skillCategory struct {
	Title, Icon, State, Status, Limit string
	Checks                            []skillCheck
	Count                             int
	Priority                          int
	Utility                           *utilityView
	Decisions                         *decisionsView
	FlashSupport                      *flashSupportView
	Shooting, Movement                *mechanicsReviewView
}

// Skill colors describe recorded patterns, never an overall skill grade.
// No red/severity threshold is introduced without calibration.
func (p *matchPage) buildSkills(an *store.Analysis) {
	id := userID(p.User)
	if id == 0 || an.Match.StartSide(id) == match.SideNone {
		return // Never substitute another player's results for the viewer.
	}
	mechanics := mechanicsReviewFor(an.Match, id)
	p.Categories = []skillCategory{
		{Title: "Utility", Icon: "◈", Limit: "Smoke placement and area denial are not assessed. Recorded HE and fire damage is shown separately.", Utility: utilityFor(an.Match, id), Checks: []skillCheck{
			{Kind: detect.KindFlashNoEnemy, Title: "Enemy blinding", Observation: "No enemy was blinded for at least one second.", Action: "Review the flash timing and burst position before choosing a lineup to practice.", Limit: "A short blind does not prove the flash was useless.", PlaceMeaning: "The position is where you threw from, not where the flash burst or where the enemy stood."},
			{Kind: detect.KindTeamFlash, Title: "Blinding your own team", Observation: "Your flash blinded you or a teammate for at least 1.5 seconds.", Action: "Agree on the flash timing with your teammate and check where it bursts.", Limit: "The blind alone does not establish that it caused a lost fight.", PlaceMeaning: "The position is where you threw from, not where the flash burst or where the enemy stood."},
			{Kind: detect.KindUnusedUtility, Title: "Grenades left at death", Observation: "You died carrying grenades other than decoys.", Action: "Review whether there was a useful, safe chance to use a grenade before the fight.", Limit: "Keeping a grenade is not automatically a mistake. Do not throw it just to improve this number."},
			{Kind: detect.KindNoUtility, Title: "Rounds without a grenade throw", Observation: "You threw no grenade in a round with equipment worth at least $3700 and survived at least 30 seconds.", Action: "Before a buy round, choose a useful first grenade and the situation in which you will use it.", Limit: "This check covers the whole round, not utility before your first duel."},
		}},
		{Title: "Teamplay", Icon: "⇄", Limit: "Communication and party membership are not inferred from the demo.", FlashSupport: flashSupportFor(an.Match, id), Checks: []skillCheck{
			{Kind: detect.KindMissedTrade, Title: "Nearby deaths without a trade", Observation: "You were the nearest teammate within the distance threshold, but the killer was not traded within five seconds.", Action: "Review your spacing with a partner and whether you could see the angle.", Limit: "Distance ignores walls; being nearby does not prove you could trade."},
			{Kind: detect.KindOpeningDeath, Title: "Opening deaths without a recent enemy blind", Observation: "You were the first player killed; your team recorded no enemy blind in the preceding two seconds.", Action: "Review whether this opening fight needed a flash or a teammate's support.", Limit: "This does not prove you peeked without support: smokes and other support are not checked."},
		}},
		{Title: "Positioning", Icon: "⌖", Limit: "Only spacing around deaths is checked; cover, exposed angles and rotations are not assessed.", Checks: []skillCheck{
			{Kind: detect.KindIsolatedDeath, Title: "Untraded deaths far from teammates", Observation: "You died without a teammate within the distance threshold and were not traded within five seconds.", Action: "Before taking a fight, check whether a teammate can follow you or whether you should wait.", Limit: "Distance ignores walls and does not establish that the position was wrong."},
		}},
		{Title: "Economy", Icon: "$", Limit: "Equipment and money are checked; team buy calls and planned weapon drops are unknown.", Checks: []skillCheck{
			{Kind: detect.KindBuyDesync, Title: "Buying differently from the team", Observation: "Your equipment purchase differed from the majority buy under the check's money thresholds.", Action: "Agree on a buy or save together before spending.", Limit: "A deliberate team plan may explain this difference."},
			{Kind: detect.KindMissedDrop, Title: "Money available while a teammate lacked a buy", Observation: "You kept at least $3000 after buying while a teammate could not afford a rifle buy.", Action: "Check teammates' money and ask whether someone needs a drop.", Limit: "This is a possible drop to review, not proof that a drop was required."},
		}},
		{Title: "Shooting", Icon: "◎", Limit: "Aim, first-shot accuracy and recoil control do not yet have a validated assessment.", Shooting: mechanics},
		{Title: "Movement", Icon: "↗", Limit: "Counter-strafing, peeking and movement technique do not yet have a validated assessment.", Movement: mechanics},
		{Title: "Decisions", Icon: "◇", Limit: "Recorded situations support review; communication, visibility and the player's information are unknown. No decision quality score is assigned.", Decisions: decisionsFor(an.Match, id)},
	}
	for ci := range p.Categories {
		c := &p.Categories[ci]
		c.State, c.Status = "unknown", "Assessment unavailable"
		hasChances := false
		for si := range c.Checks {
			s := &c.Checks[si]
			s.State = "unknown"
			for _, stat := range an.Report.Stats {
				if stat.Player == id && stat.Kind == s.Kind {
					s.Count, s.Chances = stat.Count, stat.Chances
				}
			}
			for _, f := range an.Report.Findings {
				if f.Player != id || f.Kind != s.Kind {
					continue
				}
				s.Evidence = append(s.Evidence, findingView{ID: f.ID, Round: f.Round, Time: f.Time, Side: f.Side.String(), Place: match.PlaceName(f.Place), PlaceHint: placeHint(an.Match.Map, f.Place)})
				if f.Side == match.SideT {
					s.TCount++
				}
				if f.Side == match.SideCT {
					s.CTCount++
				}
			}
			if s.Chances > 0 {
				hasChances = true
				s.State = "clear"
			}
			if s.Count > 0 {
				s.State = "attention"
				c.Count++
				for _, d := range drills.All() {
					// Do not recommend spending utility merely to empty pockets.
					if d.Kind == s.Kind && d.ID != "utility-before-you-die" {
						s.Drills = append(s.Drills, d)
					}
				}
			}
		}
		if hasChances {
			c.State, c.Status = "clear", "No episodes in the checked situations"
		}
		if c.Count > 0 {
			c.State, c.Status = "attention", "Checks with episodes"
		}
		if len(c.Checks) > 0 && !hasChances {
			c.Status = "Not enough situations to assess"
		}
		if c.Decisions != nil && (len(c.Decisions.Advantages) > 0 || len(c.Decisions.Rounds) > 0) {
			c.Status = "Situations to review"
		}
		if (c.Shooting != nil && len(c.Shooting.Episodes) > 0) || (c.Movement != nil && len(c.Movement.Episodes) > 0) {
			c.Status = "Observations available"
		}
	}
	// Keep the coach's existing validated focus order. Never rank unrelated
	// denominators or invent a new severity score from raw finding counts.
	if an.Advice != nil {
		for _, player := range an.Advice.Players {
			if coach.ResolvePlayer(player, an.Match.Players, p.Match.Party) != id {
				continue
			}
			for _, focus := range player.Focus {
				for ci := range p.Categories {
					c := &p.Categories[ci]
					for si := range c.Checks {
						check := &c.Checks[si]
						if check.Kind == focus.Kind && check.Count > 0 && check.Priority == 0 && len(p.SkillPlan) < 3 {
							check.Priority = len(p.SkillPlan) + 1
							if c.Priority == 0 {
								c.Priority = check.Priority
							}
							p.SkillPlan = append(p.SkillPlan, *check)
						}
					}
				}
			}
		}
	}
}

// References are deliberately allowlisted: demo map names may be arbitrary.
// The guide is a community reference, not the source of demo place labels.
func calloutGuide(mapName string) string {
	name := strings.TrimPrefix(strings.ToLower(mapName), "de_")
	switch name {
	case "cache", "ancient", "anubis", "dust2", "mirage", "inferno", "nuke", "overpass", "vertigo":
		return "https://totalcsgo.com/callouts/" + name
	default:
		return "https://totalcsgo.com/callouts"
	}
}

// These two Cache descriptions paraphrase the linked Total CS guide.
// Never apply a familiar label's meaning to another map.
func placeHint(mapName, place string) string {
	if strings.TrimPrefix(strings.ToLower(mapName), "de_") != "cache" {
		return ""
	}
	switch strings.ToLower(strings.ReplaceAll(place, " ", "")) {
	case "quad":
		return "Around the four-box stack at the back of A site."
	case "forklift":
		return "Around the forklift beneath the A-site balcony."
	default:
		return ""
	}
}

func sortScoreboard(rows []scoreboardView) {
	slices.SortStableFunc(rows, func(a, b scoreboardView) int {
		if a.Group != b.Group {
			if a.Group == "Your team" {
				return -1
			}
			if b.Group == "Your team" {
				return 1
			}
		}
		return cmp.Or(cmp.Compare(a.Group, b.Group), cmp.Compare(b.Kills, a.Kills), cmp.Compare(a.Deaths, b.Deaths), cmp.Compare(a.Name, b.Name))
	})
}
