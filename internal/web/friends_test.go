package web

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/mrolegmir/cs-demo-coach/internal/detect"
	"github.com/mrolegmir/cs-demo-coach/internal/jobwire"
	"github.com/mrolegmir/cs-demo-coach/internal/match"
	"github.com/mrolegmir/cs-demo-coach/internal/store"
)

func TestFriendsAreOwnedAndFormsAreProtected(t *testing.T) {
	e := newAuthEnv(t)
	session := cookie(e.login(admin), sessionCookie)
	post := func(values url.Values, origin string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(http.MethodPost, "/friends", strings.NewReader(values.Encode()))
		r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		r.Header.Set("Origin", origin)
		r.AddCookie(session)
		w := httptest.NewRecorder()
		e.h.ServeHTTP(w, r)
		return w
	}
	values := url.Values{"steam_id": {"https://steamcommunity.com/profiles/" + strconv.FormatUint(uint64(friend), 10) + "/"}, "automatic": {"on"}}
	if w := post(values, "https://elsewhere.example"); w.Code != http.StatusForbidden {
		t.Fatal("cross-site friend edit accepted")
	}
	if w := post(values, publicURL); w.Code != http.StatusSeeOther {
		t.Fatalf("save: %d %s", w.Code, w.Body.String())
	}
	got, err := e.st.Friends(t.Context(), admin)
	if err != nil || !slices.Equal(got, []match.PlayerID{friend}) {
		t.Fatalf("friends: %v %v", got, err)
	}
	if err := e.st.EnsureAdmin(t.Context(), other); err != nil {
		t.Fatal(err)
	}
	got, err = e.st.Friends(t.Context(), other)
	if err != nil || len(got) != 0 {
		t.Fatal("friends leaked to another owner")
	}
	if w := post(url.Values{"friend": {strconv.FormatUint(uint64(admin), 10)}}, publicURL); !strings.Contains(w.Header().Get("Location"), "error=") {
		t.Fatal("self accepted as friend")
	}
	got, _ = e.st.Friends(t.Context(), admin)
	if len(got) != 1 {
		t.Fatal("invalid form erased saved friends")
	}
	if w := e.do(http.MethodGet, "/friends", session); w.Code != http.StatusOK || !strings.Contains(w.Body.String(), "Automatically analyze new matches") {
		t.Fatalf("settings page failed: %d", w.Code)
	}
}

func TestDetectionEndpointAutomaticallyQueuesSavedFriends(t *testing.T) {
	e := newAuthEnv(t)
	e.srv.Jobs = e.st
	h, err := e.srv.Handler()
	if err != nil {
		t.Fatal(err)
	}
	if err := e.st.SaveFriendSettings(t.Context(), admin, []match.PlayerID{friend, other}, true); err != nil {
		t.Fatal(err)
	}
	id, err := e.st.CreateMatch(t.Context(), store.Upload{Filename: "auto.dem", By: admin})
	if err != nil {
		t.Fatal(err)
	}
	m := &match.Match{Players: []match.Player{{ID: admin, Name: "player01"}, {ID: friend, Name: "player02"}, {ID: other, Name: "player03"}}, Rounds: []match.Round{{Players: []match.PlayerRound{{Player: admin, Side: match.SideT}, {Player: friend, Side: match.SideT}, {Player: other, Side: match.SideCT}}}}}
	job, err := e.st.ClaimJob(t.Context(), time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	body, err := json.Marshal(jobwire.DetectionRequest{Attempt: job.Attempts, Match: m, Report: detect.Report{}})
	if err != nil {
		t.Fatal(err)
	}
	for range 2 {
		r := httptest.NewRequest(http.MethodPut, "/internal/v2/jobs/"+strconv.FormatInt(job.ID, 10)+"/detection", strings.NewReader(string(body)))
		r.Header.Set("Authorization", "Bearer secret")
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Code != http.StatusNoContent {
			t.Fatalf("detection: %d %s", w.Code, w.Body.String())
		}
	}
	mt, err := e.st.GetMatch(t.Context(), id)
	if err != nil || mt.Status != store.StatusCoaching || !slices.Equal(mt.Party, []match.PlayerID{admin, friend}) {
		t.Fatalf("automatic group: %+v %v", mt, err)
	}
}

func TestFriendsPreview(t *testing.T) {
	dir := os.Getenv("CSDC_PREVIEW_DIR")
	if dir == "" {
		t.Skip("preview not requested")
	}
	s, _ := newServer(t, coachedStore(t))
	w := httptest.NewRecorder()
	s.render(w, httptest.NewRequest(http.MethodGet, "/friends", http.NoBody), "friends", friendsPage{Viewer: Viewer{User: &store.User{Name: "player01", SteamID: 1}}, Automatic: true, Contacts: []store.FriendContact{{ID: 76561198000000002, Name: "player02", Selected: true}, {ID: 76561198000000003, Name: "player03"}}})
	if w.Code != http.StatusOK {
		t.Fatal(w.Body.String())
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "friends.html"), w.Body.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestManualFriendsActionUsesSavedTeammates(t *testing.T) {
	e := newAuthEnv(t)
	session := cookie(e.login(admin), sessionCookie)
	if err := e.st.SaveFriendSettings(t.Context(), admin, []match.PlayerID{friend, other}, false); err != nil {
		t.Fatal(err)
	}
	id, err := e.st.CreateMatch(t.Context(), store.Upload{Filename: "manual-friends.dem", By: admin})
	if err != nil {
		t.Fatal(err)
	}
	m := &match.Match{Players: []match.Player{{ID: admin}, {ID: friend}, {ID: other}}, Rounds: []match.Round{{Players: []match.PlayerRound{{Player: admin, Side: match.SideT}, {Player: friend, Side: match.SideT}, {Player: other, Side: match.SideCT}}}}}
	if err := e.st.SaveDetection(t.Context(), id, m, detect.Report{}); err != nil {
		t.Fatal(err)
	}
	r := httptest.NewRequest(http.MethodPost, "/matches/"+strconv.FormatInt(id, 10)+"/coach", strings.NewReader("use_friends=1&player="+strconv.FormatUint(uint64(other), 10)))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	r.Header.Set("Origin", publicURL)
	r.AddCookie(session)
	w := httptest.NewRecorder()
	e.h.ServeHTTP(w, r)
	mt, err := e.st.GetMatch(t.Context(), id)
	if w.Code != http.StatusSeeOther || err != nil || !slices.Equal(mt.Party, []match.PlayerID{admin, friend}) {
		t.Fatalf("manual friends: status=%d match=%+v err=%v", w.Code, mt, err)
	}
	ms, err := e.st.PlayerMatches(t.Context(), admin)
	if err != nil || len(ms) != 1 || !ms[0].ManualParty {
		t.Fatal("manual selection not recorded for weekly exclusions")
	}
}
