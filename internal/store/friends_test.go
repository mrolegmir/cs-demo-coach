package store

import (
	"slices"
	"testing"

	"github.com/mrolegmir/cs-demo-coach/internal/detect"
	"github.com/mrolegmir/cs-demo-coach/internal/match"
)

func TestSavedFriendsAutomaticPartyAndQuota(t *testing.T) {
	s := testStore(t)
	ctx := t.Context()
	const owner, friend, opponent, random match.PlayerID = 76561198000000001, 76561198000000002, 76561198000000003, 76561198000000004
	if err := s.EnsureAdmin(ctx, owner); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.Exec(ctx, `UPDATE users SET admin=false WHERE steam_id=$1`, int64(owner)); err != nil {
		t.Fatal(err)
	}
	if err := s.SaveFriendSettings(ctx, owner, []match.PlayerID{friend, opponent}, true); err != nil {
		t.Fatal(err)
	}
	m := &match.Match{Map: "de_cache", Players: []match.Player{{ID: owner, Name: "player01"}, {ID: friend, Name: "player02"}, {ID: opponent, Name: "player03"}, {ID: random, Name: "player04"}}, Rounds: []match.Round{{Players: []match.PlayerRound{{Player: owner, Side: match.SideT}, {Player: friend, Side: match.SideT}, {Player: opponent, Side: match.SideCT}, {Player: random, Side: match.SideT}}}}}
	newMatch := func() int64 {
		t.Helper()
		id, err := s.CreateMatch(ctx, Upload{Filename: "friends.dem", By: owner})
		if err != nil {
			t.Fatal(err)
		}
		if err := s.SaveDetection(ctx, id, m, detect.Report{}); err != nil {
			t.Fatal(err)
		}
		return id
	}
	id := newMatch()
	for range 2 {
		if err := s.AutoCoach(ctx, id, 1); err != nil {
			t.Fatal(err)
		}
	}
	mt, err := s.GetMatch(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if mt.Status != StatusCoaching || !slices.Equal(mt.Party, []match.PlayerID{owner, friend}) {
		t.Fatalf("wrong automatic party: %+v", mt)
	}
	u, err := s.GetUsage(ctx, owner, mt.UploadedAt)
	if err != nil || u.Coaching != 1 {
		t.Fatalf("duplicate automatic request: %+v %v", u, err)
	}
	if err := s.SaveDetection(ctx, id, m, detect.Report{}); err != nil {
		t.Fatal(err)
	}
	mt, _ = s.GetMatch(ctx, id)
	if mt.Status != StatusCoaching {
		t.Fatal("replayed detection reset queued coaching")
	}
	second := newMatch()
	if err := s.AutoCoach(ctx, second, 1); err != nil {
		t.Fatal(err)
	}
	mt, _ = s.GetMatch(ctx, second)
	if mt.Status != StatusDetected || len(mt.Party) != 0 {
		t.Fatal("quota did not prevent automatic coaching")
	}
	contacts, on, err := s.FriendSettings(ctx, owner)
	if err != nil || !on {
		t.Fatalf("settings: %+v %v", contacts, err)
	}
	for _, c := range contacts {
		if c.ID == random && c.Selected {
			t.Fatal("random teammate became a friend")
		}
	}
	// A manual selection wins; even an unlimited automatic call leaves it alone.
	if err := s.RequestCoaching(ctx, second, []match.PlayerID{owner}, owner, "en"); err != nil {
		t.Fatal(err)
	}
	if err := s.AutoCoach(ctx, second, 0); err != nil {
		t.Fatal(err)
	}
	mt, _ = s.GetMatch(ctx, second)
	if !slices.Equal(mt.Party, []match.PlayerID{owner}) {
		t.Fatal("manual party overwritten")
	}
	if err := s.SaveFriendSettings(ctx, owner, nil, false); err != nil {
		t.Fatal(err)
	}
	third := newMatch()
	if err := s.AutoCoach(ctx, third, 0); err != nil {
		t.Fatal(err)
	}
	mt, _ = s.GetMatch(ctx, third)
	if mt.Status != StatusDetected {
		t.Fatal("disabled automatic coaching still ran")
	}
}

func TestPartyForOnlyIncludesSavedTeammates(t *testing.T) {
	players := []Player{{ID: 1, Side: match.SideT}, {ID: 2, Side: match.SideCT}, {ID: 3, Side: match.SideT}, {ID: 4, Side: match.SideT}}
	if got := PartyFor(1, players, []match.PlayerID{2, 3}); !slices.Equal(got, []match.PlayerID{1, 3}) {
		t.Fatalf("wrong party: %v", got)
	}
	if got := PartyFor(99, players, []match.PlayerID{3}); len(got) != 0 {
		t.Fatalf("party for nonparticipant: %v", got)
	}
}
