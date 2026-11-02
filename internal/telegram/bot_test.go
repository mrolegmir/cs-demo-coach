package telegram

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
	"unicode/utf16"

	"github.com/mrolegmir/cs-demo-coach/internal/coach"
	"github.com/mrolegmir/cs-demo-coach/internal/match"
	"github.com/mrolegmir/cs-demo-coach/internal/store"
	"github.com/mrolegmir/cs-demo-coach/internal/store/storetest"
)

const token = "123:secret"

// fakeAPI is a Bot API that records sent messages and answers getMe;
// sending to a chat in refuse gets that status and description.
type fakeAPI struct {
	mu     sync.Mutex
	sent   []sent
	refuse map[int64]refusal
}

type refusal struct {
	status int
	desc   string
}

type sent struct {
	Chat int64  `json:"chat_id"`
	Text string `json:"text"`
}

func newFakeAPI(t *testing.T) (*fakeAPI, *Client) {
	f := &fakeAPI{refuse: map[int64]refusal{}}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		method, ok := strings.CutPrefix(r.URL.Path, "/bot"+token+"/")
		if !ok {
			w.WriteHeader(http.StatusNotFound)
			_, _ = io.WriteString(w, `{"ok":false,"description":"Not Found"}`)
			return
		}
		switch method {
		case "getMe":
			_, _ = io.WriteString(w, `{"ok":true,"result":{"id":1,"is_bot":true,"username":"CsdcBot"}}`)
		case "sendMessage":
			var m sent
			_ = json.NewDecoder(r.Body).Decode(&m)
			f.mu.Lock()
			defer f.mu.Unlock()
			if r, ok := f.refuse[m.Chat]; ok {
				w.WriteHeader(r.status)
				_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "error_code": r.status, "description": r.desc})
				return
			}
			f.sent = append(f.sent, m)
			_, _ = io.WriteString(w, `{"ok":true,"result":{"message_id":1}}`)
		default:
			_, _ = io.WriteString(w, `{"ok":false,"description":"unexpected method"}`)
		}
	}))
	t.Cleanup(srv.Close)
	return f, &Client{Token: token, Base: srv.URL}
}

func (f *fakeAPI) take() []sent {
	f.mu.Lock()
	defer f.mu.Unlock()
	s := f.sent
	f.sent = nil
	return s
}

func TestClient(t *testing.T) {
	_, c := newFakeAPI(t)
	if name, err := c.Username(t.Context()); err != nil || name != "CsdcBot" {
		t.Fatalf("username = %q, %v", name, err)
	}
	bad := &Client{Token: "wrong", Base: c.Base}
	_, err := bad.Username(t.Context())
	if err == nil || strings.Contains(err.Error(), "wrong") {
		t.Errorf("bad token: err = %v; want an error without the token", err)
	}
	down := &Client{Token: token, Base: "http://127.0.0.1:1"}
	if _, err := down.Username(t.Context()); err == nil || strings.Contains(err.Error(), token) {
		t.Errorf("unreachable: err = %v; want an error without the token", err)
	}
}

func TestText(t *testing.T) {
	br := &store.Brief{
		ID:          12,
		PeriodStart: time.Date(2026, 10, 23, 0, 0, 0, 0, time.UTC),
		PeriodEnd:   time.Date(2026, 10, 30, 0, 0, 0, 0, time.UTC),
		Advice: &coach.Advice{
			Players: []coach.PlayerAdvice{
				{Player: "anna", Focus: []coach.Focus{{Title: "Throw utility on full buys"}, {Title: "Trade the entry"}}},
				{Player: "bob"},
			},
			Team: []coach.TeamAdvice{{Title: "Hit B together"}},
		},
	}
	want := "Your weekly brief for Oct 23 – Oct 30 is ready.\n" +
		"\nanna\n• Throw utility on full buys\n• Trade the entry\n" +
		"\nTeam\n• Hit B together\n" +
		"\nWhy, with the rounds to watch, and the drills: https://csdc.example.com/briefs/12"
	if got := Text(br, "https://csdc.example.com"); got != want {
		t.Errorf("Text =\n%s\nwant\n%s", got, want)
	}
}

func TestTruncate(t *testing.T) {
	if got := truncate("abc", 3); got != "abc" {
		t.Errorf("a string that fits was cut: %q", got)
	}
	// Each emoji is two UTF-16 units, which is how Telegram counts.
	got := truncate(strings.Repeat("\U0001F525", 10), 10)
	if n := len(utf16.Encode([]rune(got))); n > 10 || !strings.HasSuffix(got, "\u2026\n") {
		t.Errorf("truncate = %q (%d units)", got, n)
	}
}

func TestRun(t *testing.T) {
	b, _, _ := newBot(t)
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan struct{})
	go func() { b.Run(ctx); close(done) }()
	for b.Name() == "" {
		time.Sleep(time.Millisecond)
	}
	if b.Name() != "CsdcBot" {
		t.Errorf("name = %q", b.Name())
	}
	cancel()
	<-done
}

const (
	owner  match.PlayerID = 76561198000000001
	friend match.PlayerID = 76561198000000002
)

func newBot(t *testing.T) (*Bot, *store.Store, *fakeAPI) {
	st, err := store.Open(t.Context(), storetest.URL(t), store.Limits{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(st.Close)
	for _, id := range []match.PlayerID{owner, friend} {
		if err := st.EnsureAdmin(t.Context(), id); err != nil {
			t.Fatal(err)
		}
	}
	f, c := newFakeAPI(t)
	return &Bot{API: c, Store: st, PublicURL: "https://csdc.example.com", Log: slog.New(slog.DiscardHandler)}, st, f
}

func private(chat int64, text string) *Message {
	return &Message{Chat: Chat{ID: chat, Type: "private"}, Text: text}
}

func TestLinking(t *testing.T) {
	b, st, f := newBot(t)
	ctx := t.Context()
	code, hash := NewCode()
	if err := st.CreateTelegramCode(ctx, owner, hash, time.Now().Add(CodeTTL)); err != nil {
		t.Fatal(err)
	}

	b.handle(ctx, &Message{Chat: Chat{ID: -5, Type: "group"}, Text: ""}) // "bot was added"
	b.handle(ctx, &Message{Chat: Chat{ID: -5, Type: "group"}, Text: "gg"})
	b.handle(ctx, &Message{Chat: Chat{ID: -5, Type: "group"}, Text: "/start " + code})
	b.handle(ctx, private(42, "/start"))
	b.handle(ctx, private(42, "/start "+code))
	b.handle(ctx, private(43, "/start "+code)) // spent
	if got := f.take(); len(got) != 4 || got[0].Text != replyPrivate || got[1].Text != replyHelp ||
		got[2].Text != replyLinked || got[3].Text != replyExpired {
		t.Fatalf("replies = %+v", got)
	}
	l, err := st.GetTelegramLink(ctx, owner)
	if err != nil || l.ChatID != 42 {
		t.Fatalf("link = %+v, %v", l, err)
	}

	// The same chat linked by another account moves there.
	code, hash = NewCode()
	if err := st.CreateTelegramCode(ctx, friend, hash, time.Now().Add(CodeTTL)); err != nil {
		t.Fatal(err)
	}
	b.handle(ctx, private(42, "/start "+code))
	if _, err := st.GetTelegramLink(ctx, owner); !errors.Is(err, store.ErrNotFound) {
		t.Errorf("owner still linked: %v", err)
	}

	// Expired codes do not link.
	code, hash = NewCode()
	if err := st.CreateTelegramCode(ctx, owner, hash, time.Now().Add(-time.Second)); err != nil {
		t.Fatal(err)
	}
	b.handle(ctx, private(44, "/start "+code))

	b.handle(ctx, private(42, "/stop@CsdcBot"))
	b.handle(ctx, private(42, "/stop"))
	if got := f.take(); len(got) != 4 || got[1].Text != replyExpired || got[2].Text != replyStopped || got[3].Text != replyNotLinked {
		t.Fatalf("replies = %+v", got)
	}
	if _, err := st.GetTelegramLink(ctx, friend); !errors.Is(err, store.ErrNotFound) {
		t.Errorf("friend still linked after /stop: %v", err)
	}
}

// readyBrief stores a ready brief for id.
func readyBrief(t *testing.T, st *store.Store, id match.PlayerID) int64 {
	t.Helper()
	now := time.Now()
	bid, err := st.CreateBrief(t.Context(), &store.Brief{
		SteamID: id, PeriodStart: now.AddDate(0, 0, -7), PeriodEnd: now, Party: []match.PlayerID{id}, Input: &coach.Input{},
	})
	if err != nil {
		t.Fatal(err)
	}
	a := &coach.Advice{Players: []coach.PlayerAdvice{{Player: "anna", Focus: []coach.Focus{{Title: "Trade the entry"}}}}}
	if err := st.SaveBriefAdvice(t.Context(), bid, a, "rules"); err != nil {
		t.Fatal(err)
	}
	return bid
}

func link(t *testing.T, st *store.Store, id match.PlayerID, chat int64) {
	t.Helper()
	_, hash := NewCode()
	if err := st.CreateTelegramCode(t.Context(), id, hash, time.Now().Add(CodeTTL)); err != nil {
		t.Fatal(err)
	}
	if _, err := st.UseTelegramCode(t.Context(), hash, chat); err != nil {
		t.Fatal(err)
	}
}

func TestNotify(t *testing.T) {
	b, st, f := newBot(t)
	ctx := t.Context()

	// A brief made before the chat was linked is not sent.
	readyBrief(t, st, owner)
	link(t, st, owner, 42)
	link(t, st, friend, 43)
	if err := b.NotifyOnce(ctx); err != nil {
		t.Fatal(err)
	}
	if got := f.take(); len(got) != 0 {
		t.Fatalf("old brief sent: %+v", got)
	}

	id := readyBrief(t, st, owner)
	queued, err := st.CreateBrief(ctx, &store.Brief{
		SteamID: owner, Party: []match.PlayerID{owner}, Input: &coach.Input{},
		PeriodStart: time.Now(), PeriodEnd: time.Now(),
	})
	if err != nil {
		t.Fatal(err)
	}
	readyBrief(t, st, friend)
	f.mu.Lock()
	f.refuse[43] = refusal{http.StatusForbidden, "Forbidden: bot was blocked by the user"}
	f.mu.Unlock()
	if err := b.NotifyOnce(ctx); err != nil {
		t.Fatal(err)
	}
	got := f.take()
	if len(got) != 1 || got[0].Chat != 42 || !strings.Contains(got[0].Text, "• Trade the entry") ||
		!strings.HasSuffix(got[0].Text, "/briefs/"+strconv.FormatInt(id, 10)) {
		t.Fatalf("sent = %+v; want only the owner's ready brief (queued brief %d)", got, queued)
	}
	if _, err := st.GetTelegramLink(ctx, friend); !errors.Is(err, store.ErrNotFound) {
		t.Errorf("a chat that blocked the bot stays linked: %v", err)
	}

	if err := b.NotifyOnce(ctx); err != nil {
		t.Fatal(err)
	}
	if got := f.take(); len(got) != 0 {
		t.Errorf("brief sent twice: %+v", got)
	}

	// A deleted chat is unlinked too; a refusal that will not change is not
	// retried every minute, and a passing failure is.
	for chat, r := range map[int64]refusal{
		44: {http.StatusBadRequest, "Bad Request: chat not found"},
		45: {http.StatusBadRequest, "Bad Request: message is too long"},
		46: {http.StatusTooManyRequests, "Too Many Requests: retry after 5"},
	} {
		id := match.PlayerID(76561198000000010 + chat)
		if err := st.EnsureAdmin(ctx, id); err != nil {
			t.Fatal(err)
		}
		link(t, st, id, chat)
		readyBrief(t, st, id)
		f.mu.Lock()
		f.refuse[chat] = r
		f.mu.Unlock()
	}
	if err := b.NotifyOnce(ctx); err == nil {
		t.Error("a flood-control refusal should be reported")
	}
	if _, err := st.GetTelegramLink(ctx, 76561198000000054); !errors.Is(err, store.ErrNotFound) {
		t.Errorf("a deleted chat stays linked: %v", err)
	}
	left, err := st.BriefsToNotify(ctx, time.Now().Add(-time.Hour))
	if err != nil || len(left) != 1 || left[0].ChatID != 46 {
		t.Errorf("left to send = %+v, %v; want only the flood-controlled one", left, err)
	}
}
