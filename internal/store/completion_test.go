package store

import (
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/mrolegmir/cs-demo-coach/internal/coach"
	"github.com/mrolegmir/cs-demo-coach/internal/detect"
	"github.com/mrolegmir/cs-demo-coach/internal/match"
)

func TestCompletionIsAtomicAndFencesOldAttempts(t *testing.T) {
	s := testStore(t)
	ctx := t.Context()
	id, err := s.CreateMatch(ctx, Upload{Filename: "test.dem"})
	if err != nil {
		t.Fatal(err)
	}
	a, err := s.ClaimJob(ctx, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.Exec(ctx, `UPDATE jobs SET locked_at=now()-interval '2 hours' WHERE id=$1`, a.ID); err != nil {
		t.Fatal(err)
	}
	b, err := s.ClaimJob(ctx, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	m := &match.Match{Players: []match.Player{{ID: 1, Name: "p1"}}}
	if err := s.FinishDetection(ctx, a, m, detect.Report{}); !errors.Is(err, ErrLease) {
		t.Fatalf("old finish: %v", err)
	}
	// An invalid child row must roll back both match data and completion.
	bad := &match.Match{Players: []match.Player{{ID: 1}, {ID: 1}}}
	if err := s.FinishDetection(ctx, b, bad, detect.Report{}); err == nil {
		t.Fatal("invalid result accepted")
	}
	var state string
	if err := s.db.QueryRow(ctx, `SELECT status FROM jobs WHERE id=$1`, b.ID).Scan(&state); err != nil || state != "running" {
		t.Fatalf("partial finish %s %v", state, err)
	}
	if _, err := s.GetAnalysis(ctx, id); err == nil {
		t.Fatal("partial analysis committed")
	}
	if err := s.FinishDetection(ctx, b, m, detect.Report{}); err != nil {
		t.Fatal(err)
	}
	if err := s.FinishDetection(ctx, b, m, detect.Report{}); err != nil {
		t.Fatalf("lost response retry: %v", err)
	}
	for _, j := range []*Job{a, b} {
		if err := s.FailAttempt(ctx, j, true, 0, "late retry"); !errors.Is(err, ErrLease) {
			t.Fatalf("late retry: %v", err)
		}
		if err := s.FailAttempt(ctx, j, false, 0, "late failure"); !errors.Is(err, ErrLease) {
			t.Fatalf("late fail: %v", err)
		}
	}
	ids, err := s.PendingDemoCleanup(ctx)
	if err != nil || len(ids) != 1 || ids[0] != id {
		t.Fatalf("cleanup %v %v", ids, err)
	}
	if err := s.MarkDemoDeleted(ctx, id); err != nil {
		t.Fatal(err)
	}
	ids, err = s.PendingDemoCleanup(ctx)
	if err != nil || len(ids) != 0 {
		t.Fatalf("cleanup acknowledged %v %v", ids, err)
	}
}

func TestManualBriefAndCoachingShareAtomicQuota(t *testing.T) {
	s := testStore(t)
	s.limits = Limits{Coaching: 1}
	ctx := t.Context()
	if _, err := s.db.Exec(ctx, `INSERT INTO users (steam_id) VALUES (1)`); err != nil {
		t.Fatal(err)
	}
	ids := make([]int64, 8)
	for i := range ids {
		id, err := s.CreateMatch(ctx, Upload{Filename: "test.dem", By: 1})
		if err != nil {
			t.Fatal(err)
		}
		ids[i] = id
		if err := s.SaveDetection(ctx, id, &match.Match{}, detect.Report{}); err != nil {
			t.Fatal(err)
		}
	}
	var accepted atomic.Int32
	var wg sync.WaitGroup
	start := make(chan struct{})
	for i, id := range ids {
		wg.Go(func() {
			<-start
			var err error
			if i%2 == 0 {
				err = s.RequestCoaching(ctx, id, []match.PlayerID{1}, 1, "ru")
			} else {
				_, err = s.CreateBrief(ctx, &Brief{SteamID: 1, RequestedBy: 1, PeriodStart: time.Now().Add(-time.Hour), PeriodEnd: time.Now(), Input: &coach.Input{}})
			}
			if err == nil {
				accepted.Add(1)
			} else if !errors.Is(err, ErrQuota) {
				t.Errorf("request: %v", err)
			}
		})
	}
	close(start)
	wg.Wait()
	if accepted.Load() != 1 {
		t.Fatalf("accepted %d requests for quota=1", accepted.Load())
	}
}

func TestConcurrentUploadsRespectQuota(t *testing.T) {
	s := testStore(t)
	s.limits = Limits{Uploads: 1}
	ctx := t.Context()
	if _, err := s.db.Exec(ctx, `INSERT INTO users (steam_id) VALUES (1)`); err != nil {
		t.Fatal(err)
	}
	var accepted atomic.Int32
	var wg sync.WaitGroup
	for range 8 {
		wg.Go(func() {
			_, err := s.CreateMatch(ctx, Upload{Filename: "test.dem", By: 1})
			if err == nil {
				accepted.Add(1)
			} else if !errors.Is(err, ErrQuota) {
				t.Errorf("upload: %v", err)
			}
		})
	}
	wg.Wait()
	if accepted.Load() != 1 {
		t.Fatalf("accepted %d uploads", accepted.Load())
	}
}

func TestAutomaticAndManualCoachingShareAtomicQuota(t *testing.T) {
	s := testStore(t)
	s.limits = Limits{Coaching: 1}
	ctx := t.Context()
	if _, err := s.db.Exec(ctx, `INSERT INTO users (steam_id) VALUES (1)`); err != nil {
		t.Fatal(err)
	}
	m := &match.Match{Players: []match.Player{{ID: 1, Name: "player"}}, Rounds: []match.Round{{Players: []match.PlayerRound{{Player: 1, Side: match.SideT}}}}}
	ids := make([]int64, 2)
	jobs := make([]*Job, 2)
	for i := range ids {
		var err error
		ids[i], err = s.CreateMatch(ctx, Upload{Filename: "test.dem", By: 1})
		if err != nil {
			t.Fatal(err)
		}
		jobs[i], err = s.ClaimJob(ctx, time.Hour)
		if err != nil {
			t.Fatal(err)
		}
	}
	if err := s.FinishDetection(ctx, jobs[0], m, detect.Report{}); err != nil {
		t.Fatal(err)
	}
	if err := s.SaveFriendSettings(ctx, 1, nil, true); err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	start := make(chan struct{})
	wg.Go(func() {
		<-start
		if err := s.RequestCoaching(ctx, ids[0], []match.PlayerID{1}, 1, "ru"); err != nil && !errors.Is(err, ErrQuota) {
			t.Errorf("manual coaching: %v", err)
		}
	})
	wg.Go(func() {
		<-start
		if err := s.FinishDetection(ctx, jobs[1], m, detect.Report{}); err != nil {
			t.Errorf("automatic coaching must not fail detection: %v", err)
		}
	})
	close(start)
	wg.Wait()
	var count int
	if err := s.db.QueryRow(ctx, `SELECT count(*) FROM jobs WHERE kind='coach'`).Scan(&count); err != nil || count != 1 {
		t.Fatalf("coaching jobs=%d err=%v, want one shared quota slot", count, err)
	}
	for _, id := range ids {
		mt, err := s.GetMatch(ctx, id)
		if err != nil || (mt.Status != StatusDetected && mt.Status != StatusCoaching) {
			t.Fatalf("detection was rolled back: %+v %v", mt, err)
		}
	}
}

func TestLateAdviceCannotOverwriteNewRequest(t *testing.T) {
	s := testStore(t)
	ctx := t.Context()
	id, err := s.CreateMatch(ctx, Upload{Filename: "test.dem"})
	if err != nil {
		t.Fatal(err)
	}
	detectJob, err := s.ClaimJob(ctx, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.FinishDetection(ctx, detectJob, &match.Match{}, detect.Report{}); err != nil {
		t.Fatal(err)
	}
	if err := s.RequestCoaching(ctx, id, []match.PlayerID{1}, 0, "ru"); err != nil {
		t.Fatal(err)
	}
	a, err := s.ClaimJob(ctx, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.Exec(ctx, `UPDATE jobs SET locked_at=now()-interval '2 hours' WHERE id=$1`, a.ID); err != nil {
		t.Fatal(err)
	}
	b, err := s.ClaimJob(ctx, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.FinishAdvice(ctx, a, &coach.Advice{Notes: []string{"stale"}}, "rules"); !errors.Is(err, ErrLease) {
		t.Fatalf("stale advice accepted: %v", err)
	}
	if err := s.FinishAdvice(ctx, b, &coach.Advice{Notes: []string{"current"}}, "rules"); err != nil {
		t.Fatal(err)
	}
	if err := s.RequestCoaching(ctx, id, []match.PlayerID{2}, 0, "en"); err != nil {
		t.Fatal(err)
	}
	// Duplicate success for the previous job must not restore the old advice.
	if err := s.FinishAdvice(ctx, b, &coach.Advice{Notes: []string{"late duplicate"}}, "rules"); err != nil {
		t.Fatal(err)
	}
	mt, err := s.GetMatch(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	an, err := s.GetAnalysis(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if mt.Status != StatusCoaching || len(mt.Party) != 1 || mt.Party[0] != 2 || an.Advice != nil {
		t.Fatal("previous job overwrote new request")
	}
}
