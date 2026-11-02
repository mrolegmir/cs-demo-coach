package web

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/mrolegmir/cs-demo-coach/internal/detect"
	"github.com/mrolegmir/cs-demo-coach/internal/jobwire"
	"github.com/mrolegmir/cs-demo-coach/internal/match"
	"github.com/mrolegmir/cs-demo-coach/internal/store"
)

func TestCompletionAdaptersAndDurableCleanup(t *testing.T) {
	e := newAuthEnv(t)
	e.srv.Jobs = e.st
	h, err := e.srv.Handler()
	if err != nil {
		t.Fatal(err)
	}
	ctx := t.Context()
	if err := e.st.SaveFriendSettings(ctx, admin, []match.PlayerID{friend}, true); err != nil {
		t.Fatal(err)
	}
	m := &match.Match{Players: []match.Player{{ID: admin, Name: "same"}, {ID: friend, Name: "same"}}, Rounds: []match.Round{{Players: []match.PlayerRound{{Player: admin, Side: match.SideT}, {Player: friend, Side: match.SideT}}}}}
	var directParty []match.PlayerID
	for _, remote := range []bool{false, true} {
		id, err := e.st.CreateMatch(ctx, store.Upload{Filename: "synthetic.dem", By: admin})
		if err != nil {
			t.Fatal(err)
		}
		// Earlier automatic coach jobs may also be waiting.
		var j *store.Job
		for {
			j, err = e.st.ClaimJob(ctx, time.Hour)
			if err != nil {
				t.Fatal(err)
			}
			if j.Kind == store.JobDetect {
				break
			}
		}
		if err := os.WriteFile(e.srv.demoPath(id), []byte("synthetic"), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := e.srv.CleanupDemos(ctx); err != nil {
			t.Fatal(err)
		}
		if _, err := os.Stat(e.srv.demoPath(id)); err != nil {
			t.Fatal("unfinished demo deleted")
		}
		if remote {
			body, err := json.Marshal(jobwire.DetectionRequest{Attempt: j.Attempts, Match: m, Report: detect.Report{}})
			if err != nil {
				t.Fatal(err)
			}
			for range 2 {
				r := httptest.NewRequest(http.MethodPut, fmt.Sprintf("/internal/v2/jobs/%d/detection", j.ID), bytes.NewReader(body))
				r.Header.Set("Authorization", "Bearer secret")
				w := httptest.NewRecorder()
				h.ServeHTTP(w, r)
				if w.Code != http.StatusNoContent {
					t.Fatalf("finish %d: %s", w.Code, w.Body.String())
				}
			}
		} else if err := e.st.FinishDetection(ctx, j, m, detect.Report{}); err != nil {
			t.Fatal(err)
		}
		mt, err := e.st.GetMatch(ctx, id)
		if err != nil || mt.Status != store.StatusCoaching || len(mt.Party) != 2 {
			t.Fatalf("auto-coach: %+v %v", mt, err)
		}
		if !remote {
			directParty = mt.Party
		} else if mt.Party[0] != directParty[0] || mt.Party[1] != directParty[1] {
			t.Fatal("adapters disagree")
		}
		// Simulate the cleanup process dying after removal, before its DB ack.
		if err := os.Remove(e.srv.demoPath(id)); err != nil {
			t.Fatal(err)
		}
		for range 2 {
			if err := e.srv.CleanupDemos(ctx); err != nil {
				t.Fatal(err)
			}
		}
	}
	ids, err := e.st.PendingDemoCleanup(ctx)
	if err != nil || len(ids) != 0 {
		t.Fatalf("cleanup not acked %v %v", ids, err)
	}
	for _, path := range []string{"/internal/jobs/claim", "/internal/jobs/1/fail", "/internal/matches/1/fail"} {
		r := httptest.NewRequest(http.MethodPost, path, bytes.NewBufferString(`{}`))
		r.Header.Set("Authorization", "Bearer secret")
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Code != http.StatusConflict {
			t.Fatalf("legacy write %s: %d", path, w.Code)
		}
	}
}

func TestCleanupContinuesAfterRemovalError(t *testing.T) {
	e := newAuthEnv(t)
	e.srv.Jobs = e.st
	ctx := t.Context()
	ids := make([]int64, 2)
	for i := range ids {
		var err error
		ids[i], err = e.st.CreateMatch(ctx, store.Upload{Filename: "test.dem"})
		if err != nil {
			t.Fatal(err)
		}
		j, err := e.st.ClaimJob(ctx, time.Hour)
		if err != nil {
			t.Fatal(err)
		}
		if err := e.st.FinishDetection(ctx, j, &match.Match{}, detect.Report{}); err != nil {
			t.Fatal(err)
		}
	}
	// A nonempty directory is an unremovable file path on every supported OS.
	blocked := e.srv.demoPath(ids[0])
	if err := os.Mkdir(blocked, 0o700); err != nil {
		t.Fatal(err)
	}
	child := filepath.Join(blocked, "child")
	if err := os.WriteFile(child, []byte("keep"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(e.srv.demoPath(ids[1]), []byte("demo"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := e.srv.CleanupDemos(ctx); err == nil {
		t.Fatal("removal error was hidden")
	}
	pending, err := e.st.PendingDemoCleanup(ctx)
	if err != nil || len(pending) != 1 || pending[0] != ids[0] {
		t.Fatalf("unrelated cleanup was blocked: %v %v", pending, err)
	}
	if _, err := os.Stat(e.srv.demoPath(ids[1])); !os.IsNotExist(err) {
		t.Fatalf("second demo was not removed: %v", err)
	}
	if err := os.Remove(child); err != nil {
		t.Fatal(err)
	}
	if err := e.srv.CleanupDemos(ctx); err != nil {
		t.Fatalf("cleanup did not recover: %v", err)
	}
}
