package web

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mrolegmir/cs-demo-coach/internal/detect"
	"github.com/mrolegmir/cs-demo-coach/internal/match"
	"github.com/mrolegmir/cs-demo-coach/internal/store"
)

func TestPersonalUtilityDoesNotFollowPartyOrStartingSides(t *testing.T) {
	st := coachedStore(t)
	m := st.analysis.Match
	id := st.m.Party[0]
	m.UtilityVersion = 1
	st.m.Party = []match.PlayerID{999} // Personal category must ignore party selection.
	m.Rounds[0].UtilityDamage = []match.UtilityDamage{
		{Tick: m.Rounds[0].FreezeEndTick + 64, Attacker: id, Victim: id, Weapon: "Molotov", HealthDamage: new(7), VictimPlace: "SELF-POSITION"},
		{Tick: m.Rounds[0].FreezeEndTick + 128, Attacker: id, Victim: 99, Weapon: "HE Grenade", AttackerSide: match.SideCT, VictimSide: match.SideT, HealthDamage: new(0)},
		{Attacker: id, Victim: 99, Weapon: "HE Grenade", AttackerSide: match.SideCT, VictimSide: match.SideT, ReportedDamage: 999},
		{Attacker: 999, Victim: id, Weapon: "HE Grenade", HealthDamage: new(9999), VictimPlace: "OTHER-PLAYER-POSITION"},
	}
	for _, lang := range []string{"en", "ru"} {
		p := matchPage{Viewer: Viewer{User: &store.User{SteamID: id}}, Match: st.m}
		p.build(st.analysis)
		u := p.Categories[0].Utility
		if !u.Captured || !u.DamageAvailable || len(u.Evidence) != 3 || u.Summary[1].SelfDamage != 7 || u.Summary[0].MissingDamageEvents != 1 || u.Evidence[1].Relation != "Enemy" {
			t.Fatalf("personal utility: %+v", u)
		}
		s, _ := newServer(t, st)
		r := englishRequest(http.MethodGet, "/matches/7", http.NoBody)
		if lang == "ru" {
			r = httptest.NewRequest(http.MethodGet, "/matches/7", http.NoBody)
		}
		w := httptest.NewRecorder()
		s.render(w, r, "match", p)
		body := w.Body.String()
		if w.Code != http.StatusOK || !strings.Contains(body, "<td>7</td>") || strings.Contains(body, "OTHER-PLAYER-POSITION") || strings.Contains(body, "<td>9999</td>") {
			t.Fatalf("bad %s render: %s", lang, body)
		}
		label, missing := "HE and fire damage", "not recorded"
		if lang == "ru" {
			label, missing = "Урон от HE и огня", "не записано"
		}
		if !strings.Contains(body, label) || !strings.Contains(body, missing) {
			t.Fatalf("missing %s utility labels", lang)
		}
	}
	m.UtilityVersion = 0
	if utilityFor(m, id).Captured {
		t.Fatal("legacy utility shown as measured")
	}
	p := matchPage{Viewer: Viewer{User: &store.User{SteamID: 999}}, Match: st.m}
	p.build(st.analysis)
	if len(p.Categories) != 0 {
		t.Fatal("utility assigned to non-participant")
	}
}

func TestUtilityAbsentEventsAreUnavailable(t *testing.T) {
	m := &match.Match{UtilityVersion: 1, Rounds: []match.Round{{Players: []match.PlayerRound{{Player: 1, Side: match.SideCT}}}}}
	u := utilityFor(m, 1)
	if !u.Captured || u.DamageAvailable || len(u.Summary) != 2 {
		t.Fatal("no events treated as measured zero damage")
	}
}

func TestUtilityCaptureAvailabilityRendered(t *testing.T) {
	for _, version := range []int{0, 1, 99} {
		st := coachedStore(t)
		m := st.analysis.Match
		m.UtilityVersion = version
		for i := range m.Rounds {
			m.Rounds[i].UtilityDamage = nil
		}
		p := matchPage{Viewer: Viewer{User: &store.User{SteamID: st.m.Party[0]}}, Match: st.m}
		p.build(st.analysis)
		s, _ := newServer(t, st)
		w := httptest.NewRecorder()
		s.render(w, englishRequest(http.MethodGet, "/matches/7", http.NoBody), "match", p)
		body := w.Body.String()
		if w.Code != http.StatusOK {
			t.Fatal(body)
		}
		if version == 1 {
			if !strings.Contains(body, "Damage totals are unavailable") || !strings.Contains(body, "<dd>—</dd>") {
				t.Fatal("absent events presented as measured totals")
			}
		} else if !strings.Contains(body, "HE and fire damage was not captured") {
			t.Fatal("missing legacy/unsupported utility notice")
		}
	}
}

// Optional local demo uses only the committed anonymized fixture.
func TestUtilityRealDemoPreview(t *testing.T) {
	dir := os.Getenv("CSDC_UTILITY_PREVIEW_DIR")
	if dir == "" {
		t.Skip("no preview directory requested")
	}
	m, err := match.LoadFile(filepath.Join("..", "..", "testdata", "matches", "mm_2026_dust2.json.gz"))
	if err != nil {
		t.Fatal(err)
	}
	var id match.PlayerID
	most := 0
	for _, player := range m.Players {
		if n := len(m.UtilityFor(player.ID).Evidence); n > most {
			most, id = n, player.ID
		}
	}
	if most == 0 {
		t.Fatal("no utility evidence in real demo")
	}
	rep := detect.Run(m, detect.Default())
	st := &fakeStore{m: &store.Match{ID: 7, Map: m.Map, Status: store.StatusCoached}, analysis: &store.Analysis{Match: m, Report: &rep}}
	s, _ := newServer(t, st)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, lang := range []string{"en", "ru"} {
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
