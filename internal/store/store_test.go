package store

import (
	"errors"
	"fmt"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/mrolegmir/cs-demo-coach/internal/coach"
	"github.com/mrolegmir/cs-demo-coach/internal/detect"
	"github.com/mrolegmir/cs-demo-coach/internal/match"
	"github.com/mrolegmir/cs-demo-coach/internal/store/storetest"
)

// testStore opens a store on a fresh test database.
func testStore(t *testing.T) *Store {
	t.Helper()
	s, err := Open(t.Context(), storetest.URL(t), Limits{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(s.Close)
	return s
}

func fixture(t *testing.T) (*match.Match, detect.Report) {
	t.Helper()
	m, err := match.LoadFile(filepath.Join("..", "..", "testdata", "matches", "Anubis_ShortMatch_2023-08-04.json.gz"))
	if err != nil {
		t.Fatal(err)
	}
	return m, detect.Run(m, detect.Default())
}

func TestMatchLifecycle(t *testing.T) {
	s := testStore(t)
	ctx := t.Context()

	id, err := s.CreateMatch(ctx, Upload{Filename: "anubis.dem"})
	if err != nil {
		t.Fatal(err)
	}
	got, err := s.GetMatch(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != StatusUploaded || got.Filename != "anubis.dem" {
		t.Fatalf("new match = %+v", got)
	}

	job, err := s.ClaimJob(ctx, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if job.Kind != JobDetect || job.MatchID != id || job.Attempts != 1 {
		t.Fatalf("job = %+v", job)
	}

	m, rep := fixture(t)
	if err := s.SaveDetection(ctx, id, m, rep); err != nil {
		t.Fatal(err)
	}
	if err := s.FinishDetection(ctx, job, m, rep); err != nil {
		t.Fatal(err)
	}
	got, err = s.GetMatch(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != StatusDetected || got.Map != m.Map || got.Rounds != len(m.Rounds) || len(got.Players) != len(m.Players) {
		t.Fatalf("detected match = %+v", got)
	}
	var findings int
	if err := s.db.QueryRow(ctx, `SELECT count(*) FROM findings WHERE match_id = $1`, id).Scan(&findings); err != nil {
		t.Fatal(err)
	}
	if findings != len(rep.Findings) {
		t.Errorf("stored %d findings, want %d", findings, len(rep.Findings))
	}

	party := []match.PlayerID{m.Players[0].ID, m.Players[1].ID}
	if err := s.RequestCoaching(ctx, id, party, 0, "ru"); err != nil {
		t.Fatal(err)
	}
	job, err = s.ClaimJob(ctx, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if job.Kind != JobCoach {
		t.Fatalf("job kind = %s, want coach", job.Kind)
	}
	advice := &coach.Advice{Players: []coach.PlayerAdvice{{Player: "x"}}}
	if err := s.SaveAdvice(ctx, id, advice, "rules"); err != nil {
		t.Fatal(err)
	}

	an, err := s.GetAnalysis(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if an.Match.Map != m.Map || len(an.Report.Findings) != len(rep.Findings) || an.Advice == nil || an.Advice.Players[0].Player != "x" {
		t.Fatalf("analysis = %+v", an)
	}
	got, err = s.GetMatch(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != StatusCoached || got.Coach != "rules" || len(got.Party) != 2 || got.Party[0] != party[0] {
		t.Fatalf("coached match = %+v", got)
	}

	list, err := s.ListMatches(ctx, nil, 10)
	if err != nil || len(list) != 1 || list[0].ID != id {
		t.Fatalf("list = %+v, %v", list, err)
	}
	if len(list[0].RoundResults) != len(m.Rounds) || list[0].RoundResults[0].Winner != m.Rounds[0].Winner ||
		list[0].RoundResults[0].Side(m.Players[0].ID) != m.Rounds[0].Side(m.Players[0].ID) ||
		len(list[0].RoundResults[0].Kills) != 0 {
		t.Fatal("match list must retain result data without full event telemetry")
	}
	if _, err := s.GetMatch(ctx, id+1000); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing match error = %v", err)
	}
}

func TestRequestCoachingNeedsDetection(t *testing.T) {
	s := testStore(t)
	id, err := s.CreateMatch(t.Context(), Upload{Filename: "x.dem"})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.RequestCoaching(t.Context(), id, []match.PlayerID{1}, 0, "ru"); err == nil {
		t.Fatal("coaching an undetected match should fail")
	}
}

func TestClaimJob(t *testing.T) {
	s := testStore(t)
	ctx := t.Context()

	if _, err := s.ClaimJob(ctx, time.Minute); !errors.Is(err, ErrNoJob) {
		t.Fatalf("empty queue error = %v", err)
	}

	const n = 20
	for i := range n {
		if _, err := s.CreateMatch(ctx, Upload{Filename: fmt.Sprintf("%d.dem", i)}); err != nil {
			t.Fatal(err)
		}
	}

	// Concurrent workers must never claim the same job.
	var mu sync.Mutex
	claimed := map[int64]bool{}
	var wg sync.WaitGroup
	for range 5 {
		wg.Go(func() {
			for {
				j, err := s.ClaimJob(ctx, time.Minute)
				if errors.Is(err, ErrNoJob) {
					return
				}
				if err != nil {
					t.Error(err)
					return
				}
				mu.Lock()
				if claimed[j.ID] {
					t.Errorf("job %d claimed twice", j.ID)
				}
				claimed[j.ID] = true
				mu.Unlock()
			}
		})
	}
	wg.Wait()
	if len(claimed) != n {
		t.Fatalf("claimed %d jobs, want %d", len(claimed), n)
	}
}

func TestRetryAndStaleJobs(t *testing.T) {
	s := testStore(t)
	ctx := t.Context()
	if _, err := s.CreateMatch(ctx, Upload{Filename: "x.dem"}); err != nil {
		t.Fatal(err)
	}

	j, err := s.ClaimJob(ctx, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.FailAttempt(ctx, j, true, time.Hour, "boom"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ClaimJob(ctx, time.Minute); !errors.Is(err, ErrNoJob) {
		t.Fatalf("delayed job was claimable: %v", err)
	}
	if err := func() error {
		_, err := s.db.Exec(ctx, `UPDATE jobs SET run_after=now() WHERE id=$1`, j.ID)
		return err
	}(); err != nil {
		t.Fatal(err)
	}
	j, err = s.ClaimJob(ctx, time.Minute)
	if err != nil || j.Attempts != 2 {
		t.Fatalf("retried job = %+v, %v", j, err)
	}

	// A worker that died leaves the job running; it is reclaimed once stale.
	if _, err := s.ClaimJob(ctx, time.Hour); !errors.Is(err, ErrNoJob) {
		t.Fatalf("running job was claimable before going stale: %v", err)
	}
	time.Sleep(10 * time.Millisecond)
	j, err = s.ClaimJob(ctx, time.Millisecond)
	if err != nil || j.Attempts != 3 {
		t.Fatalf("stale job = %+v, %v", j, err)
	}

	if err := s.FailAttempt(ctx, j, false, 0, "gave up"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ClaimJob(ctx, time.Millisecond); !errors.Is(err, ErrNoJob) {
		t.Fatalf("failed job was claimable: %v", err)
	}
}

func TestPlayerHistoryQueries(t *testing.T) {
	s := testStore(t)
	ctx := t.Context()
	m, rep := fixture(t)
	for range 2 {
		id, err := s.CreateMatch(ctx, Upload{Filename: "x.dem"})
		if err != nil {
			t.Fatal(err)
		}
		if err := s.SaveDetection(ctx, id, m, rep); err != nil {
			t.Fatal(err)
		}
	}
	p, q := m.Players[0].ID, m.Players[1].ID
	pms, err := s.PlayerMatches(ctx, p)
	if err != nil || len(pms) != 2 || len(pms[0].Report.Stats) != len(rep.Stats) || pms[0].ID > pms[1].ID {
		t.Fatalf("player matches = %d, %v", len(pms), err)
	}
	if name, err := s.PlayerName(ctx, p); err != nil || name != m.Players[0].Name {
		t.Fatalf("name = %q, %v", name, err)
	}

	stranger := &User{SteamID: 99}
	for _, tt := range []struct {
		viewer *User
		want   bool
	}{
		{&User{SteamID: p}, true},
		{&User{SteamID: q}, true}, // played together
		{stranger, false},
		{&User{SteamID: 99, Admin: true}, true},
	} {
		if got, err := s.CanViewPlayer(ctx, tt.viewer, p); err != nil || got != tt.want {
			t.Errorf("CanViewPlayer(%d) = %v, %v; want %v", tt.viewer.SteamID, got, err, tt.want)
		}
	}
}

func TestDuplicateUpload(t *testing.T) {
	s := testStore(t)
	ctx := t.Context()
	sum := []byte("same file")
	played := time.Date(2026, 10, 20, 21, 0, 0, 0, time.UTC)

	first, err := s.CreateMatch(ctx, Upload{Filename: "a.dem", SHA256: sum, Played: played})
	if err != nil {
		t.Fatal(err)
	}
	again, err := s.CreateMatch(ctx, Upload{Filename: "a.dem", SHA256: sum})
	if !errors.Is(err, ErrDuplicate) || again != first {
		t.Fatalf("second upload = %d, %v; want match %d and ErrDuplicate", again, err, first)
	}
	if _, err := s.CreateMatch(ctx, Upload{Filename: "b.dem", SHA256: []byte("other file")}); err != nil {
		t.Fatalf("a different file: %v", err)
	}

	// A failed match does not block analyzing the same demo again.
	if err := s.FailMatch(ctx, first, "parse error"); err != nil {
		t.Fatal(err)
	}
	retry, err := s.CreateMatch(ctx, Upload{Filename: "a.dem", SHA256: sum})
	if err != nil || retry == first {
		t.Fatalf("upload after failure = %d, %v; want a new match", retry, err)
	}
}

func TestBackfill(t *testing.T) {
	s := testStore(t)
	ctx := t.Context()
	m, rep := fixture(t)
	old := rep
	old.Pairs, old.Roles = nil, nil // a report from before they were counted
	var ids []int64
	for range 2 {
		id, err := s.CreateMatch(ctx, Upload{Filename: "anubis.dem"})
		if err != nil {
			t.Fatal(err)
		}
		ids = append(ids, id)
	}
	// SaveDetection fills in pairs an old worker leaves out, so the old
	// report is written directly, as before v0.7.
	if err := s.SaveDetection(ctx, ids[0], m, rep); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.Exec(ctx, `UPDATE matches SET report = report - 'roles' WHERE id = $1`, ids[0]); err != nil {
		t.Fatal(err)
	}
	if err := s.SaveDetection(ctx, ids[1], m, old); err != nil {
		t.Fatal(err)
	}
	if an, err := s.GetAnalysis(ctx, ids[1]); err != nil || len(an.Report.Pairs) != len(rep.Pairs) || len(an.Report.Roles) != len(rep.Roles) {
		t.Fatalf("a report saved without pairs and roles did not get them: %v", err)
	}
	due, err := s.MatchesToBackfill(ctx)
	if err != nil || len(due) != 1 || due[0] != ids[0] {
		t.Fatalf("due = %v, %v; want only the old report", due, err)
	}
	// nil slices marshal as null: the report stays due.
	if err := s.SetReportDerived(ctx, ids[0], nil, nil); err != nil {
		t.Fatal(err)
	}
	if due, _ := s.MatchesToBackfill(ctx); len(due) != 1 {
		t.Fatal("a null backfill marked the report done")
	}
	pairs, roles := detect.Derived(m)
	if err := s.SetReportDerived(ctx, ids[0], pairs, roles); err != nil {
		t.Fatal(err)
	}
	if due, _ := s.MatchesToBackfill(ctx); len(due) != 0 {
		t.Errorf("still due after the backfill: %v", due)
	}
	an, err := s.GetAnalysis(ctx, ids[0])
	if err != nil || len(an.Report.Pairs) != len(rep.Pairs) || len(an.Report.Roles) != len(rep.Roles) ||
		len(an.Report.Findings) != len(rep.Findings) {
		t.Fatalf("backfilled report: %d pairs, %d roles, %d findings, %v",
			len(an.Report.Pairs), len(an.Report.Roles), len(an.Report.Findings), err)
	}
}
