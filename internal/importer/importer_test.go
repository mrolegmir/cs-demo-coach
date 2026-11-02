package importer

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/mrolegmir/cs-demo-coach/internal/match"
	"github.com/mrolegmir/cs-demo-coach/internal/sharecode"
	"github.com/mrolegmir/cs-demo-coach/internal/store"
)

func code(n uint64) string { return sharecode.Code{MatchID: n, OutcomeID: n, TVPort: 1}.String() }

func TestSteamNextCode(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		if r.URL.Path != "/ICSGOPlayers_730/GetNextMatchSharingCode/v1" || q.Get("key") != "k" ||
			q.Get("steamid") != "76561197960265729" || q.Get("steamidkey") != "AAAA-BBBBB-CCCC" {
			t.Errorf("unexpected request %s", r.URL)
		}
		switch q.Get("knowncode") {
		case "old":
			_, _ = io.WriteString(w, `{"result":{"nextcode":"CSGO-new"}}`)
		case "newest":
			w.WriteHeader(http.StatusAccepted)
			_, _ = io.WriteString(w, `{"result":{"nextcode":"n/a"}}`)
		case "newest-empty":
			w.WriteHeader(http.StatusAccepted)
		case "foreign":
			w.WriteHeader(http.StatusPreconditionFailed)
		default:
			w.WriteHeader(http.StatusForbidden)
		}
	}))
	defer srv.Close()
	s := &Steam{Key: "k", Base: srv.URL}
	ctx, player := t.Context(), match.PlayerID(76561197960265729)

	if got, err := s.NextCode(ctx, player, "AAAA-BBBBB-CCCC", "old"); err != nil || got != "CSGO-new" {
		t.Errorf("next after old = %q, %v", got, err)
	}
	if got, err := s.NextCode(ctx, player, "AAAA-BBBBB-CCCC", "newest"); err != nil || got != "" {
		t.Errorf("next after newest = %q, %v; want none", got, err)
	}
	if got, err := s.NextCode(ctx, player, "AAAA-BBBBB-CCCC", "newest-empty"); err != nil || got != "" {
		t.Errorf("next after newest, empty 202 = %q, %v; want none", got, err)
	}
	if _, err := s.NextCode(ctx, player, "AAAA-BBBBB-CCCC", "foreign"); !errors.Is(err, ErrUnknownCode) {
		t.Errorf("foreign code err = %v, want ErrUnknownCode", err)
	}
	if _, err := s.NextCode(ctx, player, "AAAA-BBBBB-CCCC", "x"); !errors.Is(err, ErrForbidden) {
		t.Errorf("rejected err = %v, want ErrForbidden", err)
	}
}

func TestSteamErrorsHideSecrets(t *testing.T) {
	s := &Steam{Key: "secret-key", Base: "http://127.0.0.1:1"}
	_, err := s.NextCode(t.Context(), 1, "secret-auth", "CSGO-x")
	if err == nil {
		t.Fatal("want a connection error")
	}
	if msg := err.Error(); strings.Contains(msg, "secret") {
		t.Errorf("error leaks a secret: %s", msg)
	}
}

// fakeAPI serves a history of codes per player.
type fakeAPI struct {
	history  map[match.PlayerID][]string
	reject   map[match.PlayerID]bool
	badKey   bool
	fail     bool
	requests int
}

func (f *fakeAPI) NextCode(_ context.Context, p match.PlayerID, _, known string) (string, error) {
	f.requests++
	if f.fail {
		return "", errors.New("connection reset")
	}
	if f.badKey || f.reject[p] {
		return "", ErrForbidden
	}
	h := f.history[p]
	i := slices.Index(h, known)
	if i < 0 {
		return "", ErrUnknownCode
	}
	if i == len(h)-1 {
		return "", nil
	}
	return h[i+1], nil
}

func (f *fakeAPI) KeyValid(context.Context, match.PlayerID) (bool, error) { return !f.badKey, nil }

type recorded struct {
	codes   []string
	linkErr string
}

type fakeStore struct {
	links   []store.SteamLink
	got     map[match.PlayerID]recorded
	failFor match.PlayerID
}

func (f *fakeStore) SteamLinksToCheck(context.Context, time.Time) ([]store.SteamLink, error) {
	return f.links, nil
}

func (f *fakeStore) RecordShareCodes(_ context.Context, l store.SteamLink, codes []string, linkErr string) (bool, error) {
	if f.failFor == l.SteamID {
		return false, errors.New("db down")
	}
	f.got[l.SteamID] = recorded{codes, linkErr}
	return true, nil
}

func newImporter(api *fakeAPI, links ...store.SteamLink) (*Importer, *fakeStore) {
	st := &fakeStore{links: links, got: map[match.PlayerID]recorded{}}
	return &Importer{Store: st, API: api, Log: slog.New(slog.DiscardHandler), Every: time.Hour}, st
}

func TestRunOnce(t *testing.T) {
	api := &fakeAPI{
		history: map[match.PlayerID][]string{
			1: {code(1), code(2), code(3)},
			2: {code(10)},
			3: {code(20)},
		},
		reject: map[match.PlayerID]bool{4: true},
	}
	im, st := newImporter(api,
		store.SteamLink{SteamID: 1, LastCode: code(1)},
		store.SteamLink{SteamID: 2, LastCode: code(10)},
		store.SteamLink{SteamID: 3, LastCode: code(99)},
		store.SteamLink{SteamID: 4, LastCode: code(1)},
	)
	if err := im.RunOnce(t.Context(), time.Now()); err != nil {
		t.Fatal(err)
	}
	want := map[match.PlayerID]recorded{
		1: {codes: []string{code(2), code(3)}},
		2: {},
		3: {linkErr: MsgUnknownCode},
		4: {linkErr: MsgAuthCode},
	}
	for id, w := range want {
		g, ok := st.got[id]
		if !ok || !slices.Equal(g.codes, w.codes) || g.linkErr != w.linkErr {
			t.Errorf("user %d: recorded %+v (ok %v), want %+v", id, g, ok, w)
		}
	}
}

func TestRunOnceCapsOneCheck(t *testing.T) {
	var h []string
	for i := range MaxPerCheck + 5 {
		h = append(h, code(uint64(i+1)))
	}
	im, st := newImporter(&fakeAPI{history: map[match.PlayerID][]string{1: h}},
		store.SteamLink{SteamID: 1, LastCode: h[0]})
	if err := im.RunOnce(t.Context(), time.Now()); err != nil {
		t.Fatal(err)
	}
	if got := st.got[1].codes; len(got) != MaxPerCheck || got[0] != h[1] {
		t.Errorf("recorded %d codes starting %v, want %d starting %s", len(got), got[:1], MaxPerCheck, h[1])
	}
}

func TestRunOnceBadKeyDisablesNobody(t *testing.T) {
	api := &fakeAPI{badKey: true}
	im, st := newImporter(api, store.SteamLink{SteamID: 1}, store.SteamLink{SteamID: 2})
	if err := im.RunOnce(t.Context(), time.Now()); !errors.Is(err, errBadKey) {
		t.Fatalf("err = %v, want errBadKey", err)
	}
	if len(st.got) != 0 || api.requests != 1 {
		t.Errorf("recorded %v after %d requests; want nothing after the first", st.got, api.requests)
	}
}

func TestRunOnceNetworkErrorKeepsLink(t *testing.T) {
	im, st := newImporter(&fakeAPI{fail: true}, store.SteamLink{SteamID: 1})
	if err := im.RunOnce(t.Context(), time.Now()); err != nil {
		t.Fatal(err)
	}
	// The check is recorded, so the link waits Every, but it stays enabled.
	if g, ok := st.got[1]; !ok || g.linkErr != "" || len(g.codes) != 0 {
		t.Errorf("recorded %+v (ok %v); want an empty check without an error", g, ok)
	}
}

func TestRunOnceBadCodeFromSteam(t *testing.T) {
	api := &fakeAPI{history: map[match.PlayerID][]string{1: {code(1), code(2), "garbage"}}}
	im, st := newImporter(api, store.SteamLink{SteamID: 1, LastCode: code(1)})
	if err := im.RunOnce(t.Context(), time.Now()); err != nil {
		t.Fatal(err)
	}
	if g := st.got[1]; !slices.Equal(g.codes, []string{code(2)}) || g.linkErr != "" {
		t.Errorf("recorded %+v; want the code before the bad one, link left enabled", g)
	}
}

func TestRunOnceStoreErrorSkipsOnlyThatLink(t *testing.T) {
	api := &fakeAPI{history: map[match.PlayerID][]string{1: {code(1)}, 2: {code(2)}}}
	im, st := newImporter(api, store.SteamLink{SteamID: 1, LastCode: code(1)}, store.SteamLink{SteamID: 2, LastCode: code(2)})
	st.failFor = 1
	if err := im.RunOnce(t.Context(), time.Now()); err == nil {
		t.Fatal("want the store error reported")
	}
	if _, ok := st.got[2]; !ok {
		t.Error("the link after the failing one was not checked")
	}
}
