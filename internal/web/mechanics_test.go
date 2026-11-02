package web

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/mrolegmir/cs-demo-coach/internal/detect"
	"github.com/mrolegmir/cs-demo-coach/internal/match"
	"github.com/mrolegmir/cs-demo-coach/internal/store"
)

func TestMechanicsCoverageRendered(t *testing.T) {
	for _, version := range []int{0, 1, 2, 3} {
		st := coachedStore(t)
		st.analysis.Match.MechanicsVersion = version
		s, h := newServer(t, st)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, englishRequest(http.MethodGet, "/matches/7", http.NoBody))
		if rec.Code != http.StatusOK {
			t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
		}
		body := rec.Body.String()
		if strings.Contains(body, "Above 80") || strings.Contains(body, "Diagnostics") {
			t.Fatal("diagnostic telemetry exposed on the player page")
		}
		r := englishRequest(http.MethodGet, "/matches/7?diagnostics=1", http.NoBody)
		r.SetPathValue("id", "7")
		rec = httptest.NewRecorder()
		s.match(rec, r, &store.User{Admin: true})
		body = rec.Body.String()
		if version == 0 && !strings.Contains(body, "newly parsed demo is required") {
			t.Fatal("missing legacy notice")
		}
		if version == 1 && !strings.Contains(body, "Recent speed available") {
			t.Fatal("missing measurements")
		}
		if version == 2 && !strings.Contains(body, "No usable firearm damage events") {
			t.Fatal("missing unsupported damage notice")
		}
	}
}

func TestMovementHistoryRendering(t *testing.T) {
	st := coachedStore(t)
	m := st.analysis.Match
	m.MechanicsVersion = 3
	id := st.m.Party[0]
	tick := m.Rounds[0].FreezeEndTick + 64
	m.Rounds[0].Shots = []match.Shot{
		{Tick: tick, Player: id, Weapon: "AK-47", Preparation: []match.MovementSample{
			{Tick: tick - 2, WindowTicks: 1, Speed: new(200.0)},
			{Tick: tick - 1},
			{Tick: tick, WindowTicks: 1, Speed: new(0.0)},
		}},
		{Tick: tick, Player: 999, Weapon: "AK-47", Place: "PRIVATE-PLACE"},
	}
	_, h := newServer(t, st)
	for _, lang := range []string{"en", "ru"} {
		rec := httptest.NewRecorder()
		r := englishRequest(http.MethodGet, "/matches/7", http.NoBody)
		if lang == "ru" {
			r = httptest.NewRequest(http.MethodGet, "/matches/7", http.NoBody)
		}
		h.ServeHTTP(rec, r)
		body := rec.Body.String()
		if rec.Code != http.StatusOK || !strings.Contains(body, "<td>200</td>") || !strings.Contains(body, "<td>0</td>") || strings.Contains(body, "%!f") || strings.Contains(body, "PRIVATE-PLACE") {
			t.Fatalf("invalid %s movement render: %s", lang, body)
		}
		label, missing := "Movement before firing", "not recorded"
		if lang == "ru" {
			label, missing = "Движение перед стрельбой", "не записано"
		}
		if !strings.Contains(body, label) || !strings.Contains(body, missing) {
			t.Fatalf("missing %s movement labels", lang)
		}
	}
}

func TestShootingEvidenceParty(t *testing.T) {
	st := coachedStore(t)
	m := st.analysis.Match
	m.MechanicsVersion = 2
	id := st.m.Party[0]
	m.Rounds[0].Shots = []match.Shot{{Tick: m.Rounds[0].FreezeEndTick + 64, Player: id, Weapon: "AK-47"}}
	m.Rounds[0].Damage = []match.Damage{{Tick: m.Rounds[0].FreezeEndTick + 64, Attacker: id, Victim: 99, Weapon: "AK-47", HealthDamage: 20}}
	p := matchPage{Match: st.m}
	p.build(st.analysis)
	if !p.DamageAvailable {
		t.Fatal("damage marked unavailable")
	}
	for _, s := range p.Shooting {
		found := false
		for _, party := range st.m.Party {
			if s.Player == party {
				found = true
			}
		}
		if !found {
			t.Fatal("other player's sequence exposed in party table")
		}
	}
	s, _ := newServer(t, st)
	rec := httptest.NewRecorder()
	r := englishRequest(http.MethodGet, "/matches/7?diagnostics=1", http.NoBody)
	r.SetPathValue("id", "7")
	s.match(rec, r, &store.User{Admin: true})
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "First damage at shot") {
		t.Fatal("missing sequence evidence table")
	}
}

func TestMechanicsPartyAndLegacy(t *testing.T) {
	m := &match.Match{MechanicsVersion: 1, TickRate: 64, Players: []match.Player{{ID: 1, Name: "one"}, {ID: 2, Name: "two"}}, Rounds: []match.Round{{Number: 1, Shots: []match.Shot{{Player: 1, Weapon: "AK-47", Tick: 10}, {Player: 2, Weapon: "M4A1", Tick: 11}}}}}
	p := matchPage{Match: &store.Match{Party: []match.PlayerID{1}}}
	p.build(&store.Analysis{Match: m, Report: &detect.Report{}})
	if !p.MechanicsAvailable || len(p.Mechanics) != 1 || p.Mechanics[0].Name != "one" || p.Mechanics[0].SpeedSamples != 0 {
		t.Fatalf("page = %+v", p)
	}
	m.MechanicsVersion = 0
	p = matchPage{Match: &store.Match{}}
	p.build(&store.Analysis{Match: m, Report: &detect.Report{}})
	if p.MechanicsAvailable || len(p.Mechanics) != 0 {
		t.Fatal("legacy analysis presented as measured")
	}
}
