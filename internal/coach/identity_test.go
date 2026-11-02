package coach

import (
	"errors"
	"testing"

	"github.com/mrolegmir/cs-demo-coach/internal/detect"
	"github.com/mrolegmir/cs-demo-coach/internal/match"
)

func TestLegacyAmbiguousInputFailsBeforeCoaching(t *testing.T) {
	in := &Input{Party: []string{"same", "same"}}
	if _, err := (Rules{PerPlayer: 2}).Advise(t.Context(), in); !errors.Is(err, ErrUngrounded) {
		t.Fatalf("ambiguous legacy input: %v", err)
	}
}

func TestDuplicateNamesKeepDistinctEvidenceAndAdvice(t *testing.T) {
	m := &match.Match{Players: []match.Player{{ID: 1, Name: "same"}, {ID: 2, Name: "same"}, {ID: 3, Name: "same [player 1]"}}}
	rep := detect.Report{Stats: []detect.Stat{{Player: 1, Kind: detect.KindNoUtility, Count: 3, Chances: 10, Findings: []string{"mine"}}, {Player: 2, Kind: detect.KindNoUtility, Count: 3, Chances: 10, Findings: []string{"other"}}, {Player: 3, Kind: detect.KindNoUtility, Chances: 100}}, Findings: []detect.Finding{{ID: "mine", Player: 1, Kind: detect.KindNoUtility}, {ID: "other", Player: 2, Kind: detect.KindNoUtility}}}
	in := NewInput(m, rep, []match.PlayerID{1, 2})
	if in.Party[0] == in.Party[1] || in.Party[0] == m.Players[2].Name {
		t.Fatal("labels collide")
	}
	if m.Players[0].Name != "same" {
		t.Fatal("input mutated the match")
	}
	a, err := (Rules{PerPlayer: 2}).Advise(t.Context(), in)
	if err != nil {
		t.Fatal(err)
	}
	if err := in.Validate(a); err != nil {
		t.Fatal(err)
	}
	if a.Players[0].PlayerID != 1 || a.Players[1].PlayerID != 2 {
		t.Fatal("lost stable identity")
	}
	a.Players[0].Focus[0].Evidence = []string{"other"}
	if err := in.Validate(a); err == nil {
		t.Fatal("accepted the other same-named player's evidence")
	}
	a.Players[0].Focus[0].Evidence = []string{"mine"}
	a.Players[0].PlayerID = 2
	if err := in.Validate(a); err == nil {
		t.Fatal("accepted mismatched ID")
	}
}

func TestLegacyAdviceNeverGuessesDuplicateIdentity(t *testing.T) {
	players := []match.Player{{ID: 1, Name: "same"}, {ID: 2, Name: "same"}}
	party := []match.PlayerID{1}
	if id := ResolvePlayer(PlayerAdvice{Player: "same"}, players, party); id != 0 {
		t.Fatalf("guessed %d", id)
	}
	if id := ResolvePlayer(PlayerAdvice{Player: "old nickname", PlayerID: 1}, players, party); id != 1 {
		t.Fatalf("stable ID lost: %d", id)
	}
	if id := ResolvePlayer(PlayerAdvice{PlayerID: 2}, players, party); id != 0 {
		t.Fatal("opponent became party member")
	}
}
