package web

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mrolegmir/cs-demo-coach/internal/coach"
	"github.com/mrolegmir/cs-demo-coach/internal/detect"
	"github.com/mrolegmir/cs-demo-coach/internal/match"
	"github.com/mrolegmir/cs-demo-coach/internal/store"
)

func TestSkillCoverageAndPersonalEvidence(t *testing.T) {
	st := coachedStore(t)
	id := st.m.Party[0]
	st.m.Party = nil // The viewer, not the saved party, defines the dashboard.
	p := matchPage{Viewer: Viewer{User: &store.User{SteamID: id}}, Match: st.m}
	p.build(st.analysis)
	if len(p.Categories) != 7 {
		t.Fatalf("categories: %d", len(p.Categories))
	}
	for _, c := range p.Categories {
		if c.Title == "Shooting" || c.Title == "Movement" {
			if c.State != "unknown" {
				t.Fatalf("invented mechanics assessment: %+v", c)
			}
		}
		for _, check := range c.Checks {
			for _, f := range check.Evidence {
				found := false
				for _, original := range st.analysis.Report.Findings {
					if original.ID == f.ID && original.Player == id && original.Kind == check.Kind {
						found = true
					}
				}
				if !found || (f.Side != "T" && f.Side != "CT") {
					t.Fatalf("wrong personal evidence: %+v", f)
				}
			}
		}
	}
	missing := matchPage{Viewer: Viewer{User: &store.User{SteamID: 99}}, Match: st.m}
	missing.build(st.analysis)
	if len(missing.Categories) != 0 {
		t.Fatal("assigned another participant's skills to viewer")
	}

	s, _ := newServer(t, st)
	w := httptest.NewRecorder()
	s.render(w, httptest.NewRequest(http.MethodGet, "/matches/7", http.NoBody), "match", p)
	if w.Code != http.StatusOK {
		t.Fatal(w.Body.String())
	}
	body := w.Body.String()
	for _, text := range []string{"Анализ твоей игры", "Гранаты", "Командная игра", "Экономика", "Стрельба", "Мувмент", "Позиционирование", "Начать тренировку", "Твоя команда", "Соперники"} {
		if !strings.Contains(body, text) {
			t.Errorf("missing %q", text)
		}
	}
	if strings.Index(body, "Анализ твоей игры") > strings.Index(body, "Подробный разбор тренера") {
		t.Fatal("old prose precedes categories")
	}
}

func TestPlaceGuideAndPriorityAreSpecificToTheMatch(t *testing.T) {
	m := &match.Match{Map: "de_cache", Players: []match.Player{{ID: 1, Name: "player01"}}, Rounds: []match.Round{{Players: []match.PlayerRound{{Player: 1, Side: match.SideT}}}}}
	rep := &detect.Report{
		Stats:    []detect.Stat{{Player: 1, Kind: detect.KindFlashNoEnemy, Count: 1, Chances: 3}, {Player: 1, Kind: detect.KindTeamFlash, Count: 1, Chances: 3}},
		Findings: []detect.Finding{{ID: "f1", Player: 1, Kind: detect.KindFlashNoEnemy, Place: "Quad", Side: match.SideT, Round: 1}, {ID: "f2", Player: 1, Kind: detect.KindTeamFlash, Place: "Forklift", Side: match.SideT, Round: 1}},
	}
	an := &store.Analysis{Match: m, Report: rep, Advice: &coach.Advice{Players: []coach.PlayerAdvice{{Player: "player01", Focus: []coach.Focus{{Kind: detect.KindFlashNoEnemy}}}}}}
	p := matchPage{Viewer: Viewer{User: &store.User{SteamID: 1}}, Match: &store.Match{ID: 7, Map: m.Map, Party: []match.PlayerID{1}}}
	p.build(an)
	if p.CalloutURL != "https://totalcsgo.com/callouts/cache" || p.Categories[0].Priority != 1 || p.Categories[0].Checks[0].Priority != 1 || p.Categories[0].Checks[1].Priority != 0 || len(p.SkillPlan) != 1 || p.SkillPlan[0].Priority != 1 {
		t.Fatalf("guide or priority incorrect: %+v", p.Categories[0])
	}
	s, _ := newServer(t, coachedStore(t))
	w := httptest.NewRecorder()
	s.render(w, httptest.NewRequest(http.MethodGet, "/matches/7", http.NoBody), "match", p)
	if w.Code != http.StatusOK {
		t.Fatal(w.Body.String())
	}
	for _, want := range []string{"Где находятся эти позиции?", "Приоритет тренера</span>", "Приоритет тренера 1", "четырёх ящиков", "погрузчика", "откуда ты бросал флешку", "https://totalcsgo.com/callouts/cache"} {
		if !strings.Contains(w.Body.String(), want) {
			t.Errorf("missing %q", want)
		}
	}
	if dir := os.Getenv("CSDC_PREVIEW_DIR"); dir != "" {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "cache.html"), w.Body.Bytes(), 0o600); err != nil {
			t.Fatal(err)
		}
	}
}

func TestUnknownMapsDoNotInventCalloutMappings(t *testing.T) {
	for _, name := range []string{"", "de_custom", "de_cache/../../bad", "https://example.com"} {
		if got := calloutGuide(name); got != "https://totalcsgo.com/callouts" {
			t.Fatalf("untrusted map produced URL %q", got)
		}
	}
	if placeHint("de_mirage", "Quad") != "" || placeHint("de_cache", "Unknown") != "" || placeHint("de_cache", "Fork Lift") == "" {
		t.Fatal("place hint assigned to wrong map or missing variant")
	}
}

func TestSkillZeroMissingAndObservedAreDistinct(t *testing.T) {
	m := &match.Match{Players: []match.Player{{ID: 1}}, Rounds: []match.Round{{Players: []match.PlayerRound{{Player: 1, Side: match.SideT}}}}}
	rep := &detect.Report{Stats: []detect.Stat{{Player: 1, Kind: detect.KindFlashNoEnemy, Chances: 7}, {Player: 1, Kind: detect.KindUnusedUtility, Count: 2, Chances: 3}}}
	p := matchPage{Viewer: Viewer{User: &store.User{SteamID: 1}}, Match: &store.Match{}}
	p.buildSkills(&store.Analysis{Match: m, Report: rep})
	checks := p.Categories[0].Checks
	if checks[0].State != "clear" || checks[1].State != "unknown" || checks[2].State != "attention" {
		t.Fatalf("incorrect states: %+v", checks)
	}
}

func TestScoreboardGroupsOwnTeamFirstAndSortsKills(t *testing.T) {
	rows := []scoreboardView{{Name: "enemy", Group: "Opponents", Kills: 20}, {Name: "own-low", Group: "Your team", Kills: 2}, {Name: "own-high", Group: "Your team", Kills: 10}}
	sortScoreboard(rows)
	if rows[0].Name != "own-high" || rows[1].Name != "own-low" || rows[2].Name != "enemy" {
		t.Fatalf("wrong team/kill order: %+v", rows)
	}
}
