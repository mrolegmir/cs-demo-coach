package coach

import (
	"cmp"
	"slices"

	"github.com/mrolegmir/cs-demo-coach/internal/history"
)

// compareIssues puts recurring habits before improving ones, then orders
// by excess occurrences against the lobby. Kind breaks ties deterministically.
func compareIssues(a, b Issue) int {
	stage := func(is Issue) int {
		if is.Status == string(history.Improving) {
			return 1
		}
		return 0
	}
	return cmp.Or(
		cmp.Compare(stage(a), stage(b)),
		cmp.Compare(b.excess(), a.excess()),
		cmp.Compare(a.Kind, b.Kind),
	)
}

// finish supplies the same ranking explanation and history acknowledgements
// for both coaches. The model cannot invent or override these numbers.
func (in *Input) finish(a *Advice) *Advice {
	a.Language = Language(in.Language)
	in.Identify(a)
	for i := range a.Players {
		pa := &a.Players[i]
		slices.SortFunc(pa.Focus, func(a, b Focus) int {
			return compareIssues(*in.issue(pa.Player, a.Kind), *in.issue(pa.Player, b.Kind))
		})
		for j := range pa.Focus {
			f := &pa.Focus[j]
			is := in.issue(pa.Player, f.Kind)
			f.Priority = in.text("Priority %d of %d: about %.1f excess occurrences versus the lobby over %d chances. Recurring habits come first, then the largest excess; this is not an estimate of rounds lost.",
				j+1, len(pa.Focus), max(0, is.excess()), is.Chances)
		}
	}
	for _, ps := range in.Stats {
		for _, is := range ps.Issues {
			if is.Status == string(history.Resolved) {
				a.Notes = append(a.Notes, in.text("%s: %s no longer stands out in recent matches; it stays out of your focus list.", ps.Player, in.kind(is.Kind).Meaning))
				break // one acknowledgement per player keeps the brief short
			}
		}
	}
	return a
}
