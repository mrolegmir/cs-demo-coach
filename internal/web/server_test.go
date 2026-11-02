package web

import (
	"bytes"
	"context"
	"io"
	"log/slog"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/mrolegmir/cs-demo-coach/internal/coach"
	"github.com/mrolegmir/cs-demo-coach/internal/detect"
	"github.com/mrolegmir/cs-demo-coach/internal/match"
	"github.com/mrolegmir/cs-demo-coach/internal/store"
)

// fakeStore keeps one match in memory. Account methods come from the
// embedded UserStore and are only used by the auth tests, which run against
// a real database.
type fakeStore struct {
	UserStore
	TokenStore
	SteamLinkStore
	ImportStore
	TelegramStore
	BriefStore
	m                *store.Match
	analysis         *store.Analysis
	created          []string
	party            []match.PlayerID
	coachingLanguage string
	notReady         bool
}

func (f *fakeStore) CanView(context.Context, int64, *store.User) (bool, error) { return true, nil }

func (f *fakeStore) Friends(context.Context, match.PlayerID) ([]match.PlayerID, error) {
	return nil, nil
}

func (f *fakeStore) FriendSettings(context.Context, match.PlayerID) ([]store.FriendContact, bool, error) {
	return nil, false, nil
}

func (f *fakeStore) SaveFriendSettings(context.Context, match.PlayerID, []match.PlayerID, bool) error {
	return nil
}

func (f *fakeStore) PlayerMatches(context.Context, match.PlayerID) ([]store.PlayerMatch, error) {
	if f.analysis == nil {
		return nil, nil
	}
	return []store.PlayerMatch{{ID: f.m.ID, Map: f.m.Map, Report: *f.analysis.Report}}, nil
}

func (f *fakeStore) CanViewPlayer(context.Context, *store.User, match.PlayerID) (bool, error) {
	return true, nil
}

func (f *fakeStore) AddPractice(context.Context, match.PlayerID, store.Practice) (int64, error) {
	return 0, nil
}

func (f *fakeStore) DeletePractice(context.Context, match.PlayerID, int64) error { return nil }

func (f *fakeStore) StartPractice(context.Context, match.PlayerID, string, int) error { return nil }
func (f *fakeStore) ActivePractice(context.Context, match.PlayerID) ([]store.PracticeSession, error) {
	return nil, nil
}
func (f *fakeStore) FinishPractice(context.Context, match.PlayerID, int64, int) error { return nil }

func (f *fakeStore) PlayerName(context.Context, match.PlayerID) (string, error) {
	return "player03", nil
}

func (f *fakeStore) GetUsage(context.Context, match.PlayerID, time.Time) (store.Usage, error) {
	return store.Usage{}, nil
}

func (f *fakeStore) CreateMatch(_ context.Context, u store.Upload) (int64, error) {
	f.created = append(f.created, u.Filename)
	return int64(len(f.created)), nil
}

func (f *fakeStore) ListMatches(context.Context, *store.User, int) ([]store.Match, error) {
	if f.m == nil {
		return nil, nil
	}
	return []store.Match{*f.m}, nil
}

func (f *fakeStore) GetMatch(_ context.Context, id int64) (*store.Match, error) {
	if f.m == nil || f.m.ID != id {
		return nil, store.ErrNotFound
	}
	return f.m, nil
}

func (f *fakeStore) GetAnalysis(context.Context, int64) (*store.Analysis, error) {
	return f.analysis, nil
}

func (f *fakeStore) RequestCoaching(_ context.Context, _ int64, party []match.PlayerID, _ match.PlayerID, language string) error {
	if f.notReady {
		return store.ErrNotReady
	}
	f.party = party
	f.coachingLanguage = language
	return nil
}

func newServer(t *testing.T, st Store) (*Server, http.Handler) {
	t.Helper()
	s := &Server{Store: st, DemoDir: t.TempDir(), WorkerToken: "secret", Log: slog.New(slog.DiscardHandler)}
	h, err := s.Handler()
	if err != nil {
		t.Fatal(err)
	}
	return s, h
}

// coachedStore holds a fixture match analyzed by the rules coach for the
// team that started as T.
func coachedStore(t *testing.T) *fakeStore {
	t.Helper()
	m, err := match.LoadFile(filepath.Join("..", "..", "testdata", "matches", "cs2_valve_mm_8thAug23_ancient.json.gz"))
	if err != nil {
		t.Fatal(err)
	}
	rep := detect.Run(m, detect.Default())
	var party []match.PlayerID
	for _, p := range m.Rounds[0].Players {
		if p.Side == match.SideT {
			party = append(party, p.Player)
		}
	}
	in := coach.NewInput(m, rep, party)
	in.Language = "en"
	advice, err := coach.Rules{PerPlayer: 2}.Advise(t.Context(), in)
	if err != nil {
		t.Fatal(err)
	}
	var players []store.Player
	for _, p := range m.Players {
		players = append(players, store.Player{ID: p.ID, Name: p.Name, Side: m.StartSide(p.ID)})
	}
	return &fakeStore{
		m: &store.Match{
			ID: 7, Filename: "ancient.dem", Status: store.StatusCoached, Map: m.Map, Rounds: len(m.Rounds),
			Party: party, Coach: "rules", Players: players,
		},
		analysis: &store.Analysis{Match: m, Report: &rep, Advice: advice},
	}
}

func uploadRequest(t *testing.T, content []byte) *http.Request {
	t.Helper()
	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	fw, err := mw.CreateFormFile("demo", "match.dem")
	if err != nil {
		t.Fatal(err)
	}
	_, _ = fw.Write(content)
	_ = mw.Close()
	req := englishRequest(http.MethodPost, "/matches", &body)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	return req
}

func TestUpload(t *testing.T) {
	st := &fakeStore{}
	s, h := newServer(t, st)

	demo := append(append([]byte{}, demoMagic...), []byte("rest of the demo")...)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, uploadRequest(t, demo))
	if rec.Code != http.StatusSeeOther || rec.Header().Get("Location") != "/matches/1" {
		t.Fatalf("upload: %d %s", rec.Code, rec.Header().Get("Location"))
	}
	got, err := os.ReadFile(s.demoPath(1))
	if err != nil || !bytes.Equal(got, demo) {
		t.Fatalf("stored demo = %q, %v", got, err)
	}

	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, uploadRequest(t, []byte("hello")))
	if loc := rec.Header().Get("Location"); !strings.HasPrefix(loc, "/?error=") {
		t.Fatalf("non-demo upload redirected to %q", loc)
	}
	if len(st.created) != 1 {
		t.Fatalf("created %d matches, want 1", len(st.created))
	}
	entries, _ := os.ReadDir(s.DemoDir)
	if len(entries) != 1 {
		t.Fatalf("demo dir has %d files, want only the accepted demo", len(entries))
	}
}

func TestInternalDemos(t *testing.T) {
	s, h := newServer(t, &fakeStore{})
	if err := os.WriteFile(s.demoPath(3), []byte("demo"), 0o600); err != nil {
		t.Fatal(err)
	}
	do := func(method, token string) *httptest.ResponseRecorder {
		req := englishRequest(method, "/internal/demos/3", http.NoBody)
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		return rec
	}

	if rec := do(http.MethodGet, ""); rec.Code != http.StatusUnauthorized {
		t.Errorf("no token: %d", rec.Code)
	}
	if rec := do(http.MethodGet, "wrong"); rec.Code != http.StatusUnauthorized {
		t.Errorf("wrong token: %d", rec.Code)
	}
	if rec := do(http.MethodGet, "secret"); rec.Code != http.StatusOK || rec.Body.String() != "demo" {
		t.Errorf("get: %d %q", rec.Code, rec.Body.String())
	}
	if rec := do(http.MethodDelete, "secret"); rec.Code != http.StatusConflict {
		t.Errorf("delete: %d", rec.Code)
	}
	if _, err := os.Stat(s.demoPath(3)); err != nil {
		t.Errorf("legacy endpoint deleted a demo: %v", err)
	}
	if rec := do(http.MethodDelete, "secret"); rec.Code != http.StatusConflict {
		t.Errorf("deleting twice: %d", rec.Code)
	}
}

func TestMatchPage(t *testing.T) {
	st := coachedStore(t)
	_, h := newServer(t, st)

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, englishRequest(http.MethodGet, "/matches/7", http.NoBody))
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d", rec.Code)
	}
	body := rec.Body.String()
	for _, want := range []string{"de_ancient", "Recommendations", "Evidence", `href="#f`, "All findings", "Change party", `class="drill"`, "Done right:", "/practice?drill=", "<th>Where</th>", "Priority 1 of", "excess occurrences"} {
		if !strings.Contains(body, want) {
			t.Errorf("page is missing %q", want)
		}
	}
	if strings.Contains(body, `http-equiv="refresh"`) {
		t.Error("finished page refreshes itself")
	}
	// Every evidence link must point at a row of the findings table.
	for _, part := range strings.Split(body, `href="#`)[1:] {
		id, _, _ := strings.Cut(part, `"`)
		if !strings.Contains(body, `<tr id="`+id+`">`) {
			t.Errorf("evidence link #%s has no findings row", id)
		}
	}

	st.m.Status = store.StatusCoaching
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, englishRequest(http.MethodGet, "/matches/7", http.NoBody))
	if strings.Contains(rec.Body.String(), `http-equiv="refresh"`) || !strings.Contains(rec.Body.String(), "fetch(location.pathname") {
		t.Error("pending page should poll status without repeated page refreshes")
	}

	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, englishRequest(http.MethodGet, "/matches/8", http.NoBody))
	if rec.Code != http.StatusNotFound {
		t.Errorf("missing match: %d", rec.Code)
	}
}

func TestRequestCoaching(t *testing.T) {
	st := coachedStore(t)
	_, h := newServer(t, st)
	post := func(players ...string) *httptest.ResponseRecorder {
		form := url.Values{"player": players}
		req := englishRequest(http.MethodPost, "/matches/7/coach", strings.NewReader(form.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		return rec
	}

	if rec := post("2", "1", "2"); rec.Code != http.StatusSeeOther || len(st.party) != 2 || st.party[0] != 1 || st.party[1] != 2 {
		t.Fatalf("coach: %d, party %v (want deduplicated [1 2])", rec.Code, st.party)
	}
	rejected := func(name string, players ...string) {
		t.Helper()
		st.party = nil
		rec := post(players...)
		if !strings.HasSuffix(rec.Header().Get("Location"), "?notice=pick") || st.party != nil {
			t.Errorf("%s: accepted, redirect %q", name, rec.Header().Get("Location"))
		}
	}
	rejected("empty party")
	rejected("six players", "1", "2", "3", "4", "5", "6")
	rejected("player not in match", "1", "999")
	if rec := post("abc"); rec.Code != http.StatusBadRequest {
		t.Errorf("bad player id: %d", rec.Code)
	}

	st.notReady = true
	if rec := post("1"); !strings.HasSuffix(rec.Header().Get("Location"), "?notice=busy") {
		t.Errorf("coaching a busy match: %d %q", rec.Code, rec.Header().Get("Location"))
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, englishRequest(http.MethodGet, "/matches/7?notice=busy", http.NoBody))
	if !strings.Contains(rec.Body.String(), "still being processed") {
		t.Error("busy notice not shown")
	}
}

func TestIndex(t *testing.T) {
	_, h := newServer(t, coachedStore(t))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, englishRequest(http.MethodGet, "/?error=notdemo", http.NoBody))
	body, _ := io.ReadAll(rec.Body)
	for _, want := range []string{"de_ancient", "That is not a CS2 demo", `action="/matches"`} {
		if !bytes.Contains(body, []byte(want)) {
			t.Errorf("index is missing %q", want)
		}
	}

	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, englishRequest(http.MethodGet, "/?error=Your+account+is+banned", http.NoBody))
	if strings.Contains(rec.Body.String(), `class="panel error"`) {
		t.Error("index shows arbitrary error text from the URL")
	}
}

func TestPlayerHistoryPage(t *testing.T) {
	st := coachedStore(t)
	_, h := newServer(t, st)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, englishRequest(http.MethodGet, "/players/3", http.NoBody))
	body := rec.Body.String()
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d", rec.Code)
	}
	for _, want := range []string{"player03", "Progress across 1 analyzed match", "Not enough data yet", `href="/matches/7"`} {
		if !strings.Contains(body, want) {
			t.Errorf("history page is missing %q", want)
		}
	}
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, englishRequest(http.MethodGet, "/players/abc", http.NoBody))
	if rec.Code != http.StatusNotFound {
		t.Errorf("bad player id: %d", rec.Code)
	}
}

func TestOnlyTheUploaderCoaches(t *testing.T) {
	m := &store.Match{UploadedBy: 5}
	for _, tt := range []struct {
		name string
		u    *store.User
		m    *store.Match
		want bool
	}{
		{"uploader", &store.User{SteamID: 5}, m, true},
		{"someone who played in it", &store.User{SteamID: 6}, m, false},
		{"admin", &store.User{SteamID: 6, Admin: true}, m, true},
		{"legacy upload without owner", &store.User{SteamID: 5}, &store.Match{}, false},
		{"auth off", nil, m, true},
	} {
		if got := canCoach(tt.m, tt.u); got != tt.want {
			t.Errorf("%s: canCoach = %v, want %v", tt.name, got, tt.want)
		}
	}
}

func (f *fakeStore) FailMatch(context.Context, int64, string) error { return nil }

func TestTradingPartners(t *testing.T) {
	st := coachedStore(t)
	st.analysis.Report.Pairs = []detect.Pair{
		{Player: 3, Partner: 4, Chances: 5, Traded: 1},
		{Player: 4, Partner: 3, Chances: 2, Traded: 2},
		{Player: 3, Partner: 5, Chances: 1, Traded: 0}, // too few to list
	}
	_, h := newServer(t, st)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, englishRequest(http.MethodGet, "/players/3", http.NoBody))
	body := rec.Body.String()
	for _, want := range []string{"Trading with teammates", `href="/players/4"`, "20% traded", "1 of 5", "100% traded"} {
		if !strings.Contains(body, want) {
			t.Errorf("progress page is missing %q", want)
		}
	}
	if strings.Contains(body, `href="/players/5"`) {
		t.Error("a teammate with one shared death is listed")
	}
}

// Existing behavior checks use English explicitly; localization tests exercise
// the Russian default and switching separately.
func englishRequest(method, target string, body io.Reader) *http.Request {
	r := httptest.NewRequest(method, target, body)
	r.AddCookie(&http.Cookie{Name: languageCookie, Value: "en"})
	return r
}
