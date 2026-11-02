package coach

import (
	"encoding/json"
	"slices"
	"strings"
	"testing"

	"github.com/mrolegmir/cs-demo-coach/internal/detect"
)

func TestGroundingContract(t *testing.T) {
	in := fixtureInput(t)
	player, is := standout(t, in)
	good := Focus{Kind: is.Kind, Title: "t", Why: "w", DrillID: drillFor(is.Kind), Drill: "d", Evidence: []string{is.Examples[0].ID}}
	tests := []struct {
		name string
		a    *Advice
		want string
	}{
		{"nil", nil, "advice is required"},
		{"duplicate focus", &Advice{Players: []PlayerAdvice{{Player: player, Focus: []Focus{good, good}}}}, "duplicate kind"},
		{"too much team advice", &Advice{Team: make([]TeamAdvice, 3)}, "team items, max 2"},
	}
	blank := good
	blank.Why = " \n\t"
	tests = append(tests, struct {
		name string
		a    *Advice
		want string
	}{"blank explanation", &Advice{Players: []PlayerAdvice{{Player: player, Focus: []Focus{blank}}}}, "title, why and drill are required"})
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := in.Validate(tt.a); err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("Validate = %v, want %q", err, tt.want)
			}
		})
	}

	// Being related to someone else's mistake does not prove that you made it.
	in.Evidence["foreign"] = Evidence{Kind: is.Kind, Player: "another player", Related: []string{player}}
	good.Evidence = []string{"foreign"}
	if err := in.Validate(&Advice{Players: []PlayerAdvice{{Player: player, Focus: []Focus{good}}}}); err == nil || !strings.Contains(err.Error(), "for this player") {
		t.Fatalf("related player's finding was accepted: %v", err)
	}
	if _, err := parseAdvice("null", in); err == nil {
		t.Fatal("null model reply accepted")
	}
}

func TestPrioritiesAndHistoryNotes(t *testing.T) {
	in := &Input{
		Language: "en",
		Party:    []string{"anna"}, Period: "Oct 23 – Oct 30", Matches: 4,
		Stats: []PlayerStats{{Player: "anna", Issues: []Issue{
			{Kind: detect.KindTeamFlash, Status: "improving", Count: 20, Chances: 50, LobbyRate: .1, StandsOut: true, Examples: []Example{{ID: "f1"}}},
			{Kind: detect.KindIsolatedDeath, Status: "active", Count: 8, Chances: 20, LobbyRate: .1, StandsOut: true, Examples: []Example{{ID: "f2"}}},
			{Kind: detect.KindMissedTrade, Status: "active", Count: 10, Chances: 20, LobbyRate: .1, StandsOut: true, Examples: []Example{{ID: "f3"}}},
			{Kind: detect.KindUnusedUtility, Status: "resolved", Count: 3, Chances: 50},
		}}},
		Evidence: map[string]Evidence{
			"f1": {Player: "anna", Kind: detect.KindTeamFlash},
			"f2": {Player: "anna", Kind: detect.KindIsolatedDeath},
			"f3": {Player: "anna", Kind: detect.KindMissedTrade},
		},
	}
	a, err := (Rules{PerPlayer: 3}).Advise(t.Context(), in)
	if err != nil {
		t.Fatal(err)
	}
	if err := in.Validate(a); err != nil {
		t.Fatal(err)
	}
	focus := a.Players[0].Focus
	if focus[0].Kind != detect.KindMissedTrade || focus[1].Kind != detect.KindIsolatedDeath || focus[2].Kind != detect.KindTeamFlash {
		t.Fatalf("focus order = %+v", focus)
	}
	if !strings.Contains(focus[0].Priority, "Priority 1 of 3: about 8.0 excess") {
		t.Fatalf("priority = %q", focus[0].Priority)
	}
	if len(a.Notes) != 1 || !strings.Contains(a.Notes[0], "no longer stands out") {
		t.Fatalf("history notes = %v", a.Notes)
	}
	if in.Stats[0].Issues[0].Kind != detect.KindTeamFlash {
		t.Fatal("coach mutated its input")
	}

	// The model's ordering and priority text cannot override computed ranks.
	slices.Reverse(focus)
	for i := range focus {
		focus[i].Priority = "invented ranking"
	}
	raw, err := json.Marshal(a)
	if err != nil {
		t.Fatal(err)
	}
	model, err := (LLM{Provider: &scripted{replies: []string{string(raw)}}}).Advise(t.Context(), in)
	if err != nil || !strings.Contains(model.Players[0].Focus[0].Priority, "Priority 1 of 3: about 8.0 excess") {
		t.Fatalf("model ranking = %+v, %v", model, err)
	}
}

func TestLLMSkipsCallsWithoutFocus(t *testing.T) {
	in := &Input{Language: "en", Party: []string{"anna"}, Period: "Oct 23 – Oct 30", Matches: 1, Stats: []PlayerStats{{Player: "anna"}}}
	p := &scripted{}
	a, err := (LLM{Provider: p}).Advise(t.Context(), in)
	if err != nil || len(p.prompts) != 0 || len(a.Players) != 1 || len(a.Players[0].Focus) != 0 {
		t.Fatalf("empty focus: advice %+v, err %v, calls %d", a, err, len(p.prompts))
	}
	if len(a.Notes) != 1 || strings.Contains(a.Notes[0], "this match") || strings.Contains(a.Notes[0], "0 rounds") {
		t.Fatalf("brief caveats = %v", a.Notes)
	}
	// Pair advice still works without a stand-out individual issue.
	in.Party = append(in.Party, "bob")
	in.Pairs = []Pair{{Player: "anna", Partner: "bob", Chances: 5, Traded: 1, Rate: .2}}
	in.Evidence = map[string]Evidence{"f1": {Player: "bob", Related: []string{"anna"}, Kind: detect.KindMissedTrade}}
	a, err = (LLM{Provider: p}).Advise(t.Context(), in)
	if err != nil || len(a.Team) != 1 || len(p.prompts) != 0 {
		t.Fatalf("pair advice = %+v, err %v, calls %d", a, err, len(p.prompts))
	}
	if err := in.Validate(a); err != nil {
		t.Fatal(err)
	}
}
