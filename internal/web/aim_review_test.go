package web

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mrolegmir/cs-demo-coach/internal/detect"
	"github.com/mrolegmir/cs-demo-coach/internal/jobwire"
	"github.com/mrolegmir/cs-demo-coach/internal/match"
	"github.com/mrolegmir/cs-demo-coach/internal/store"
)

func TestMechanicsDetectionPayloadFitsInternalAPI(t *testing.T) {
	m, err := match.LoadFile(filepath.Join("..", "..", "testdata", "matches", "mm_2026_dust2.json.gz"))
	if err != nil {
		t.Fatal(err)
	}
	rep := detect.Run(m, detect.Default())
	b, err := json.Marshal(jobwire.DetectionRequest{Attempt: 1, Match: m, Report: rep})
	if err != nil {
		t.Fatal(err)
	}
	if len(b) >= maxInternalBody {
		t.Fatalf("detection payload %d exceeds API limit %d", len(b), maxInternalBody)
	}
	var decoded jobwire.DetectionRequest
	if err := decode(httptest.NewRequest(http.MethodPut, "/internal/v2/jobs/7/detection", bytes.NewReader(b)), &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.Match.MechanicsVersion != 4 || decoded.Match.Rounds[0].Shots[0].View == nil {
		t.Fatal("mechanics fields lost in worker/API transport")
	}
	t.Logf("uncompressed worker detection bytes=%d limit=%d", len(b), maxInternalBody)
}

func TestPersonalAimMovementRendering(t *testing.T) {
	m := &match.Match{MechanicsVersion: 4, TickRate: 64, Players: []match.Player{{ID: 1, Name: "one"}, {ID: 2, Name: "No"}}, Rounds: []match.Round{{Number: 1, Players: []match.PlayerRound{{Player: 1, Side: match.SideT}}, Shots: []match.Shot{
		{Tick: 100, Player: 1, Weapon: "AK-47", Speed: new(0.0), Preparation: []match.MovementSample{{Tick: 99, Speed: new(10.0)}}},
		{Tick: 101, Player: 2, Weapon: "M4A4"},
	}, Damage: []match.Damage{{Tick: 100, Attacker: 1, Victim: 2, Weapon: "AK-47", HealthDamage: 20, Preparation: []match.ContactSample{
		{MovementSample: match.MovementSample{Tick: 98, Speed: new(0.0), View: &match.ViewSample{Eye: &match.Vec{}}}, TargetEye: &match.Vec{X: 100}},
		{MovementSample: match.MovementSample{Tick: 99}},
	}}}}}}
	rep := detect.Run(m, detect.Default())
	st := &fakeStore{m: &store.Match{ID: 7, Status: store.StatusCoached, Party: []match.PlayerID{2}}, analysis: &store.Analysis{Match: m, Report: &rep}}
	for _, lang := range []string{"ru", "en"} {
		p := matchPage{Viewer: Viewer{User: &store.User{SteamID: 1}}, Match: st.m}
		p.build(st.analysis)
		sv, mv := p.Categories[4], p.Categories[5]
		if sv.State != "unknown" || sv.Priority != 0 || sv.Status != "Observations available" || len(sv.Shooting.Episodes) != 1 || len(mv.Movement.Episodes[0].Fires) != 1 {
			t.Fatal("personal observations missing or promoted to a grade")
		}
		if row := sv.Shooting.Episodes[0]; !row.Contact[0].AngleKnown || row.Contact[0].Angle != 0 || row.Contact[1].AngleKnown || row.Target != "No" {
			t.Fatal("zero/missing angle or identity lost")
		}
		s, _ := newServer(t, st)
		w := httptest.NewRecorder()
		r := englishRequest(http.MethodGet, "/matches/7", http.NoBody)
		if lang == "ru" {
			r = httptest.NewRequest(http.MethodGet, "/matches/7", http.NoBody)
		}
		s.render(w, r, "match", p)
		body := w.Body.String()
		if w.Code != http.StatusOK || !strings.Contains(body, "<td>0.00</td>") || !strings.Contains(body, "No ·") {
			t.Fatalf("bad %s aim render: %s", lang, body)
		}
	}
	m.MechanicsVersion = 3
	if v := mechanicsReviewFor(m, 1); v.TrajectoryCaptured || len(v.Episodes[0].Contact) != 0 || len(v.Episodes[0].Preparation) != 1 {
		t.Fatal("legacy angle invented or movement lost")
	}
	m.MechanicsVersion = 0
	if v := mechanicsReviewFor(m, 1); v.Captured || len(v.Episodes) != 0 {
		t.Fatal("legacy shots invented")
	}
	if len(mechanicsReviewFor(m, 99).Episodes) != 0 {
		t.Fatal("non-participant assigned observations")
	}
}

func TestAimMovementRealDemoPreview(t *testing.T) {
	dir := os.Getenv("CSDC_AIM_PREVIEW_DIR")
	if dir == "" {
		t.Skip("no preview requested")
	}
	m, err := match.LoadFile(filepath.Join("..", "..", "testdata", "matches", "mm_2026_dust2.json.gz"))
	if err != nil {
		t.Fatal(err)
	}
	var id match.PlayerID
	most := -1
	for _, p := range m.Players {
		n := 0
		for _, e := range m.RifleEpisodes(p.ID) {
			if e.Contact != nil {
				n += len(e.Contact.Preparation)
			}
		}
		if n > most {
			most, id = n, p.ID
		}
	}
	if most <= 0 {
		t.Fatal("no paired contact evidence")
	}
	rep := detect.Run(m, detect.Default())
	st := &fakeStore{m: &store.Match{ID: 7, Map: m.Map, Status: store.StatusCoached}, analysis: &store.Analysis{Match: m, Report: &rep}}
	s, _ := newServer(t, st)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, version := range []int{4, 3} {
		m.MechanicsVersion = version
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
			if version == 3 {
				name = "legacy-" + name
			}
			if err := os.WriteFile(filepath.Join(dir, name), w.Body.Bytes(), 0o600); err != nil {
				t.Fatal(err)
			}
		}
	}
}
