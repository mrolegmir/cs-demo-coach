package brief

import (
	"encoding/json"
	"slices"
	"testing"
	"time"

	"github.com/mrolegmir/cs-demo-coach/internal/coach"
	"github.com/mrolegmir/cs-demo-coach/internal/detect"
	"github.com/mrolegmir/cs-demo-coach/internal/match"
	"github.com/mrolegmir/cs-demo-coach/internal/store"
)

func TestBriefCutoffAppliesToHistoryAndPractice(t *testing.T) {
	ms := fixtureMatches(t)
	to := week.Add(Period)
	members := []Member{{ID: 1, Matches: ms, ShareNotes: true}}
	baseline, ok := Input(members, week, to)
	if !ok {
		t.Fatal("no baseline")
	}
	future := ms[len(ms)-1]
	future.ID, future.Played = 999, to // the cutoff is exclusive
	future.Names = map[match.PlayerID]string{1: "future name"}
	members[0].Matches = append(slices.Clone(ms), future)
	members[0].Practice = []store.Practice{{Kind: detect.KindIsolatedDeath, On: to, Note: "future practice"}}
	got, ok := Input(members, week, to)
	if !ok {
		t.Fatal("no input")
	}
	wantJSON, _ := json.Marshal(baseline)
	gotJSON, _ := json.Marshal(got)
	if string(gotJSON) != string(wantJSON) {
		t.Fatal("matches or practice at/after the cutoff changed the brief")
	}
	if len(members[0].Matches) != len(ms)+1 || members[0].Matches[len(ms)].ID != 999 || len(members[0].Practice) != 1 {
		t.Fatal("building the brief mutated caller data")
	}
}

func TestBriefEvidenceSurvivesNameChanges(t *testing.T) {
	ms := fixtureMatches(t)
	// A player has a different name in every match. Findings from older
	// matches must still identify the same party member in the coach input.
	for i := range ms {
		ms[i].Names[1] = ms[i].Played.Format(time.RFC3339)
	}
	in, ok := Input([]Member{{ID: 1, Matches: ms}}, week, week.Add(Period))
	if !ok {
		t.Fatal("no input")
	}
	var examples int
	for _, is := range in.Stats[0].Issues {
		for _, ex := range is.Examples {
			examples++
			if e := in.Evidence[ex.ID]; e.Player != in.Party[0] {
				t.Fatalf("finding %s names %q, party member is %q", ex.ID, e.Player, in.Party[0])
			}
		}
	}
	if examples == 0 {
		t.Fatal("test has no evidence")
	}
	a, err := (coach.Rules{PerPlayer: 3}).Advise(t.Context(), in)
	if err != nil {
		t.Fatal(err)
	}
	if err := in.Validate(a); err != nil {
		t.Fatalf("name changes broke grounding: %v", err)
	}
}
