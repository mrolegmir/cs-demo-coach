package web

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/mrolegmir/cs-demo-coach/internal/detect"
	"github.com/mrolegmir/cs-demo-coach/internal/match"
	"github.com/mrolegmir/cs-demo-coach/internal/store"
)

func flashPageMatch() *match.Match {
	return &match.Match{TickRate: 64, Players: []match.Player{{ID: 1, Name: "No"}, {ID: 2, Name: "<b>ally</b>"}, {ID: 3, Name: "enemy"}}, Rounds: []match.Round{{
		Number: 1, FreezeEndTick: 64, EndTick: 512,
		Players: []match.PlayerRound{{Player: 1, Side: match.SideT}, {Player: 2, Side: match.SideT}, {Player: 3, Side: match.SideCT}},
		Flashes: []match.Flash{{Tick: 100, Thrower: 1, Side: match.SideT, Place: "LongA", Blinded: []match.Blinding{{Player: 3, Side: match.SideCT, Duration: 2 * time.Second}}}},
		Kills:   []match.Kill{{Tick: 164, Killer: 2, Victim: 3, Alive: []match.PlayerState{{Player: 2, Side: match.SideT}, {Player: 3, Side: match.SideCT}}}},
	}}}
}

func TestFlashFollowupRenderingIsPersonalAndNeutral(t *testing.T) {
	m := flashPageMatch()
	rep := detect.Run(m, detect.Default())
	st := &fakeStore{m: &store.Match{ID: 7, Status: store.StatusCoached, Party: []match.PlayerID{3}}, analysis: &store.Analysis{Match: m, Report: &rep}}
	for _, lang := range []string{"ru", "en"} {
		p := matchPage{Viewer: Viewer{User: &store.User{SteamID: 1}}, Match: st.m}
		p.build(st.analysis)
		v := p.Categories[1].FlashSupport
		if len(v.Flashes) != 1 || v.TeammateKills != 1 || v.Kills[0].State != "One eligible flash" || p.Categories[1].Priority != 0 {
			t.Fatal("personal flash support lost or promoted to a priority")
		}
		s, _ := newServer(t, st)
		w := httptest.NewRecorder()
		r := englishRequest(http.MethodGet, "/matches/7", http.NoBody)
		if lang == "ru" {
			r = httptest.NewRequest(http.MethodGet, "/matches/7", http.NoBody)
		}
		s.render(w, r, "match", p)
		body := w.Body.String()
		if w.Code != http.StatusOK || !strings.Contains(body, "No ·") || !strings.Contains(body, "&lt;b&gt;ally&lt;/b&gt;") || strings.Contains(body, "<b>ally</b>") {
			t.Fatalf("invalid %s flash render: %s", lang, body)
		}
		label := "Flash follow-up"
		if lang == "ru" {
			label = "Игра после флешек"
		}
		if !strings.Contains(body, label) {
			t.Fatal("missing flash localization")
		}
	}
	if v := flashSupportFor(m, 2); len(v.Flashes) != 0 || v.SupportedKills != 1 {
		t.Fatal("killer's perspective lost")
	}
	p := matchPage{Viewer: Viewer{User: &store.User{SteamID: 99}}, Match: st.m}
	p.build(st.analysis)
	if len(p.Categories) != 0 {
		t.Fatal("non-participant assigned another player's flashes")
	}
}

func TestFlashTimingMissingAndAmbiguity(t *testing.T) {
	m := flashPageMatch()
	m.TickRate = 0
	v := flashSupportFor(m, 1)
	if v.TimingAvailable || len(v.Flashes) != 1 || v.Flashes[0].TimeKnown || len(v.Kills) != 0 {
		t.Fatal("unknown timing became measured zero support")
	}
	m.TickRate = 64
	m.Rounds[0].Flashes = append(m.Rounds[0].Flashes, match.Flash{Tick: 110, Blinded: []match.Blinding{{Player: 3, Side: match.SideCT, Duration: 2 * time.Second}}})
	v = flashSupportFor(m, 1)
	if v.AmbiguousKills != 1 || v.TeammateKills != 0 || v.Kills[0].DelayKnown || v.Kills[0].Sources[1].Known {
		t.Fatal("ambiguous source was guessed")
	}
}

func TestFlashSupportRealDemoPreview(t *testing.T) {
	dir := os.Getenv("CSDC_FLASH_PREVIEW_DIR")
	if dir == "" {
		t.Skip("no preview directory requested")
	}
	m, err := match.LoadFile(filepath.Join("..", "..", "testdata", "matches", "mm_2026_dust2.json.gz"))
	if err != nil {
		t.Fatal(err)
	}
	var id match.PlayerID
	most := -1
	for _, player := range m.Players {
		rep := m.FlashSupportFor(player.ID)
		if n := rep.TeammateKills + rep.SupportedKills; n > most {
			most, id = n, player.ID
		}
	}
	if most <= 0 {
		t.Fatal("no real-demo flash follow-up")
	}
	rep := detect.Run(m, detect.Default())
	st := &fakeStore{m: &store.Match{ID: 7, Map: m.Map, Status: store.StatusCoached}, analysis: &store.Analysis{Match: m, Report: &rep}}
	s, _ := newServer(t, st)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, lang := range []string{"ru", "en"} {
		p := matchPage{Viewer: Viewer{User: &store.User{SteamID: id}}, Match: st.m}
		p.build(st.analysis)
		w := httptest.NewRecorder()
		r := englishRequest(http.MethodGet, "/matches/7", http.NoBody)
		if lang == "ru" {
			r = httptest.NewRequest(http.MethodGet, "/matches/7", http.NoBody)
		}
		s.render(w, r, "match", p)
		if w.Code != http.StatusOK {
			t.Fatal(w.Body.String())
		}
		if err := os.WriteFile(filepath.Join(dir, lang+".html"), w.Body.Bytes(), 0o600); err != nil {
			t.Fatal(err)
		}
	}
}
