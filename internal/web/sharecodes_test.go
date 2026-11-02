package web

import (
	"net/http"
	"net/url"
	"strings"
	"testing"
)

func TestSteamLinkForm(t *testing.T) {
	e := newAuthEnv(t)
	ctx := t.Context()
	session := cookie(e.login(admin), sessionCookie)
	const share = "CSGO-GADqf-jjyJ8-cSP2r-smZRo-TO2xK"

	for _, tt := range []struct {
		auth, share, want string
	}{
		{"abcd", share, "/tokens?link=authcode"},
		{"ABCD-EFGHJ-KLMN", "CSGO-nope", "/tokens?link=sharecode"},
		{" wxyz-23456-qrst ", strings.ReplaceAll(share, "-", ""), "/tokens"},
	} {
		rec := e.form("/steam-link", url.Values{"auth_code": {tt.auth}, "share_code": {tt.share}}, session)
		if rec.Code != http.StatusSeeOther || rec.Header().Get("Location") != tt.want {
			t.Errorf("save %q %q = %d to %q, want redirect to %q", tt.auth, tt.share, rec.Code, rec.Header().Get("Location"), tt.want)
		}
	}
	link, err := e.st.GetSteamLink(ctx, admin)
	if err != nil || link.AuthCode != "WXYZ-23456-QRST" || link.LastCode != share {
		t.Fatalf("link = %+v, %v; want the normalized codes", link, err)
	}

	page := e.do(http.MethodGet, "/tokens", session).Body.String()
	if strings.Contains(page, "WXYZ-23456") || !strings.Contains(page, "-QRST") {
		t.Error("the page must show only the last group of the auth code")
	}
	if !strings.Contains(page, "Not checked yet") {
		t.Error("a new link should say it was not checked yet")
	}

	if rec := e.form("/steam-link/delete", nil, session); rec.Code != http.StatusSeeOther {
		t.Fatalf("delete = %d", rec.Code)
	}
	if _, err := e.st.GetSteamLink(ctx, admin); err == nil {
		t.Error("link still there after delete")
	}
}
