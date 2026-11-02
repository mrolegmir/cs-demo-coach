package web

import (
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/mrolegmir/cs-demo-coach/internal/telegram"
)

func TestConnectTelegram(t *testing.T) {
	e := newAuthEnv(t)
	ctx := t.Context()
	session := cookie(e.login(admin), sessionCookie)

	if page := e.do(http.MethodGet, "/", session).Body.String(); !strings.Contains(page, "Connect Telegram") {
		t.Fatal("the home page should offer to connect Telegram")
	}
	rec := e.form("/telegram", nil, session)
	to, err := url.Parse(rec.Header().Get("Location"))
	if rec.Code != http.StatusSeeOther || err != nil || to.Host != "t.me" || to.Path != "/CsdcBot" {
		t.Fatalf("connect = %d to %q", rec.Code, rec.Header().Get("Location"))
	}
	// The bot gets the start parameter back with the chat.
	code := to.Query().Get("start")
	if id, err := e.st.UseTelegramCode(ctx, telegram.HashCode(code), 42); err != nil || id != admin {
		t.Fatalf("use code = %d, %v", id, err)
	}
	if page := e.do(http.MethodGet, "/", session).Body.String(); !strings.Contains(page, "Stop sending to Telegram") {
		t.Fatal("the home page should show the linked chat")
	}

	if rec := e.form("/telegram/delete", nil, session); rec.Code != http.StatusSeeOther {
		t.Fatalf("disconnect = %d", rec.Code)
	}
	if _, err := e.st.GetTelegramLink(ctx, admin); err == nil {
		t.Error("still linked after disconnect")
	}
}
