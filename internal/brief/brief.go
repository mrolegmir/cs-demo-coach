// Package brief builds the weekly brief: a party's habits across all their
// analyzed matches, with fresh examples, as input for the coach.
package brief

import (
	"cmp"
	"fmt"
	"math"
	"slices"
	"time"

	"github.com/mrolegmir/cs-demo-coach/internal/coach"
	"github.com/mrolegmir/cs-demo-coach/internal/detect"
	"github.com/mrolegmir/cs-demo-coach/internal/history"
	"github.com/mrolegmir/cs-demo-coach/internal/match"
	"github.com/mrolegmir/cs-demo-coach/internal/store"
)

// MaxParty is the largest party a brief covers.
const MaxParty = 5

// maxExamples caps examples per issue, as for a single match.
const maxExamples = 6

// Party picks saved friends who shared the owner's team during the period,
// most frequent first. A friend need only have played once. Opponents and
// unselected teammates never count.
func Party(owner match.PlayerID, ownerMatches []store.PlayerMatch, from, to time.Time, friends []match.PlayerID) []match.PlayerID {
	together := map[match.PlayerID]int{}
	for _, pm := range ownerMatches {
		if !within(pm.Played, from, to) {
			continue
		}
		for p, side := range pm.Sides {
			if p != owner && side != match.SideNone && side == pm.Sides[owner] && slices.Contains(friends, p) && (!pm.ManualParty || slices.Contains(pm.Party, p)) {
				together[p]++
			}
		}
	}
	var mates []match.PlayerID
	for p, n := range together {
		if n >= 1 {
			mates = append(mates, p)
		}
	}
	slices.SortFunc(mates, func(a, b match.PlayerID) int {
		return cmp.Or(cmp.Compare(together[b], together[a]), cmp.Compare(a, b))
	})
	return append([]match.PlayerID{owner}, mates[:min(len(mates), MaxParty-1)]...)
}

func within(t, from, to time.Time) bool { return !t.Before(from) && t.Before(to) }

// Member is a party member with all their analyzed matches.
type Member struct {
	ID      match.PlayerID
	Matches []store.PlayerMatch
	// Practice is the member's practice log, oldest first.
	Practice []store.Practice
	// ShareNotes passes the text of practice notes to the coach. Only the
	// brief owner's notes are shared; teammates' practice counts as numbers.
	ShareNotes bool
}

// Input builds coach input for a party over a period. Each member's issues
// come from their whole history, so a habit counts even if this week was
// quiet; examples are the most recent findings. ok is false when nobody in
// the party played in the period.
func Input(members []Member, from, to time.Time) (in *coach.Input, ok bool) {
	// A scheduled brief may be built after its cutoff. Keep later matches
	// and practice out of its history as well as out of its weekly count.
	members = slices.Clone(members)
	names := map[match.PlayerID]string{}
	for i := range members {
		m := &members[i]
		m.Matches = slices.DeleteFunc(slices.Clone(m.Matches), func(pm store.PlayerMatch) bool { return !pm.Played.Before(to) })
		m.Practice = slices.DeleteFunc(slices.Clone(m.Practice), func(p store.Practice) bool { return !p.On.Before(to) })
		names[m.ID] = latestName(*m)
	}
	// Include non-party participants when disambiguating labels.
	roster := map[match.PlayerID]string{}
	for _, m := range members {
		for _, pm := range m.Matches {
			for id, name := range pm.Names {
				roster[id] = name
			}
		}
	}
	for id, name := range names {
		roster[id] = name
	}
	names = coach.PlayerLabels(roster)
	in = &coach.Input{
		PlayerIDs: map[string]match.PlayerID{},
		Period:    fmt.Sprintf("%s – %s", from.Format("02.01.2006"), to.Add(-time.Second).Format("02.01.2006")),
		Evidence:  map[string]coach.Evidence{},
	}
	inPeriod := map[int64]bool{}
	for _, m := range members {
		for _, pm := range m.Matches {
			if within(pm.Played, from, to) {
				inPeriod[pm.ID] = true
			}
		}
	}
	in.Matches = len(inPeriod)
	if in.Matches == 0 {
		return nil, false
	}

	in.Pairs = partyPairs(members, from, to, in.Evidence, names)
	for _, m := range members {
		name := names[m.ID]
		in.Party = append(in.Party, name)
		in.PlayerIDs[name] = m.ID
		ps := coach.PlayerStats{Player: name, Issues: []coach.Issue{}}

		var hist []history.Match
		for _, pm := range m.Matches {
			hist = append(hist, history.FromReport(pm.ID, pm.Played, pm.Map, m.ID, pm.Report))
		}
		if rs := history.Roles(hist); rs.Player.Rounds > 0 {
			in.Roles = append(in.Roles, coach.NewRole(name, rs))
		}
		for _, is := range history.Default().Build(hist) {
			if is.Total.Count == 0 || is.Status == history.NotEnoughData || is.Status == history.Fine {
				continue
			}
			ci := coach.Issue{
				Kind:      is.Kind,
				Count:     is.Total.Count,
				Chances:   is.Total.Chances,
				Rate:      round2(is.Total.Rate()),
				LobbyRate: round2(is.Lobby.Rate()),
				StandsOut: is.Status == history.Active || is.Status == history.Improving,
				Status:    string(is.Status),
				Examples:  []coach.Example{},
			}
			ci.Practice = practice(m, is)
			ci.Places, ci.Placed = coach.TopPlaces(is.Places, true)
			for _, ex := range recentFindings(m, is.Kind, names) {
				in.Evidence[ex.id] = ex.ev
				ce := coach.Example{ID: ex.id, Match: ex.match, Round: ex.ev.Round, Place: ex.ev.Place, Detail: ex.ev.Detail}
				if ex.ev.Time > 0 {
					ce.Time = match.Clock(ex.ev.Time)
				}
				ci.Examples = append(ci.Examples, ce)
			}
			ps.Issues = append(ps.Issues, ci)
		}
		in.Stats = append(in.Stats, ps)
	}
	return in, true
}

type found struct {
	id    string
	match string // e.g. "de_dust2, Oct 24"
	ev    coach.Evidence
}

// maxNotes is how many of the latest practice notes per issue reach the
// coach.
const maxNotes = 3

// practice summarizes a member's practice on an issue, or nil if they logged
// none.
func practice(m Member, is history.Issue) *coach.Practice {
	p, ok := is.Practiced(store.PracticeDays(m.Practice, is.Kind))
	if !ok {
		return nil
	}
	cp := &coach.Practice{
		Sessions:     p.Sessions,
		Since:        p.Since.Format("02.01.2006"),
		MatchesAfter: p.MatchesAfter,
		RateBefore:   round2(p.Before.Rate()),
		RateAfter:    round2(p.After.Rate()),
		Verdict:      p.Verdict(),
	}
	// Library drill IDs are not free text, so they reach the coach for every
	// member: they are what keeps a drill that is not working from coming
	// back. Notes are the member's own words and only shared by the owner.
	var notes []string
	for _, e := range m.Practice {
		if e.Kind != is.Kind {
			continue
		}
		notes = append(notes, e.Note)
		if e.Drill != "" && !slices.Contains(cp.Drills, e.Drill) {
			cp.Drills = append(cp.Drills, e.Drill)
		}
	}
	if m.ShareNotes {
		cp.Notes = notes[max(0, len(notes)-maxNotes):]
	}
	return cp
}

// recentFindings returns a member's latest findings of a kind, newest first.
// IDs are prefixed with the match, since finding IDs repeat across matches.
func recentFindings(m Member, kind detect.Kind, names map[match.PlayerID]string) []found {
	var out []found
	for _, pm := range slices.Backward(m.Matches) {
		for _, f := range slices.Backward(pm.Report.Findings) {
			if f.Kind != kind || f.Player != m.ID {
				continue
			}
			ev := coach.Evidence{
				PlayerID: m.ID, RelatedIDs: slices.Clone(f.Related), Kind: f.Kind, Player: names[m.ID], Match: pm.ID,
				Round: f.Round, Time: f.Time, Place: match.PlaceName(f.Place), Detail: f.Detail,
			}
			for _, r := range f.Related {
				ev.Related = append(ev.Related, cmp.Or(names[r], pm.Names[r]))
			}
			out = append(out, found{
				id:    fmt.Sprintf("m%d-%s", pm.ID, f.ID),
				match: fmt.Sprintf("%s, %s", pm.Map, pm.Played.Format("02.01.2006")),
				ev:    ev,
			})
			if len(out) == maxExamples {
				return out
			}
		}
	}
	return out
}

func latestName(m Member) string {
	for _, pm := range slices.Backward(m.Matches) {
		if n := pm.Names[m.ID]; n != "" {
			return n
		}
	}
	return fmt.Sprint(uint64(m.ID))
}

func round2(x float64) float64 { return math.Round(x*100) / 100 }

// maxPairEvidence is how many missed trades back one pair.
const maxPairEvidence = 3

// partyPairs sums how party members traded each other in the period, over
// the owner's matches (members[0]), which the party played together. The
// partner's missed trades of the player go into evidence, named like the
// pairs, so team advice about a pair can cite them.
func partyPairs(members []Member, from, to time.Time, evidence map[string]coach.Evidence, names map[match.PlayerID]string) []coach.Pair {
	if len(members) == 0 {
		return nil
	}
	ids := map[match.PlayerID]Member{}
	for _, m := range members {
		ids[m.ID] = m
	}
	sums := map[[2]match.PlayerID]*detect.Pair{}
	cited := map[[2]match.PlayerID]int{}
	for _, pm := range members[0].Matches {
		if !within(pm.Played, from, to) {
			continue
		}
		for _, p := range pm.Report.Pairs {
			_, okPlayer := ids[p.Player]
			_, okPartner := ids[p.Partner]
			if !okPlayer || !okPartner {
				continue
			}
			k := [2]match.PlayerID{p.Player, p.Partner}
			if sums[k] == nil {
				sums[k] = &detect.Pair{Player: p.Player, Partner: p.Partner}
			}
			sums[k].Chances += p.Chances
			sums[k].Traded += p.Traded
		}
		for _, f := range pm.Report.Findings {
			if f.Kind != detect.KindMissedTrade || len(f.Related) == 0 {
				continue
			}
			k := [2]match.PlayerID{f.Related[0], f.Player}
			id := fmt.Sprintf("m%d-%s", pm.ID, f.ID)
			if _, ok := ids[k[0]]; !ok || cited[k] >= maxPairEvidence {
				continue
			}
			if _, ok := ids[k[1]]; !ok {
				continue
			}
			if _, seen := evidence[id]; !seen {
				evidence[id] = coach.Evidence{
					PlayerID: f.Player, RelatedIDs: slices.Clone(f.Related), Kind: f.Kind, Player: names[k[1]], Related: []string{names[k[0]]},
					Match: pm.ID, Round: f.Round, Time: f.Time, Place: match.PlaceName(f.Place), Detail: f.Detail,
				}
			}
			cited[k]++
		}
	}
	var out []coach.Pair
	for _, p := range sums {
		out = append(out, coach.NewPair(names[p.Player], names[p.Partner], p.Chances, p.Traded))
	}
	coach.SortPairs(out)
	return out
}
