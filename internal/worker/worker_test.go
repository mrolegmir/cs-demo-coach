package worker

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/mrolegmir/cs-demo-coach/internal/coach"
	"github.com/mrolegmir/cs-demo-coach/internal/detect"
	"github.com/mrolegmir/cs-demo-coach/internal/match"
	"github.com/mrolegmir/cs-demo-coach/internal/store"
	"github.com/mrolegmir/cs-demo-coach/internal/store/storetest"
)

// fakeDemos serves demos from memory or fails with err.
type fakeDemos struct {
	data []byte
	path string // read the demo from disk instead
	err  error
}

func (f *fakeDemos) Open(context.Context, int64) (io.ReadCloser, error) {
	if f.err != nil {
		return nil, f.err
	}
	if f.path != "" {
		return os.Open(f.path)
	}
	return io.NopCloser(strings.NewReader(string(f.data))), nil
}

func newWorker(t *testing.T, demos Demos) (*Worker, *store.Store) {
	t.Helper()
	st, err := store.Open(t.Context(), storetest.URL(t), store.Limits{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(st.Close)
	return &Worker{
		Queue:       st,
		Demos:       demos,
		Coach:       coach.Rules{PerPlayer: 2},
		CoachName:   "rules",
		Log:         slog.New(slog.DiscardHandler),
		Poll:        10 * time.Millisecond,
		MaxAttempts: 2,
		Timeout:     time.Minute,
		Stale:       time.Minute,
	}, st
}

func status(t *testing.T, st *store.Store, id int64) *store.Match {
	t.Helper()
	m, err := st.GetMatch(t.Context(), id)
	if err != nil {
		t.Fatal(err)
	}
	return m
}

func TestCorruptDemoFailsImmediately(t *testing.T) {
	w, st := newWorker(t, &fakeDemos{data: []byte("garbage")})
	id, err := st.CreateMatch(t.Context(), store.Upload{Filename: "bad.dem"})
	if err != nil {
		t.Fatal(err)
	}
	if ok, err := w.RunOnce(t.Context()); !ok || err != nil {
		t.Fatalf("RunOnce = %v, %v", ok, err)
	}
	m := status(t, st, id)
	if m.Status != store.StatusFailed || !strings.Contains(m.Error, "could not read the demo") {
		t.Fatalf("match = %s %q", m.Status, m.Error)
	}
	if ok, _ := w.RunOnce(t.Context()); ok {
		t.Fatal("a corrupt demo was retried")
	}
}

func TestUnavailableDemoIsRetried(t *testing.T) {
	w, st := newWorker(t, &fakeDemos{err: errors.New("api down")})
	id, err := st.CreateMatch(t.Context(), store.Upload{Filename: "x.dem"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w.RunOnce(t.Context()); err != nil {
		t.Fatal(err)
	}
	if m := status(t, st, id); m.Status != store.StatusUploaded {
		t.Fatalf("after first failure match is %s, want it waiting for a retry", m.Status)
	}
	// The retry waits for its backoff.
	if _, err := st.ClaimJob(t.Context(), time.Hour); !errors.Is(err, store.ErrNoJob) {
		t.Fatalf("retry was not delayed: %v", err)
	}
}

func TestCoachJob(t *testing.T) {
	w, st := newWorker(t, &fakeDemos{})
	ctx := t.Context()
	id := detected(t, st)
	if err := st.RequestCoaching(ctx, id, []match.PlayerID{1, 2}, 0, "ru"); err != nil {
		t.Fatal(err)
	}

	if ok, err := w.RunOnce(ctx); !ok || err != nil {
		t.Fatalf("RunOnce = %v, %v", ok, err)
	}
	if s := status(t, st, id); s.Status != store.StatusCoached || s.Coach != "rules" {
		t.Fatalf("match = %s, coach %q", s.Status, s.Coach)
	}
	an, err := st.GetAnalysis(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if an.Advice == nil || len(an.Advice.Players) != 2 {
		t.Fatalf("advice = %+v", an.Advice)
	}
}

func TestJobThatKeepsCrashingWorkersFails(t *testing.T) {
	w, st := newWorker(t, &fakeDemos{err: errors.New("unreachable")})
	ctx := t.Context()
	id, err := st.CreateMatch(ctx, store.Upload{Filename: "crash.dem"})
	if err != nil {
		t.Fatal(err)
	}
	// Workers that die mid-job leave it running; each takeover counts.
	for range w.MaxAttempts {
		if _, err := st.ClaimJob(ctx, 0); err != nil {
			t.Fatal(err)
		}
	}
	w.Stale = 0
	if ok, err := w.RunOnce(ctx); !ok || err != nil {
		t.Fatalf("RunOnce = %v, %v", ok, err)
	}
	if m := status(t, st, id); m.Status != store.StatusFailed || !strings.Contains(m.Error, "too many times") {
		t.Fatalf("match = %s %q", m.Status, m.Error)
	}
	if _, err := st.ClaimJob(ctx, 0); !errors.Is(err, store.ErrNoJob) {
		t.Fatalf("failed job is still claimable: %v", err)
	}
}

// failingCoach always fails with err.
type failingCoach struct{ err error }

func (f failingCoach) Advise(context.Context, *coach.Input) (*coach.Advice, error) { return nil, f.err }

// invalidCoach returns successfully, but its output is not grounded.
type invalidCoach struct{}

func (invalidCoach) Advise(context.Context, *coach.Input) (*coach.Advice, error) {
	return &coach.Advice{Players: []coach.PlayerAdvice{{Player: "not in this party"}}}, nil
}

func TestEveryCoachMustPassValidation(t *testing.T) {
	w, st := newWorker(t, &fakeDemos{})
	w.Coach = invalidCoach{}
	ctx := t.Context()
	id := detected(t, st)
	if err := st.RequestCoaching(ctx, id, []match.PlayerID{1}, 0, "ru"); err != nil {
		t.Fatal(err)
	}
	if _, err := w.RunOnce(ctx); err != nil {
		t.Fatal(err)
	}
	if mt := status(t, st, id); mt.Status != store.StatusDetected || !strings.Contains(mt.Error, "not in the party") {
		t.Fatalf("invalid advice was not rejected: %+v", mt)
	}
	if _, err := st.ClaimJob(ctx, 0); !errors.Is(err, store.ErrNoJob) {
		t.Fatalf("invalid output was queued for another paid retry: %v", err)
	}
	if err := st.EnsureAdmin(ctx, 1); err != nil {
		t.Fatal(err)
	}
	briefID, err := st.CreateBrief(ctx, &store.Brief{
		SteamID: 1, PeriodStart: time.Now().Add(-7 * 24 * time.Hour), PeriodEnd: time.Now(),
		Party: []match.PlayerID{1}, Input: &coach.Input{Party: []string{"player01"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w.RunOnce(ctx); err != nil {
		t.Fatal(err)
	}
	b, err := st.GetBrief(ctx, briefID)
	if err != nil {
		t.Fatal(err)
	}
	if b.Status != store.BriefFailed || b.Advice != nil || !strings.Contains(b.Error, "not in the party") {
		t.Fatalf("invalid brief advice was saved: %+v", b)
	}
}

func TestFailedCoachingKeepsFindings(t *testing.T) {
	w, st := newWorker(t, &fakeDemos{})
	w.Coach = failingCoach{coach.ErrUngrounded}
	ctx := t.Context()
	id := detected(t, st)
	if err := st.RequestCoaching(ctx, id, []match.PlayerID{1}, 0, "ru"); err != nil {
		t.Fatal(err)
	}
	if _, err := w.RunOnce(ctx); err != nil {
		t.Fatal(err)
	}
	m := status(t, st, id)
	if m.Status != store.StatusDetected || !strings.Contains(m.Error, "validation") {
		t.Fatalf("match = %s %q, want detected with the coaching error", m.Status, m.Error)
	}
	if err := st.RequestCoaching(ctx, id, []match.PlayerID{1}, 0, "ru"); err != nil {
		t.Fatalf("coaching cannot be retried: %v", err)
	}
}

// detected creates a match whose detection is done, from a fixture.
func detected(t *testing.T, st *store.Store) int64 {
	t.Helper()
	ctx := t.Context()
	m, err := match.LoadFile(filepath.Join("..", "..", "testdata", "matches", "arsham_infernocs2_1stSept23.json.gz"))
	if err != nil {
		t.Fatal(err)
	}
	id, err := st.CreateMatch(ctx, store.Upload{Filename: "inferno.dem"})
	if err != nil {
		t.Fatal(err)
	}
	job, err := st.ClaimJob(ctx, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.SaveDetection(ctx, id, m, detect.Run(m, detect.Default())); err != nil {
		t.Fatal(err)
	}
	if err := st.FinishDetection(ctx, job, m, detect.Run(m, detect.Default())); err != nil {
		t.Fatal(err)
	}
	return id
}

// TestDetectJob runs the whole detect path on a real demo when one is
// available locally (see internal/demo tests).
func TestDetectJob(t *testing.T) {
	path := filepath.Join("..", "..", "demos", "Anubis_ShortMatch_2023-08-04.dem")
	if _, err := os.Stat(path); err != nil {
		t.Skip("demo not available")
	}
	demos := &fakeDemos{path: path}
	w, st := newWorker(t, demos)
	id, err := st.CreateMatch(t.Context(), store.Upload{Filename: "anubis.dem"})
	if err != nil {
		t.Fatal(err)
	}
	if ok, err := w.RunOnce(t.Context()); !ok || err != nil {
		t.Fatalf("RunOnce = %v, %v", ok, err)
	}
	m := status(t, st, id)
	if m.Status != store.StatusDetected || m.Map != "de_anubis" || len(m.Players) != 10 {
		t.Fatalf("match = %+v", m)
	}
	ids, err := st.PendingDemoCleanup(t.Context())
	if err != nil || len(ids) != 1 || ids[0] != id {
		t.Fatalf("cleanup not scheduled: %v %v", ids, err)
	}
}

type lostDetectionReply struct{ Queue }

func (q lostDetectionReply) FinishDetection(ctx context.Context, j *store.Job, m *match.Match, rep detect.Report) error {
	if err := q.Queue.FinishDetection(ctx, j, m, rep); err != nil {
		return err
	}
	return errors.New("connection lost after commit")
}

func TestLostCompletionReplyKeepsSavedResult(t *testing.T) {
	w, st := newWorker(t, &fakeDemos{err: errors.New("demo already deleted")})
	ctx := t.Context()
	id, err := st.CreateMatch(ctx, store.Upload{Filename: "recovered.dem"})
	if err != nil {
		t.Fatal(err)
	}
	// A legacy worker saved the result but stopped before acknowledging the job.
	m := &match.Match{Players: []match.Player{{ID: 1, Name: "p1"}}}
	if err := st.SaveDetection(ctx, id, m, detect.Report{}); err != nil {
		t.Fatal(err)
	}
	w.Queue = lostDetectionReply{Queue: st}
	if _, err := w.RunOnce(ctx); !errors.Is(err, store.ErrLease) {
		t.Fatalf("lost reply: %v", err)
	}
	if got := status(t, st, id); got.Status != store.StatusDetected {
		t.Fatalf("saved result hidden: %s", got.Status)
	}
	if ok, err := w.RunOnce(ctx); ok || err != nil {
		t.Fatalf("committed job ran twice: %v %v", ok, err)
	}
	ids, err := st.PendingDemoCleanup(ctx)
	if err != nil || len(ids) != 1 || ids[0] != id {
		t.Fatalf("cleanup lost: %v %v", ids, err)
	}
}
