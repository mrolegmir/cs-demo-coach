package web

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/mrolegmir/cs-demo-coach/internal/auth"
	"github.com/mrolegmir/cs-demo-coach/internal/coach"
	"github.com/mrolegmir/cs-demo-coach/internal/detect"
	"github.com/mrolegmir/cs-demo-coach/internal/match"
	"github.com/mrolegmir/cs-demo-coach/internal/store"
	"github.com/mrolegmir/cs-demo-coach/internal/store/storetest"
)

const publicURL = "https://csdc.example.com"

func TestManualBriefSnapshotsBrowserLanguage(t *testing.T) {
	e := newAuthEnv(t)
	session := cookie(e.login(admin), sessionCookie)
	id, err := e.st.CreateMatch(t.Context(), store.Upload{Filename: "language.dem", By: admin})
	if err != nil {
		t.Fatal(err)
	}
	m := &match.Match{Players: []match.Player{{ID: admin, Name: "player01"}}}
	if err := e.st.SaveDetection(t.Context(), id, m, detect.Report{}); err != nil {
		t.Fatal(err)
	}
	for _, language := range []string{"", "en"} {
		r := httptest.NewRequest(http.MethodPost, "/briefs", http.NoBody)
		r.Header.Set("Origin", publicURL)
		r.AddCookie(session)
		if language != "" {
			r.AddCookie(&http.Cookie{Name: languageCookie, Value: language})
		}
		w := httptest.NewRecorder()
		e.h.ServeHTTP(w, r)
		location := strings.TrimPrefix(w.Header().Get("Location"), "/briefs/")
		briefID, err := strconv.ParseInt(location, 10, 64)
		if err != nil || w.Code != http.StatusSeeOther {
			t.Fatalf("brief request: %d %s", w.Code, w.Header().Get("Location"))
		}
		in, err := e.st.GetBriefInput(t.Context(), briefID)
		if err != nil || in.Language != coach.Language(language) {
			t.Fatalf("stored input: %+v, %v", in, err)
		}
	}
}

const (
	admin  match.PlayerID = 76561198000000001
	friend match.PlayerID = 76561198000000002
	other  match.PlayerID = 76561198000000003
)

// authEnv is a server with Steam sign-in on a real store and a fake Steam
// that confirms every login.
type authEnv struct {
	t     *testing.T
	st    *store.Store
	srv   *Server
	h     http.Handler
	steam string
}

func newAuthEnv(t *testing.T) *authEnv {
	t.Helper()
	st, err := store.Open(t.Context(), storetest.URL(t), store.Limits{Uploads: 1, Coaching: 1})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(st.Close)
	steam := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, "is_valid:true\n")
	}))
	t.Cleanup(steam.Close)

	s := &Server{
		Store: st, DemoDir: t.TempDir(), WorkerToken: "secret", Log: slog.New(slog.DiscardHandler), ShareCodes: true, TelegramBot: func() string { return "CsdcBot" },
		Auth: &Auth{Steam: auth.Steam{Endpoint: steam.URL}, PublicURL: publicURL, UploadsPerWeek: 1, CoachingPerWeek: 1},
	}
	h, err := s.Handler()
	if err != nil {
		t.Fatal(err)
	}
	if err := st.EnsureAdmin(t.Context(), admin); err != nil {
		t.Fatal(err)
	}
	return &authEnv{t: t, st: st, srv: s, h: h, steam: steam.URL}
}

// do sends a request with the given cookies; POSTs come from our origin.
func (e *authEnv) do(method, target string, cookies ...*http.Cookie) *httptest.ResponseRecorder {
	e.t.Helper()
	req := englishRequest(method, target, http.NoBody)
	if method == http.MethodPost {
		req.Header.Set("Origin", publicURL)
	}
	for _, c := range cookies {
		req.AddCookie(c)
	}
	rec := httptest.NewRecorder()
	e.h.ServeHTTP(rec, req)
	return rec
}

// login completes a Steam sign-in as id, starting it in the same browser,
// and returns the callback response.
func (e *authEnv) login(id match.PlayerID, cookies ...*http.Cookie) *httptest.ResponseRecorder {
	e.t.Helper()
	start := e.do(http.MethodGet, "/auth/steam", cookies...)
	state := cookie(start, loginCookie)
	if state == nil {
		e.t.Fatal("login did not set a state cookie")
	}
	return e.callback(id, state.Value, append(cookies, state)...)
}

// callback sends Steam's redirect back for id with the given state.
func (e *authEnv) callback(id match.PlayerID, state string, cookies ...*http.Cookie) *httptest.ResponseRecorder {
	e.t.Helper()
	claimed := "https://steamcommunity.com/openid/id/" + strconv.FormatUint(uint64(id), 10)
	returnTo := publicURL + "/auth/steam/callback?state=" + url.QueryEscape(state)
	q := url.Values{
		"openid.mode":           {"id_res"},
		"openid.op_endpoint":    {e.steam},
		"openid.claimed_id":     {claimed},
		"openid.identity":       {claimed},
		"openid.return_to":      {returnTo},
		"openid.response_nonce": {time.Now().UTC().Format(time.RFC3339) + "n"},
		"openid.sig":            {"sig"},
	}
	q.Set("state", state)
	return e.do(http.MethodGet, "/auth/steam/callback?"+q.Encode(), cookies...)
}

func cookie(rec *httptest.ResponseRecorder, name string) *http.Cookie {
	for _, c := range rec.Result().Cookies() {
		if c.Name == name && c.Value != "" {
			return c
		}
	}
	return nil
}

func TestSignInWithInvite(t *testing.T) {
	e := newAuthEnv(t)

	if rec := e.do(http.MethodGet, "/"); rec.Header().Get("Location") != "/login" {
		t.Fatalf("anonymous visit: %d %q", rec.Code, rec.Header().Get("Location"))
	}

	// The admin is let in by configuration.
	rec := e.login(admin)
	sess := cookie(rec, sessionCookie)
	if rec.Header().Get("Location") != "/" || sess == nil || !sess.HttpOnly || !sess.Secure || sess.SameSite != http.SameSiteLaxMode {
		t.Fatalf("admin login: %q, cookie %+v", rec.Header().Get("Location"), sess)
	}
	if rec := e.do(http.MethodGet, "/", sess); rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "Invite friends") {
		t.Fatalf("signed-in index: %d", rec.Code)
	}

	// Strangers need an invite.
	if rec := e.login(friend); rec.Header().Get("Location") != "/login?m=invite" || cookie(rec, sessionCookie) != nil {
		t.Fatalf("uninvited login: %q", rec.Header().Get("Location"))
	}

	if rec := e.do(http.MethodPost, "/invites", sess); rec.Code != http.StatusSeeOther {
		t.Fatalf("create invite: %d", rec.Code)
	}
	invites, err := e.st.ListInvites(t.Context(), admin)
	if err != nil || len(invites) != 1 {
		t.Fatalf("invites = %v, %v", invites, err)
	}
	if !strings.Contains(e.do(http.MethodGet, "/invites", sess).Body.String(), publicURL+"/invite/"+invites[0].Code) {
		t.Error("invites page does not show the link")
	}

	rec = e.do(http.MethodGet, "/invite/"+invites[0].Code)
	inv := cookie(rec, inviteCookie)
	if rec.Header().Get("Location") != "/auth/steam" || inv == nil {
		t.Fatalf("invite link: %q", rec.Header().Get("Location"))
	}
	rec = e.login(friend, inv)
	friendSess := cookie(rec, sessionCookie)
	if rec.Header().Get("Location") != "/" || friendSess == nil {
		t.Fatalf("invited login: %q", rec.Header().Get("Location"))
	}

	// An invite works once.
	if rec := e.login(other, inv); rec.Header().Get("Location") != "/login?m=invite" {
		t.Fatalf("reused invite: %q", rec.Header().Get("Location"))
	}

	// Signing out ends the session.
	e.do(http.MethodPost, "/logout", friendSess)
	if rec := e.do(http.MethodGet, "/", friendSess); rec.Header().Get("Location") != "/login" {
		t.Fatalf("after logout: %d %q", rec.Code, rec.Header().Get("Location"))
	}
}

func TestCallbackIsBoundToTheBrowser(t *testing.T) {
	e := newAuthEnv(t)
	start := e.do(http.MethodGet, "/auth/steam")
	state := cookie(start, loginCookie)
	// A callback URL replayed from another browser (no state cookie), or
	// with another login's state, does not sign anyone in.
	for name, rec := range map[string]*httptest.ResponseRecorder{
		"no cookie":   e.callback(admin, state.Value),
		"wrong state": e.callback(admin, "other", state),
	} {
		if rec.Header().Get("Location") != "/login?m=failed" || cookie(rec, sessionCookie) != nil {
			t.Errorf("%s: signed in (%q)", name, rec.Header().Get("Location"))
		}
	}
	if rec := e.callback(admin, state.Value, state); cookie(rec, sessionCookie) == nil {
		t.Fatal("the browser that started the login could not finish it")
	}
}

func TestCrossSitePostIsRejected(t *testing.T) {
	e := newAuthEnv(t)
	sess := cookie(e.login(admin), sessionCookie)
	req := englishRequest(http.MethodPost, "/invites", http.NoBody)
	req.Header.Set("Origin", "https://evil.example")
	req.AddCookie(sess)
	rec := httptest.NewRecorder()
	e.h.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("cross-site POST: %d", rec.Code)
	}
}

func TestMatchVisibilityAndQuota(t *testing.T) {
	e := newAuthEnv(t)
	ctx := t.Context()
	adminSess := cookie(e.login(admin), sessionCookie)
	code := "invite-code"
	if err := e.st.CreateInvite(ctx, admin, code, inviteTTL); err != nil {
		t.Fatal(err)
	}
	friendSess := cookie(e.login(friend, &http.Cookie{Name: inviteCookie, Value: code}), sessionCookie)
	if friendSess == nil {
		t.Fatal("friend could not sign in")
	}

	id, err := e.st.CreateMatch(ctx, store.Upload{Filename: "admins.dem", By: admin})
	if err != nil {
		t.Fatal(err)
	}
	target := "/matches/" + strconv.FormatInt(id, 10)
	if rec := e.do(http.MethodGet, target, friendSess); rec.Code != http.StatusNotFound {
		t.Errorf("friend sees a match they did not upload or play in: %d", rec.Code)
	}
	if rec := e.do(http.MethodGet, target+"/status", friendSess); rec.Code != http.StatusNotFound {
		t.Errorf("status endpoint exposes another user's match: %d", rec.Code)
	}
	if rec := e.do(http.MethodGet, target+"/status", adminSess); rec.Code != http.StatusOK || rec.Header().Get("Cache-Control") != "no-store" || !strings.Contains(rec.Body.String(), `"status":"uploaded"`) {
		t.Errorf("uploader cannot poll match status: %d %s", rec.Code, rec.Body.String())
	}
	if rec := e.do(http.MethodGet, target, adminSess); rec.Code != http.StatusOK {
		t.Errorf("uploader cannot see their match: %d", rec.Code)
	}
	if strings.Contains(e.do(http.MethodGet, "/", friendSess).Body.String(), "admins.dem") {
		t.Error("friend's match list shows someone else's upload")
	}

	// The friend has one upload per week; after using it, uploads are refused.
	if _, err := e.st.CreateMatch(ctx, store.Upload{Filename: "friends.dem", By: friend}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(e.do(http.MethodGet, "/", friendSess).Body.String(), "0 uploads") {
		t.Error("index does not show the spent quota")
	}
	if rec := e.do(http.MethodPost, "/matches", friendSess); rec.Header().Get("Location") != "/?error=quota" {
		t.Errorf("upload over quota: %d %q", rec.Code, rec.Header().Get("Location"))
	}
	// Admins have no quota.
	if rec := e.do(http.MethodPost, "/matches", adminSess); rec.Header().Get("Location") == "/?error=quota" {
		t.Error("admin hit the quota")
	}
}

func TestBriefPages(t *testing.T) {
	e := newAuthEnv(t)
	ctx := t.Context()
	adminSess := cookie(e.login(admin), sessionCookie)
	if err := e.st.CreateInvite(ctx, admin, "code", inviteTTL); err != nil {
		t.Fatal(err)
	}
	friendSess := cookie(e.login(friend, &http.Cookie{Name: inviteCookie, Value: "code"}), sessionCookie)

	// Without analyzed matches there is nothing to brief.
	if rec := e.do(http.MethodPost, "/briefs", friendSess); rec.Header().Get("Location") != "/?brief=none" {
		t.Fatalf("brief without matches: %q", rec.Header().Get("Location"))
	}

	in := &coach.Input{
		Period: "Oct 23 – Oct 29", Matches: 1, Party: []string{"admin"},
		Evidence: map[string]coach.Evidence{"m1-f1": {Kind: "no_utility", Player: "admin", Match: 1, Round: 3, Detail: "threw nothing"}},
	}
	now := time.Now()
	id, err := e.st.CreateBrief(ctx, &store.Brief{SteamID: admin, PeriodStart: now.Add(-7 * 24 * time.Hour), PeriodEnd: now, Party: []match.PlayerID{admin}, Input: in})
	if err != nil {
		t.Fatal(err)
	}
	target := "/briefs/" + strconv.FormatInt(id, 10)
	if body := e.do(http.MethodGet, target, adminSess).Body.String(); !strings.Contains(body, `http-equiv="refresh"`) {
		t.Error("queued brief does not refresh")
	}
	advice := &coach.Advice{Players: []coach.PlayerAdvice{{Player: "admin", Focus: []coach.Focus{{
		Kind: "no_utility", Title: "Throw your utility", Why: "w", Drill: "d", Evidence: []string{"m1-f1"},
	}}}}}
	if err := e.st.SaveBriefAdvice(ctx, id, advice, "rules"); err != nil {
		t.Fatal(err)
	}
	body := e.do(http.MethodGet, target, adminSess).Body.String()
	for _, want := range []string{"Throw your utility", `href="/matches/1#f1"`, now.Add(-7 * 24 * time.Hour).Format("02.01.2006")} {
		if !strings.Contains(body, want) {
			t.Errorf("brief page is missing %q", want)
		}
	}
	if !strings.Contains(e.do(http.MethodGet, "/", adminSess).Body.String(), target) {
		t.Error("index does not link the brief")
	}
	if rec := e.do(http.MethodGet, target, friendSess); rec.Code != http.StatusNotFound {
		t.Errorf("someone else's brief: %d", rec.Code)
	}
}

// form posts a form from our origin.
func (e *authEnv) form(target string, values url.Values, cookies ...*http.Cookie) *httptest.ResponseRecorder {
	e.t.Helper()
	req := englishRequest(http.MethodPost, target, strings.NewReader(values.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Origin", publicURL)
	for _, c := range cookies {
		req.AddCookie(c)
	}
	rec := httptest.NewRecorder()
	e.h.ServeHTTP(rec, req)
	return rec
}

func TestPracticeLog(t *testing.T) {
	e := newAuthEnv(t)
	ctx := t.Context()
	adminSess := cookie(e.login(admin), sessionCookie)
	if err := e.st.CreateInvite(ctx, admin, "code", inviteTTL); err != nil {
		t.Fatal(err)
	}
	friendSess := cookie(e.login(friend, &http.Cookie{Name: inviteCookie, Value: "code"}), sessionCookie)
	friendPage := "/players/" + strconv.FormatUint(uint64(friend), 10)

	today := time.Now().Format(time.DateOnly)
	rec := e.form("/practice", url.Values{"kind": {"no_utility"}, "on": {today}, "minutes": {"25"}, "note": {"Mirage smokes"}}, friendSess)
	if loc := rec.Header().Get("Location"); loc != friendPage+"#practice" {
		t.Fatalf("add practice redirected to %q", loc)
	}
	for _, bad := range []url.Values{
		{"on": {today}, "note": {" "}},
		{"on": {"2001-01-01"}, "note": {"x"}},
		{"on": {time.Now().AddDate(0, 0, 3).Format(time.DateOnly)}, "note": {"x"}},
	} {
		if loc := e.form("/practice", bad, friendSess).Header().Get("Location"); !strings.Contains(loc, "?error=") {
			t.Errorf("practice %v was accepted (%q)", bad, loc)
		}
	}

	log, err := e.st.ListPractice(ctx, friend)
	if err != nil || len(log) != 1 || log[0].Minutes != 25 || log[0].Kind != "no_utility" {
		t.Fatalf("practice log = %+v, %v", log, err)
	}
	if body := e.do(http.MethodGet, friendPage, friendSess).Body.String(); !strings.Contains(body, "Mirage smokes") {
		t.Error("own progress page does not show the practice log")
	}
	if body := e.do(http.MethodGet, friendPage, adminSess).Body.String(); strings.Contains(body, "Mirage smokes") {
		t.Error("someone else sees the practice notes")
	}

	del := "/practice/" + strconv.FormatInt(log[0].ID, 10) + "/delete"
	e.do(http.MethodPost, del, adminSess)
	if log, _ := e.st.ListPractice(ctx, friend); len(log) != 1 {
		t.Fatal("someone else deleted a practice entry")
	}
	e.do(http.MethodPost, del, friendSess)
	if log, _ := e.st.ListPractice(ctx, friend); len(log) != 0 {
		t.Fatal("the owner could not delete a practice entry")
	}
}

func TestPracticeADrill(t *testing.T) {
	e := newAuthEnv(t)
	ctx := t.Context()
	sess := cookie(e.login(admin), sessionCookie)
	page := "/players/" + strconv.FormatUint(uint64(admin), 10)

	// A drill card links to the form with the drill picked.
	rec := e.do(http.MethodGet, "/practice?drill=three-smokes", sess)
	if loc := rec.Header().Get("Location"); loc != page+"?drill=three-smokes#practice" {
		t.Fatalf("drill link went to %q", loc)
	}
	if body := e.do(http.MethodGet, page+"?drill=three-smokes", sess).Body.String(); !strings.Contains(body, `value="three-smokes" selected`) {
		t.Error("the form does not preselect the drill")
	}

	// The drill sets the mistake and fills in an empty note and minutes,
	// whatever the mistake field says.
	today := time.Now().Format(time.DateOnly)
	e.form("/practice", url.Values{"drill": {"three-smokes"}, "kind": {"team_flash"}, "on": {today}}, sess)
	log, err := e.st.ListPractice(ctx, admin)
	if err != nil || len(log) != 1 {
		t.Fatalf("log = %+v, %v", log, err)
	}
	if p := log[0]; p.Drill != "three-smokes" || p.Kind != "no_utility" || p.Note != "Learn three smokes for your map" || p.Minutes != 20 {
		t.Errorf("entry = %+v", p)
	}
	if body := e.do(http.MethodGet, page, sess).Body.String(); !strings.Contains(body, "Learn three smokes for your map") {
		t.Error("the log does not show the drill")
	}
}
