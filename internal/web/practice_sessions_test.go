package web

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"

	"github.com/mrolegmir/cs-demo-coach/internal/detect"
	"github.com/mrolegmir/cs-demo-coach/internal/match"
	"github.com/mrolegmir/cs-demo-coach/internal/store"
)

func TestMatchPracticeLifecycle(t *testing.T) {
	e := newAuthEnv(t)
	session := cookie(e.login(admin), sessionCookie)
	id, err := e.st.CreateMatch(t.Context(), store.Upload{Filename: "training.dem", By: admin})
	if err != nil {
		t.Fatal(err)
	}
	m := &match.Match{Players: []match.Player{{ID: admin, Name: "player01"}}, Rounds: []match.Round{{Players: []match.PlayerRound{{Player: admin, Side: match.SideT}}}}}
	if err := e.st.SaveDetection(t.Context(), id, m, detect.Report{}); err != nil {
		t.Fatal(err)
	}
	base := "/matches/" + strconv.FormatInt(id, 10)
	post := func(path string, values url.Values, origin string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(http.MethodPost, path, strings.NewReader(values.Encode()))
		r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		r.Header.Set("Origin", origin)
		r.AddCookie(session)
		w := httptest.NewRecorder()
		e.h.ServeHTTP(w, r)
		return w
	}
	values := url.Values{"drill": {"two-pop-flashes"}, "minutes": {"20"}}
	if got := post(base+"/practice/start", values, "https://elsewhere.example"); got.Code != http.StatusForbidden {
		t.Fatalf("cross-site start: %d", got.Code)
	}
	for range 2 {
		if got := post(base+"/practice/start", values, publicURL); got.Code != http.StatusSeeOther {
			t.Fatalf("start: %d %s", got.Code, got.Body.String())
		}
	}
	active, err := e.st.ActivePractice(t.Context(), admin)
	if err != nil || len(active) != 1 {
		t.Fatalf("active: %+v %v", active, err)
	}
	for _, path := range []string{base, "/players/" + strconv.FormatUint(uint64(admin), 10)} {
		w := e.do(http.MethodGet, path, session)
		if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), "Practice in progress") {
			t.Fatalf("missing active practice: %d %s", w.Code, w.Body.String())
		}
	}
	finish := "/practice/" + strconv.FormatInt(active[0].ID, 10) + "/finish"
	if got := post(finish, url.Values{"minutes": {"17"}}, publicURL); got.Code != http.StatusSeeOther {
		t.Fatalf("finish: %d", got.Code)
	}
	log, err := e.st.ListPractice(t.Context(), admin)
	if err != nil || len(log) != 1 || log[0].Minutes != 17 {
		t.Fatalf("completed log: %+v %v", log, err)
	}
}
