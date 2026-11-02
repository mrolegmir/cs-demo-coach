package worker

import (
	"fmt"
	"log/slog"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/mrolegmir/cs-demo-coach/internal/brief"
	"github.com/mrolegmir/cs-demo-coach/internal/coach"
	"github.com/mrolegmir/cs-demo-coach/internal/match"
	"github.com/mrolegmir/cs-demo-coach/internal/store"
	"github.com/mrolegmir/cs-demo-coach/internal/web"
)

// remoteWorker returns a worker that reaches the store only through a real
// API server, the way a worker on a home PC does.
func remoteWorker(t *testing.T) (*Worker, *store.Store, string) {
	t.Helper()
	w, st := newWorker(t, nil)
	srv := &web.Server{Store: st, Jobs: st, DemoDir: t.TempDir(), WorkerToken: "token", Log: slog.New(slog.DiscardHandler)}
	h, err := srv.Handler()
	if err != nil {
		t.Fatal(err)
	}
	ts := httptest.NewServer(h)
	t.Cleanup(ts.Close)
	api := API{BaseURL: ts.URL, Token: "token"}
	w.Queue, w.Demos = api, api
	return w, st, srv.DemoDir
}

func TestRemoteCoachJob(t *testing.T) {
	w, st, _ := remoteWorker(t)
	ctx := t.Context()
	id := detected(t, st)
	if err := st.RequestCoaching(ctx, id, []match.PlayerID{1, 2}, 0, "ru"); err != nil {
		t.Fatal(err)
	}
	if ok, err := w.RunOnce(ctx); !ok || err != nil {
		t.Fatalf("RunOnce = %v, %v", ok, err)
	}
	if m := status(t, st, id); m.Status != store.StatusCoached {
		t.Fatalf("match = %s %q", m.Status, m.Error)
	}
	if ok, err := w.RunOnce(ctx); ok || err != nil {
		t.Fatalf("empty queue: RunOnce = %v, %v", ok, err)
	}
}

func TestRemoteCoachingLanguages(t *testing.T) {
	for _, language := range []string{"ru", "en"} {
		t.Run(language, func(t *testing.T) {
			w, st, _ := remoteWorker(t)
			id := detected(t, st)
			if err := st.RequestCoaching(t.Context(), id, []match.PlayerID{1, 2}, 0, language); err != nil {
				t.Fatal(err)
			}
			// Claim, retry immediately, then let the remote worker claim it again.
			// The stored language must survive both queue claims and HTTP.
			job, err := st.ClaimJob(t.Context(), time.Hour)
			if err != nil || job.Language != language {
				t.Fatalf("queued language: %+v, %v", job, err)
			}
			if err := st.FailAttempt(t.Context(), job, true, 0, "transient failure"); err != nil {
				t.Fatal(err)
			}
			if ok, err := w.RunOnce(t.Context()); !ok || err != nil {
				t.Fatalf("RunOnce: %v, %v", ok, err)
			}
			an, err := st.GetAnalysis(t.Context(), id)
			if err != nil || an.Advice == nil || an.Advice.Language != language {
				t.Fatalf("stored report: %+v, %v", an, err)
			}
			in := coach.NewInput(an.Match, *an.Report, []match.PlayerID{1, 2})
			in.Language = language
			if err := in.Validate(an.Advice); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestRemoteCorruptDemo(t *testing.T) {
	w, st, dir := remoteWorker(t)
	id, err := st.CreateMatch(t.Context(), store.Upload{Filename: "bad.dem"})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, fmt.Sprintf("%d.dem", id)), []byte("garbage"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := w.RunOnce(t.Context()); err != nil {
		t.Fatal(err)
	}
	if m := status(t, st, id); m.Status != store.StatusFailed || !strings.Contains(m.Error, "could not read the demo") {
		t.Fatalf("match = %s %q", m.Status, m.Error)
	}
}

func TestRemoteDetectJob(t *testing.T) {
	demoFile := filepath.Join("..", "..", "demos", "Anubis_ShortMatch_2023-08-04.dem")
	data, err := os.ReadFile(demoFile)
	if err != nil {
		t.Skip("demo not available")
	}
	w, st, dir := remoteWorker(t)
	id, err := st.CreateMatch(t.Context(), store.Upload{Filename: "anubis.dem"})
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, fmt.Sprintf("%d.dem", id))
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	if ok, err := w.RunOnce(t.Context()); !ok || err != nil {
		t.Fatalf("RunOnce = %v, %v", ok, err)
	}
	if m := status(t, st, id); m.Status != store.StatusDetected || m.Map != "de_anubis" {
		t.Fatalf("match = %+v", m)
	}
	srv := web.Server{Jobs: st, DemoDir: dir}
	if err := srv.CleanupDemos(t.Context()); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Errorf("demo was not deleted after parsing: %v", err)
	}
}

func TestRemoteRejectsWrongToken(t *testing.T) {
	w, _, _ := remoteWorker(t)
	api := w.Queue.(API)
	api.Token = "wrong"
	if _, err := api.ClaimJob(t.Context(), 0); err == nil || !strings.Contains(err.Error(), "401") {
		t.Fatalf("claim with a wrong token: %v", err)
	}
}

func TestRemoteBriefJob(t *testing.T) {
	w, st, _ := remoteWorker(t)
	ctx := t.Context()
	// Player 1 of the fixture match is a user; detected() uploads it now.
	if err := st.EnsureAdmin(ctx, 1); err != nil {
		t.Fatal(err)
	}
	detected(t, st)

	// Briefs are due at the last scheduled moment, here within the next
	// two hours, covering the week before it.
	target := time.Now().UTC().Add(2 * time.Hour)
	p := &brief.Planner{
		Store:    st,
		Schedule: brief.Schedule{Weekday: target.Weekday(), Hour: target.Hour(), Location: time.UTC},
		Log:      slog.New(slog.DiscardHandler),
	}
	if err := p.RunOnce(ctx, target); err != nil {
		t.Fatal(err)
	}
	briefs, err := st.ListBriefs(ctx, 1, 10)
	if err != nil || len(briefs) != 1 || briefs[0].Status != store.BriefQueued {
		t.Fatalf("briefs = %+v, %v", briefs, err)
	}
	// Running the planner again does not make a second brief.
	if err := p.RunOnce(ctx, target); err != nil {
		t.Fatal(err)
	}
	if again, _ := st.ListBriefs(ctx, 1, 10); len(again) != 1 {
		t.Fatalf("planner made %d briefs, want 1", len(again))
	}

	if ok, err := w.RunOnce(ctx); !ok || err != nil {
		t.Fatalf("RunOnce = %v, %v", ok, err)
	}
	b, err := st.GetBrief(ctx, briefs[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	if b.Status != store.BriefReady || b.Advice == nil || b.Coach != "rules" || len(b.Party) == 0 {
		t.Fatalf("brief = %s %q, advice %v", b.Status, b.Error, b.Advice)
	}
	if b.Input.Language != "ru" || b.Advice.Language != "ru" {
		t.Fatalf("scheduled brief did not default to Russian: %+v", b.Advice)
	}
	if err := b.Input.Validate(b.Advice); err != nil {
		t.Errorf("stored brief advice is not grounded: %v", err)
	}
}
