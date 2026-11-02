package web

import (
	"bytes"
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/mrolegmir/cs-demo-coach/internal/store"
)

func TestReplayURL(t *testing.T) {
	for u, want := range map[string]bool{
		"http://replay183.valve.net/730/003_1.dem.bz2": true,
		"https://replay1.valve.net/730/x.dem":          true,
		"http://valve.net.evil.com/x.dem.bz2":          false,
		"http://evilvalve.net/x.dem.bz2":               false,
		"http://replay1.valve.net:8080/x.dem.bz2":      false,
		"http://user@replay1.valve.net/x.dem.bz2":      false,
		"file:///etc/passwd":                           false,
		"":                                             false,
	} {
		if got := replayURL(u); got != want {
			t.Errorf("replayURL(%q) = %v, want %v", u, got, want)
		}
	}
}

// bot posts to a bot endpoint with the worker token.
func (e *authEnv) bot(target string, body any) *httptest.ResponseRecorder {
	e.t.Helper()
	b, _ := json.Marshal(body)
	req := englishRequest(http.MethodPost, target, bytes.NewReader(b))
	req.Header.Set("Authorization", "Bearer secret")
	rec := httptest.NewRecorder()
	e.h.ServeHTTP(rec, req)
	return rec
}

func TestShareCodeImport(t *testing.T) {
	e := newAuthEnv(t)
	ctx := t.Context()
	demo, err := os.ReadFile("testdata/demo.dem.bz2")
	if err != nil {
		t.Fatal(err)
	}
	// Every request goes to this server, whatever host the URL names.
	valve := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/730/1.dem.bz2":
			_, _ = w.Write(demo)
		case "/730/redirect.dem.bz2":
			http.Redirect(w, r, "http://169.254.169.254/latest/meta-data", http.StatusFound)
		default:
			http.NotFound(w, r)
		}
	}))
	defer valve.Close()
	e.srv.ImportHTTP = &http.Client{Transport: &http.Transport{
		DialContext: func(ctx context.Context, network, _ string) (net.Conn, error) {
			return (&net.Dialer{}).DialContext(ctx, network, valve.Listener.Addr().String())
		},
	}}

	if err := e.st.SetSteamLink(ctx, admin, "AAAA-BBBBB-CCCC", "CSGO-GADqf-jjyJ8-cSP2r-smZRo-TO2xK"); err != nil {
		t.Fatal(err)
	}
	if err := e.st.SetSteamLink(ctx, admin, "AAAA-BBBBB-CCCC", "CSGO-AAAAA-AAAAA-AAAAA-AAAAA-AAAAB"); err != nil {
		t.Fatal(err)
	}
	if err := e.st.SetSteamLink(ctx, admin, "AAAA-BBBBB-CCCC", "CSGO-AAAAA-AAAAA-AAAAA-AAAAA-AAAAC"); err != nil {
		t.Fatal(err)
	}

	claim := func() CodeClaim {
		t.Helper()
		rec := e.bot("/internal/sharecodes/claim", struct{}{})
		var c CodeClaim
		if rec.Code != http.StatusOK || json.Unmarshal(rec.Body.Bytes(), &c) != nil {
			t.Fatalf("claim = %d %s", rec.Code, rec.Body)
		}
		return c
	}
	first, second, third := claim(), claim(), claim()
	if rec := e.bot("/internal/sharecodes/claim", struct{}{}); rec.Code != http.StatusNoContent {
		t.Fatalf("claim with nothing waiting = %d, want 204", rec.Code)
	}
	if rec := e.do(http.MethodPost, "/internal/sharecodes/claim"); rec.Code != http.StatusUnauthorized {
		t.Fatalf("claim without the token = %d, want 401", rec.Code)
	}

	path := func(c CodeClaim, action string) string {
		return "/internal/sharecodes/" + strconv.FormatInt(c.ID, 10) + "/" + action
	}
	if rec := e.bot(path(first, "resolve"), CodeResolved{DemoURL: "http://169.254.169.254/latest"}); rec.Code != http.StatusBadRequest {
		t.Fatalf("resolve to a non-Valve URL = %d, want 400", rec.Code)
	}
	played := time.Date(2026, 10, 28, 20, 0, 0, 0, time.UTC)
	if rec := e.bot(path(first, "resolve"), CodeResolved{DemoURL: "http://replay1.valve.net/730/1.dem.bz2", Played: played.Unix()}); rec.Code != http.StatusNoContent {
		t.Fatalf("resolve = %d %s", rec.Code, rec.Body)
	}
	if rec := e.bot(path(second, "resolve"), CodeResolved{DemoURL: "http://replay1.valve.net/730/gone.dem.bz2"}); rec.Code != http.StatusNoContent {
		t.Fatalf("resolve = %d %s", rec.Code, rec.Body)
	}

	if rec := e.bot(path(third, "resolve"), CodeResolved{DemoURL: "http://replay1.valve.net/730/redirect.dem.bz2"}); rec.Code != http.StatusNoContent {
		t.Fatalf("resolve = %d %s", rec.Code, rec.Body)
	}
	// A late bot report for a code that moved on is refused.
	if rec := e.bot(path(first, "fail"), CodeFailed{Expired: true}); rec.Code != http.StatusNotFound {
		t.Fatalf("late fail = %d, want 404", rec.Code)
	}

	e.srv.importDue(ctx)

	codes, err := e.st.ListShareCodes(ctx, admin, 10)
	if err != nil || len(codes) != 3 {
		t.Fatalf("codes = %+v, %v", codes, err)
	}
	byCode := map[string]store.ShareCode{}
	for _, c := range codes {
		byCode[c.Code] = c
	}
	got := byCode[first.Code]
	if got.Status != store.CodeImported || got.MatchID == 0 {
		t.Fatalf("first code = %+v, want imported", got)
	}
	m, err := e.st.GetMatch(ctx, got.MatchID)
	if err != nil || m.UploadedBy != admin {
		t.Fatalf("match = %+v, %v; want uploaded by the code's owner", m, err)
	}
	if body, err := os.ReadFile(e.srv.demoPath(got.MatchID)); err != nil || !bytes.HasPrefix(body, demoMagic) {
		t.Fatalf("saved demo is not the unpacked file: %v", err)
	}
	if gone := byCode[second.Code]; gone.Status != store.CodeExpired {
		t.Errorf("second code = %+v, want expired after a 404", gone)
	}
	if redirected := byCode[third.Code]; redirected.Status != store.CodeFailed {
		t.Errorf("third code = %+v, want failed: the redirect leaves Valve's servers", redirected)
	}
}

func TestShareCodeBotFailures(t *testing.T) {
	e := newAuthEnv(t)
	ctx := t.Context()
	if err := e.st.SetSteamLink(ctx, admin, "AAAA-BBBBB-CCCC", "CSGO-GADqf-jjyJ8-cSP2r-smZRo-TO2xK"); err != nil {
		t.Fatal(err)
	}
	rec := e.bot("/internal/sharecodes/claim", struct{}{})
	var c CodeClaim
	if err := json.Unmarshal(rec.Body.Bytes(), &c); err != nil {
		t.Fatal(err)
	}
	fail := "/internal/sharecodes/" + strconv.FormatInt(c.ID, 10) + "/fail"
	if rec := e.bot(fail, CodeFailed{Reason: "GC timeout"}); rec.Code != http.StatusNoContent {
		t.Fatalf("fail = %d", rec.Code)
	}
	// A transient failure waits before the code is handed out again.
	if rec := e.bot("/internal/sharecodes/claim", struct{}{}); rec.Code != http.StatusNoContent {
		t.Fatalf("claim right after a failure = %d, want 204", rec.Code)
	}
	// The bot no longer holds it, so it cannot expire it either.
	if rec := e.bot(fail, CodeFailed{Expired: true}); rec.Code != http.StatusNotFound {
		t.Fatalf("expire a code the bot does not hold = %d, want 404", rec.Code)
	}
	if _, err := e.st.ClaimShareCode(ctx, store.CodeWaiting, store.CodeResolving, 0); err != nil {
		t.Fatal(err)
	}
	if rec := e.bot(fail, CodeFailed{Expired: true}); rec.Code != http.StatusNoContent {
		t.Fatalf("expire = %d", rec.Code)
	}
	codes, _ := e.st.ListShareCodes(ctx, admin, 10)
	if len(codes) != 1 || codes[0].Status != store.CodeExpired || codes[0].Reason == "" {
		t.Fatalf("codes = %+v, want one expired code", codes)
	}
}
