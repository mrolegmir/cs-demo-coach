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

func decisionMatch() *match.Match {
	return &match.Match{ObjectiveVersion: 1, TickRate: 64, Players: []match.Player{{ID: 1, Name: "No"}, {ID: 2, Name: "enemy"}, {ID: 3, Name: "friend"}, {ID: 4, Name: "fourth"}}, Rounds: []match.Round{
		{Number: 1, FreezeEndTick: 640, EndTick: 2000, Winner: match.SideCT, Players: []match.PlayerRound{{Player: 1, Side: match.SideCT}}, BombEvents: []match.BombEvent{
			{Tick: 1280, Kind: match.BombPlanted, Player: 2, Site: "A", AliveComplete: true, Alive: []match.PlayerState{{Player: 1, Side: match.SideCT}, {Player: 3, Side: match.SideCT}, {Player: 2, Side: match.SideT}}},
			{Tick: 1600, Kind: match.BombDefuseBegin, Player: 1, HasKit: new(false)},
			{Tick: 1920, Kind: match.BombDefused, Player: 1, Site: "A"},
		}},
		{Number: 2, FreezeEndTick: 640, EndTick: 2000, Winner: match.SideCT, Players: []match.PlayerRound{{Player: 1, Side: match.SideT}}, Kills: []match.Kill{
			{Tick: 704, Victim: 4, Alive: []match.PlayerState{{Player: 1, Side: match.SideT}, {Player: 2, Side: match.SideT}, {Player: 3, Side: match.SideCT}, {Player: 4, Side: match.SideCT}}},
		}},
	}}
}

func TestDecisionsArePersonalAndNeutral(t *testing.T) {
	m := decisionMatch()
	rep := detect.Run(m, detect.Default())
	st := &fakeStore{m: &store.Match{ID: 7, Status: store.StatusCoached, Party: []match.PlayerID{4}}, analysis: &store.Analysis{Match: m, Report: &rep}}
	for _, lang := range []string{"en", "ru"} {
		p := matchPage{Viewer: Viewer{User: &store.User{SteamID: 1}}, Match: st.m}
		p.build(st.analysis)
		c := p.Categories[len(p.Categories)-1]
		if c.Title != "Decisions" || c.State != "unknown" || c.Priority != 0 || len(c.Decisions.Advantages) != 1 || len(c.Decisions.Rounds) != 1 {
			t.Fatalf("decisions: %+v", c)
		}
		o := c.Decisions.Rounds[0]
		if !o.PlantKnown || !o.ViewerAlive || o.TAlive != 1 || o.CTAlive != 2 || o.Result != "Won" || !o.Events[1].KitKnown || o.Events[1].HasKit || o.Events[1].SincePlant != 5*time.Second {
			t.Fatalf("objective view: %+v", o)
		}
		s, _ := newServer(t, st)
		w := httptest.NewRecorder()
		r := englishRequest(http.MethodGet, "/matches/7", http.NoBody)
		if lang == "ru" {
			r = httptest.NewRequest(http.MethodGet, "/matches/7", http.NoBody)
		}
		s.render(w, r, "match", p)
		body := w.Body.String()
		if w.Code != http.StatusOK || !strings.Contains(body, "<td>No</td>") || !strings.Contains(body, "+0:05") {
			t.Fatalf("bad %s decision render: %s", lang, body)
		}
		label := "Bomb and post-plant timeline"
		if lang == "ru" {
			label = "Бомба и игра после установки"
		}
		if !strings.Contains(body, label) {
			t.Fatal("missing localized objective section")
		}
	}
	p := matchPage{Viewer: Viewer{User: &store.User{SteamID: 99}}, Match: st.m}
	p.build(st.analysis)
	if len(p.Categories) != 0 {
		t.Fatal("assigned decisions to non-participant")
	}
}

func TestDecisionsPreserveUnknownAndLegacyData(t *testing.T) {
	m := decisionMatch()
	m.ObjectiveVersion = 0
	v := decisionsFor(m, 1)
	if v.ObjectiveCaptured || len(v.Rounds) != 0 || len(v.Advantages) != 1 {
		t.Fatal("legacy advantage review depends on new parse")
	}
	m.ObjectiveVersion = 1
	m.Rounds[0].BombEvents[0].AliveComplete = false
	m.Rounds[0].BombEvents[1].HasKit = nil
	m.Rounds[0].BombEvents[1].Player = 0
	v = decisionsFor(m, 1)
	if v.Rounds[0].PlantKnown || v.Rounds[0].Events[1].KitKnown || v.Rounds[0].Events[1].ActorKnown {
		t.Fatal("missing fields were guessed")
	}
	m.Rounds[0].BombEvents = append(m.Rounds[0].BombEvents, m.Rounds[0].BombEvents[0])
	v = decisionsFor(m, 1)
	if v.Rounds[0].PlantKnown || v.Rounds[0].Events[1].AfterPlant {
		t.Fatal("duplicate plant supplied fabricated reference time")
	}
	m.TickRate = 0
	v = decisionsFor(m, 1)
	if v.Advantages[0].TimeKnown || v.Rounds[0].Events[0].TimeKnown || v.Rounds[0].Events[1].AfterPlant {
		t.Fatal("invalid tick rate supplied times")
	}
}

func TestObjectiveDeathsAreMergedOnlyAfterUniquePlant(t *testing.T) {
	m := decisionMatch()
	m.Rounds[0].Kills = []match.Kill{{Tick: 1408, Victim: 3}, {Tick: 1000, Victim: 4}, {Tick: 2500, Victim: 2}}
	v := decisionsFor(m, 1)
	o := v.Rounds[0]
	if len(o.Events) != 4 || o.Events[1].Action != "Player death" || o.Events[1].Player != "friend" || o.Events[1].SincePlant != 2*time.Second || !o.PlantKnown {
		t.Fatalf("merged timeline: %+v", o)
	}
	m.Rounds[0].Kills = append(m.Rounds[0].Kills, match.Kill{Tick: 1280, Victim: 4})
	if decisionsFor(m, 1).Rounds[0].PlantKnown {
		t.Fatal("same-tick death gave a definite plant count")
	}
	m.Rounds[0].BombEvents = append(m.Rounds[0].BombEvents, m.Rounds[0].BombEvents[0])
	if got := decisionsFor(m, 1).Rounds[0]; len(got.Events) != 4 || got.Events[1].Action == "Player death" {
		t.Fatal("ambiguous plant included inferred post-plant deaths")
	}
}

func TestDecisionsRealDemoPreview(t *testing.T) {
	dir := os.Getenv("CSDC_DECISIONS_PREVIEW_DIR")
	if dir == "" {
		t.Skip("no preview directory requested")
	}
	m, err := match.LoadFile(filepath.Join("..", "..", "testdata", "matches", "mm_2026_dust2.json.gz"))
	if err != nil {
		t.Fatal(err)
	}
	var id match.PlayerID
	most := -1
	for _, p := range m.Players {
		if n := len(m.LostAdvantages(p.ID)); n > most {
			most, id = n, p.ID
		}
	}
	rep := detect.Run(m, detect.Default())
	st := &fakeStore{m: &store.Match{ID: 7, Map: m.Map, Status: store.StatusCoached}, analysis: &store.Analysis{Match: m, Report: &rep}}
	s, _ := newServer(t, st)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, version := range []int{1, 0} {
		m.ObjectiveVersion = version
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
			name := lang + ".html"
			if version == 0 {
				name = "legacy-" + name
			}
			if err := os.WriteFile(filepath.Join(dir, name), w.Body.Bytes(), 0o600); err != nil {
				t.Fatal(err)
			}
		}
	}
}
