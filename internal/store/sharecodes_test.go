package store

import (
	"errors"
	"testing"
	"time"
)

func TestSteamLinks(t *testing.T) {
	s := testStore(t)
	ctx := t.Context()
	const id = 76561197960265729
	if err := s.EnsureAdmin(ctx, id); err != nil {
		t.Fatal(err)
	}
	if _, err := s.GetSteamLink(ctx, id); !errors.Is(err, ErrNotFound) {
		t.Fatalf("no link yet: err = %v", err)
	}
	if err := s.SetSteamLink(ctx, id, "AAAA-BBBBB-CCCC", "CSGO-a"); err != nil {
		t.Fatal(err)
	}
	due, err := s.SteamLinksToCheck(ctx, time.Now())
	if err != nil || len(due) != 1 || due[0].LastCode != "CSGO-a" || !due[0].Checked.IsZero() {
		t.Fatalf("due = %+v, %v", due, err)
	}
	link := due[0]

	if stored, err := s.RecordShareCodes(ctx, link, []string{"CSGO-b", "CSGO-c"}, ""); err != nil || !stored {
		t.Fatalf("record = %v, %v", stored, err)
	}
	got, err := s.GetSteamLink(ctx, id)
	if err != nil || got.LastCode != "CSGO-c" || got.Checked.IsZero() {
		t.Fatalf("after check: %+v, %v", got, err)
	}
	if due, _ := s.SteamLinksToCheck(ctx, time.Now().Add(-time.Minute)); len(due) != 0 {
		t.Errorf("a link checked just now is due again: %+v", due)
	}
	codes, err := s.ListShareCodes(ctx, id, 10)
	// The starting code is a match too: the importer only finds later ones.
	if err != nil || len(codes) != 3 || codes[0].Code != "CSGO-c" || codes[2].Code != "CSGO-a" || codes[0].Status != "waiting" {
		t.Fatalf("codes = %+v, %v", codes, err)
	}

	// A check that started before the user changed the link is dropped.
	if err := s.SetSteamLink(ctx, id, "AAAA-BBBBB-CCCC", "CSGO-z"); err != nil {
		t.Fatal(err)
	}
	if stored, err := s.RecordShareCodes(ctx, *got, []string{"CSGO-d"}, ""); err != nil || stored {
		t.Fatalf("stale record = %v, %v; want dropped", stored, err)
	}
	if codes, _ := s.ListShareCodes(ctx, id, 10); len(codes) != 4 || codes[0].Code != "CSGO-z" {
		t.Errorf("stale check stored codes: %+v", codes)
	}

	// Steam rejecting the link disables it until it is saved again.
	got, _ = s.GetSteamLink(ctx, id)
	if _, err := s.RecordShareCodes(ctx, *got, nil, "rejected"); err != nil {
		t.Fatal(err)
	}
	if due, _ := s.SteamLinksToCheck(ctx, time.Now().Add(time.Hour)); len(due) != 0 {
		t.Errorf("a rejected link is due: %+v", due)
	}
	if got, _ := s.GetSteamLink(ctx, id); got.Error != "rejected" || got.LastCode != "CSGO-z" {
		t.Errorf("rejected link = %+v", got)
	}
	if err := s.SetSteamLink(ctx, id, "DDDD-EEEEE-FFFF", "CSGO-z"); err != nil {
		t.Fatal(err)
	}
	if due, _ := s.SteamLinksToCheck(ctx, time.Now()); len(due) != 1 || due[0].Error != "" {
		t.Errorf("saving again did not re-enable the link: %+v", due)
	}

	if err := s.DeleteSteamLink(ctx, id); err != nil {
		t.Fatal(err)
	}
	if _, err := s.GetSteamLink(ctx, id); !errors.Is(err, ErrNotFound) {
		t.Errorf("deleted link: err = %v", err)
	}
	if codes, _ := s.ListShareCodes(ctx, id, 10); len(codes) != 4 {
		t.Errorf("deleting the link dropped found codes: %+v", codes)
	}
}

func TestShareCodeImport(t *testing.T) {
	s := testStore(t)
	ctx := t.Context()
	const id = 76561197960265729
	if err := s.EnsureAdmin(ctx, id); err != nil {
		t.Fatal(err)
	}
	if err := s.SetSteamLink(ctx, id, "AAAA-BBBBB-CCCC", "CSGO-a"); err != nil {
		t.Fatal(err)
	}

	j, err := s.ClaimShareCode(ctx, CodeWaiting, CodeResolving, time.Minute)
	if err != nil || j.Code != "CSGO-a" || j.SteamID != id {
		t.Fatalf("claim = %+v, %v", j, err)
	}
	if _, err := s.ClaimShareCode(ctx, CodeWaiting, CodeResolving, time.Minute); !errors.Is(err, ErrNotFound) {
		t.Fatalf("a claimed code was claimed twice: %v", err)
	}
	// A claim older than the retry delay is taken over (the bot died).
	again, err := s.ClaimShareCode(ctx, CodeWaiting, CodeResolving, 0)
	if err != nil || again.ID != j.ID {
		t.Fatalf("stale claim = %+v, %v", again, err)
	}

	played := time.Date(2026, 10, 28, 20, 0, 0, 0, time.UTC)
	if err := s.ResolveShareCode(ctx, j.ID, "http://replay1.valve.net/730/1.dem.bz2", played); err != nil {
		t.Fatal(err)
	}
	if err := s.ResolveShareCode(ctx, j.ID, "x", played); !errors.Is(err, ErrNotFound) {
		t.Errorf("resolving a code twice: err = %v", err)
	}
	d, err := s.ClaimShareCode(ctx, CodeDownload, CodeDownloading, time.Minute)
	if err != nil || d.DemoURL != "http://replay1.valve.net/730/1.dem.bz2" || !d.Played.Equal(played) {
		t.Fatalf("download claim = %+v, %v", d, err)
	}

	// A released claim is due at once and keeps its attempts.
	if err := s.ReleaseShareCode(ctx, d.ID, CodeDownloading, CodeDownload); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ClaimShareCode(ctx, CodeDownload, CodeDownloading, time.Hour); err != nil {
		t.Fatalf("claim after release: %v", err)
	}
	// Reports for a code in another state are refused.
	if err := s.RetryShareCode(ctx, d.ID, CodeResolving, CodeWaiting, "late"); !errors.Is(err, ErrNotFound) {
		t.Errorf("late bot report: err = %v, want ErrNotFound", err)
	}

	mid, err := s.CreateMatch(ctx, Upload{Filename: "x.dem", By: id})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.FinishShareCode(ctx, d.ID, CodeDownloading, CodeImported, mid, ""); err != nil {
		t.Fatal(err)
	}
	if codes, _ := s.ListShareCodes(ctx, id, 10); codes[0].MatchID != mid || codes[0].Status != CodeImported {
		t.Errorf("imported code = %+v", codes[0])
	}
	if err := s.FinishShareCode(ctx, d.ID, CodeResolving, CodeExpired, 0, ""); !errors.Is(err, ErrNotFound) {
		t.Errorf("expiring an imported code: err = %v, want ErrNotFound", err)
	}
}

func TestShareCodeGivesUp(t *testing.T) {
	s := testStore(t)
	ctx := t.Context()
	const id = 76561197960265729
	if err := s.EnsureAdmin(ctx, id); err != nil {
		t.Fatal(err)
	}
	if err := s.SetSteamLink(ctx, id, "AAAA-BBBBB-CCCC", "CSGO-a"); err != nil {
		t.Fatal(err)
	}
	for range MaxCodeAttempts {
		j, err := s.ClaimShareCode(ctx, CodeWaiting, CodeResolving, 0)
		if err != nil {
			t.Fatal(err)
		}
		if err := s.RetryShareCode(ctx, j.ID, CodeResolving, CodeWaiting, "no answer"); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := s.ClaimShareCode(ctx, CodeWaiting, CodeResolving, 0); !errors.Is(err, ErrNotFound) {
		t.Fatalf("claim after the last attempt: err = %v, want ErrNotFound", err)
	}
	codes, _ := s.ListShareCodes(ctx, id, 10)
	if len(codes) != 1 || codes[0].Status != CodeFailed || codes[0].Reason != "Gave up after several attempts: no answer" {
		t.Fatalf("codes = %+v, want one failed code that keeps its last reason", codes)
	}
}
