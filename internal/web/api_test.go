package web

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"regexp"
	"slices"
	"strconv"
	"testing"
	"time"
)

var tokenOnPage = regexp.MustCompile(`<p><code class="copy">([A-Za-z0-9_-]{43})</code></p>`)

// upload posts a demo body to the API with an upload token.
func (e *authEnv) upload(token string, body []byte) *httptest.ResponseRecorder {
	e.t.Helper()
	req := englishRequest(http.MethodPost, "/api/demos?name=match730.dem", bytes.NewReader(body))
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	rec := httptest.NewRecorder()
	e.h.ServeHTTP(rec, req)
	return rec
}

func TestUploadTokens(t *testing.T) {
	e := newAuthEnv(t)
	ctx := t.Context()
	session := cookie(e.login(admin), sessionCookie)

	page := e.do(http.MethodPost, "/tokens", session)
	m := tokenOnPage.FindStringSubmatch(page.Body.String())
	if page.Code != http.StatusOK || m == nil {
		t.Fatalf("POST /tokens = %d, token not shown:\n%s", page.Code, page.Body)
	}
	token := m[1]
	if tokenOnPage.MatchString(e.do(http.MethodGet, "/tokens", session).Body.String()) {
		t.Fatal("the token is shown again after it was created")
	}

	demo := append([]byte(nil), demoMagic...)
	demo = append(demo, "rest of the demo"...)
	first := e.upload(token, demo)
	var got UploadResponse
	if first.Code != http.StatusCreated || json.Unmarshal(first.Body.Bytes(), &got) != nil || got.Duplicate {
		t.Fatalf("upload = %d %s, want 201 with a new match", first.Code, first.Body)
	}
	again := e.upload(token, demo)
	var dup UploadResponse
	if again.Code != http.StatusOK || json.Unmarshal(again.Body.Bytes(), &dup) != nil || !dup.Duplicate || dup.ID != got.ID {
		t.Fatalf("second upload = %d %s, want 200 duplicate of match %d", again.Code, again.Body, got.ID)
	}
	if rec := e.upload(token, []byte("not a demo")); rec.Code != http.StatusUnsupportedMediaType {
		t.Fatalf("non-demo upload = %d, want 415", rec.Code)
	}
	for _, bad := range []string{"", "wrong"} {
		if rec := e.upload(bad, demo); rec.Code != http.StatusUnauthorized {
			t.Fatalf("upload with token %q = %d, want 401", bad, rec.Code)
		}
	}
	m2, err := e.st.GetMatch(ctx, got.ID)
	if err != nil || m2.UploadedBy != admin {
		t.Fatalf("match = %+v, %v; want it uploaded by the token's owner", m2, err)
	}

	// The quota applies to API uploads too (one per week in this env).
	if err := e.st.CreateInvite(ctx, admin, "invite", time.Hour); err != nil {
		t.Fatal(err)
	}
	if err := e.st.RedeemInvite(ctx, "invite", friend); err != nil {
		t.Fatal(err)
	}
	if err := e.st.CreateUploadToken(ctx, friend, hashToken("friend-token"), "pc"); err != nil {
		t.Fatal(err)
	}
	if rec := e.upload("friend-token", demo); rec.Code != http.StatusCreated {
		t.Fatalf("friend's first upload = %d %s, want 201 (a separate match from the admin's)", rec.Code, rec.Body)
	}
	more := slices.Concat(demo, []byte("!"))
	if rec := e.upload("friend-token", more); rec.Code != http.StatusTooManyRequests {
		t.Fatalf("upload over the quota = %d, want 429", rec.Code)
	}

	// Others cannot revoke a token; the owner can.
	friendTokens, err := e.st.ListUploadTokens(ctx, friend)
	if err != nil || len(friendTokens) != 1 {
		t.Fatalf("friend's tokens = %v, %v", friendTokens, err)
	}
	e.do(http.MethodPost, "/tokens/"+strconv.FormatInt(friendTokens[0].ID, 10)+"/delete", session)
	if rec := e.upload("friend-token", demo); rec.Code == http.StatusUnauthorized {
		t.Fatal("the admin revoked the friend's token")
	}
	adminTokens, err := e.st.ListUploadTokens(ctx, admin)
	if err != nil || len(adminTokens) != 1 || adminTokens[0].LastUsed.IsZero() {
		t.Fatalf("admin's tokens = %+v, %v; want one, marked as used", adminTokens, err)
	}
	e.do(http.MethodPost, "/tokens/"+strconv.FormatInt(adminTokens[0].ID, 10)+"/delete", session)
	if rec := e.upload(token, demo); rec.Code != http.StatusUnauthorized {
		t.Fatalf("upload with a revoked token = %d, want 401", rec.Code)
	}
}
