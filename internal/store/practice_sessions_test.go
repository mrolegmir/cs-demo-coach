package store

import (
	"errors"
	"testing"

	"github.com/mrolegmir/cs-demo-coach/internal/match"
)

func TestPracticeStartsAreNotCompletedAndFinishIsOwnedAndAtomic(t *testing.T) {
	s := testStore(t)
	ctx := t.Context()
	const owner, other match.PlayerID = 1, 2
	for _, id := range []match.PlayerID{owner, other} {
		if err := s.EnsureAdmin(ctx, id); err != nil {
			t.Fatal(err)
		}
	}
	for range 2 {
		if err := s.StartPractice(ctx, owner, "two-pop-flashes", 20); err != nil {
			t.Fatal(err)
		}
	}
	active, err := s.ActivePractice(ctx, owner)
	if err != nil || len(active) != 1 {
		t.Fatalf("duplicate start: %+v %v", active, err)
	}
	log, err := s.ListPractice(ctx, owner)
	if err != nil || len(log) != 0 {
		t.Fatalf("start counted as completed: %+v %v", log, err)
	}
	if err := s.FinishPractice(ctx, other, active[0].ID, 15); !errors.Is(err, ErrNotFound) {
		t.Fatalf("other user finished session: %v", err)
	}
	if err := s.FinishPractice(ctx, owner, active[0].ID, 15); err != nil {
		t.Fatal(err)
	}
	if err := s.FinishPractice(ctx, owner, active[0].ID, 15); !errors.Is(err, ErrNotFound) {
		t.Fatalf("duplicate completion: %v", err)
	}
	log, err = s.ListPractice(ctx, owner)
	if err != nil || len(log) != 1 || log[0].Minutes != 15 || log[0].Drill != "two-pop-flashes" {
		t.Fatalf("completion: %+v %v", log, err)
	}
	active, err = s.ActivePractice(ctx, owner)
	if err != nil || len(active) != 0 {
		t.Fatalf("completed session still active: %+v %v", active, err)
	}
}
