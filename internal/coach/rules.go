package coach

import (
	"cmp"
	"context"
	"fmt"
	"slices"
	"unicode"
	"unicode/utf8"

	"github.com/mrolegmir/cs-demo-coach/internal/detect"
	"github.com/mrolegmir/cs-demo-coach/internal/drills"
	"github.com/mrolegmir/cs-demo-coach/internal/history"
)

// Rules is a deterministic coach that needs no LLM. It picks each player's
// issues by how far they exceed the lobby rate and uses fixed wording. It is
// the offline mode and the baseline the LLM coach has to beat.
type Rules struct {
	// PerPlayer is how many focus areas each player gets.
	PerPlayer int
}

// minRounds is the match length below which patterns are flagged as noisy.
const minRounds = 10

func (r Rules) Advise(_ context.Context, in *Input) (*Advice, error) {
	if err := in.ValidateIdentity(); err != nil {
		return nil, fmt.Errorf("%w: %w", ErrUngrounded, err)
	}
	a := &Advice{Players: []PlayerAdvice{}, Team: []TeamAdvice{}}
	if in.Period == "" && in.Rounds < minRounds {
		a.Notes = append(a.Notes, in.text("only %d rounds played; treat these patterns as hints, not conclusions", in.Rounds))
	}

	// players with a stand-out issue, per kind, for the team section.
	byKind := map[detect.Kind][]Issue{}
	var kindOrder []detect.Kind
	for _, ps := range in.Stats {
		pa := PlayerAdvice{Player: ps.Player, Focus: []Focus{}}
		issues := slices.Clone(ps.Issues)
		slices.SortFunc(issues, compareIssues)
		for _, is := range issues {
			if !is.StandsOut {
				continue
			}
			if byKind[is.Kind] == nil {
				kindOrder = append(kindOrder, is.Kind)
			}
			byKind[is.Kind] = append(byKind[is.Kind], is)
			if len(pa.Focus) < min(r.PerPlayer, MaxFocus) {
				pa.Focus = append(pa.Focus, in.focus(is))
			}
		}
		if len(pa.Focus) == 0 {
			period := in.text("this match")
			if in.Period != "" {
				period = in.text("your analyzed history")
			}
			a.Notes = append(a.Notes, in.text("no mistake stands out for %s in %s; more matches may be needed to judge", ps.Player, period))
		}
		a.Players = append(a.Players, pa)
	}

	pair, hasPair := pairAdvice(in)
	if hasPair {
		a.Team = append(a.Team, pair)
	}
	entry, hasEntry := entryAdvice(in)
	if hasEntry {
		a.Team = append(a.Team, entry)
	}
	for _, k := range kindOrder {
		issues := byKind[k]
		if len(issues) < 2 || len(a.Team) >= maxTeam || hasPair && k == detect.KindMissedTrade ||
			hasEntry && k == detect.KindOpeningDeath {
			continue
		}
		info := in.kind(k)
		t := TeamAdvice{
			Title:      in.text("Team: ") + info.Title,
			Why:        in.text("%d of you are above the lobby rate for %s. %s.", len(issues), info.Meaning, capitalize(info.Why)),
			Suggestion: capitalize(info.Drill) + ".",
		}
		for _, is := range issues {
			for _, ex := range is.Examples[:min(2, len(is.Examples))] {
				t.Evidence = append(t.Evidence, ex.ID)
			}
		}
		a.Team = append(a.Team, t)
	}
	return in.finish(a), nil
}

// maxTeam is how many team items the rule-based coach gives, like the LLM.
const maxTeam = 2

// Thresholds for a pair to become team advice: enough shared deaths to
// judge, and a trade rate low enough to matter.
const (
	minPairChances = 4
	maxPairRate    = 0.25
)

// pairAdvice turns the party pair that trades worst into team advice, backed
// by the partner's missed trades of that player.
func pairAdvice(in *Input) (TeamAdvice, bool) {
	var worst *Pair
	for i, p := range in.Pairs {
		if p.Chances >= minPairChances && p.Rate <= maxPairRate && (worst == nil || p.Rate < worst.Rate) {
			worst = &in.Pairs[i]
		}
	}
	if worst == nil {
		return TeamAdvice{}, false
	}
	t := TeamAdvice{
		Title: in.text("Team: %s, trade %s", worst.Partner, worst.Player),
		Why: in.text("When %s dies with %s as the nearest teammate, the killer is traded %d of %d times (%.0f%%).",
			worst.Player, worst.Partner, worst.Traded, worst.Chances, worst.Rate*100),
		Suggestion: in.text("%s, play a few steps behind %s and swing the moment they die; agree who peeks first before you go.",
			worst.Partner, worst.Player),
	}
	var ids []string
	for id, e := range in.Evidence {
		if e.Kind == detect.KindMissedTrade && e.Player == worst.Partner && slices.Contains(e.Related, worst.Player) {
			ids = append(ids, id)
		}
	}
	slices.SortFunc(ids, func(a, b string) int {
		ea, eb := in.Evidence[a], in.Evidence[b]
		return cmp.Or(cmp.Compare(ea.Match, eb.Match), cmp.Compare(ea.Round, eb.Round), cmp.Compare(ea.Time, eb.Time), cmp.Compare(a, b))
	})
	t.Evidence = ids[:min(len(ids), 3)]
	return t, len(t.Evidence) > 0
}

// entryAdvice asks the party's best flasher to flash for an entry
// who keeps dying first without one, citing those deaths.
func entryAdvice(in *Input) (TeamAdvice, bool) {
	for _, entry := range in.Roles {
		if !slices.Contains(entry.Labels, history.Entry) {
			continue
		}
		var ids []string
		for id, e := range in.Evidence {
			if e.Kind == detect.KindOpeningDeath && e.Player == entry.Player {
				ids = append(ids, id)
			}
		}
		if is := in.issue(entry.Player, detect.KindOpeningDeath); is == nil || !is.StandsOut || len(ids) == 0 {
			continue
		}
		var support *Role
		for i, r := range in.Roles {
			// The flasher has to be good at it: at least the lobby's rate.
			if r.Player != entry.Player && r.Flashes10 > 0 && r.Flashes10 >= r.LobbyFlashes10 &&
				(support == nil || r.Flashes10 > support.Flashes10) {
				support = &in.Roles[i]
			}
		}
		if support == nil {
			continue
		}
		slices.SortFunc(ids, func(a, b string) int {
			ea, eb := in.Evidence[a], in.Evidence[b]
			return cmp.Or(cmp.Compare(ea.Match, eb.Match), cmp.Compare(ea.Round, eb.Round), cmp.Compare(a, b))
		})
		plan := in.text("%s goes first, %s flashes the angle right before; say it in freeze time so it is the plan, not a favor.", entry.Player, support.Player)
		for _, player := range in.Party {
			if player != entry.Player && player != support.Player {
				plan = in.text("Practice a flash-entry-trade together: %s calls the peek and goes first, %s flashes the angle on the call, and %s follows close enough to trade. Rehearse the timing in practice mode before using it in a match.", entry.Player, support.Player, player)
				break
			}
		}
		return TeamAdvice{
			Title: in.text("Team: %s, flash for %s", support.Player, entry.Player),
			Why: in.text("%s takes the first fight in %.0f%% of rounds (lobby %.0f%%) and often dies there without a flash. %s's flashes blind enemies most often in the party: %.1f per 10 rounds (lobby %.1f).",
				entry.Player, entry.OpeningRate*100, entry.LobbyOpeningRate*100, support.Player, support.Flashes10, support.LobbyFlashes10),
			Suggestion: plan,
			Evidence:   ids[:min(len(ids), 3)],
		}, true
	}
	return TeamAdvice{}, false
}

func (in *Input) focus(is Issue) Focus {
	info := in.kind(is.Kind)
	f := Focus{
		Kind:  is.Kind,
		Title: info.Title,
		Why: in.text("%s: %d of %d %s (%.0f%%), lobby average %.0f%%. %s.",
			capitalize(info.Meaning), is.Count, is.Chances, info.Chances, is.Rate*100, is.LobbyRate*100, capitalize(info.Why)),
	}
	d := pickDrill(is)
	f.DrillID = d.ID
	f.Drill = in.text("Your target: the lobby's %.0f%%, down from your %.0f%%.", is.LobbyRate*100, is.Rate*100)
	if p, ok := mainPlace(is); ok {
		of := in.text("your %d", is.Placed)
		if is.Placed < is.Count {
			// Matches from before places were recorded have none.
			of = in.text("the %d with a known place", is.Placed)
		}
		f.Drill += in.text(" Start at %s: %d of %s were there.", p.Name, p.Count, of)
	}
	if p := is.Practice; p != nil {
		switch p.Verdict {
		case "working":
			f.Why += in.text(" Your practice since %s is working: %.0f%% before, %.0f%% since.", p.Since, p.RateBefore*100, p.RateAfter*100)
		case "not_yet":
			f.Why += in.text(" No change yet since you started practicing on %s (%.0f%% before, %.0f%% since); so here is a different drill to try.", p.Since, p.RateBefore*100, p.RateAfter*100)
		case "no_baseline":
			f.Why += in.text(" You have practiced this since %s, before your first analyzed match, so there is nothing earlier to compare with.", p.Since)

		default:
			f.Why += in.text(" You started practicing this on %s; it is too early to tell if it helps.", p.Since)
		}
	}
	for _, ex := range is.Examples {
		f.Evidence = append(f.Evidence, ex.ID)
	}
	return f
}

// mainPlace returns the place where an issue clusters: at least two
// findings and a third of those with a place.
func mainPlace(is Issue) (Place, bool) {
	if len(is.Places) == 0 {
		return Place{}, false
	}
	p := is.Places[0]
	return p, p.Count >= 2 && p.Count*3 >= is.Placed
}

// pickDrill returns the first library drill for the issue, skipping drills
// the player has practiced without effect while another one is left.
func pickDrill(is Issue) drills.Drill {
	if p := is.Practice; p != nil && p.Verdict == "not_yet" {
		if d := untried(is.Kind, p.Drills); d != nil {
			return *d
		}
	}
	return drills.ForKind(is.Kind)[0]
}

func capitalize(s string) string {
	if s == "" {
		return s
	}
	first, size := utf8.DecodeRuneInString(s)
	return string(unicode.ToUpper(first)) + s[size:]
}
