package web

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/mrolegmir/cs-demo-coach/internal/match"
	"github.com/mrolegmir/cs-demo-coach/internal/store"
)

func TestResultFollowsSideSwitches(t *testing.T) {
	rounds := []match.Round{
		{Number: 1, Winner: match.SideT, Players: []match.PlayerRound{{Player: 1, Side: match.SideT}}},
		{Number: 2, Winner: match.SideCT, Players: []match.PlayerRound{{Player: 1, Side: match.SideCT}}},
		{Number: 3, Winner: match.SideCT, Players: []match.PlayerRound{{Player: 1, Side: match.SideT}}},
		{Number: 4, Winner: match.SideT, Players: []match.PlayerRound{{Player: 1, Side: match.SideCT}}},
	}
	rounds[0].Kills = []match.Kill{{Killer: 1, Victim: 2}, {Killer: 1, Victim: 3, TeamKill: true}}
	v := resultFor(rounds, 1)
	if !v.Known || v.Won != 2 || v.Lost != 2 || v.TWon != 1 || v.CTWon != 1 || v.Outcome != "Draw" || v.Rounds[0].Kills != 1 {
		t.Fatalf("wrong result across side switches: %+v", v)
	}
	for _, id := range []match.PlayerID{0, 99} {
		if resultFor(rounds, id).Known {
			t.Fatal("invented a result for a non-participant")
		}
	}
	rounds[3].Players = nil
	if resultFor(rounds, 1).Known {
		t.Fatal("partial perspective presented as a complete result")
	}
}

func TestScoreboardExcludesTeamKillsAndSuicides(t *testing.T) {
	m := &match.Match{Players: []match.Player{{ID: 1, Name: "player01"}}, Rounds: []match.Round{{
		Kills: []match.Kill{
			{Killer: 1, Victim: 2, Headshot: true},
			{Killer: 1, Victim: 3, TeamKill: true},
			{Killer: 1, Victim: 1},
			{Killer: 2, Victim: 3, Assister: 1},
		},
		Grenades: []match.Grenade{{Thrower: 1, Type: "Flashbang"}, {Thrower: 1, Type: "Smoke Grenade"}},
	}}}
	v := scoreboard(m, 1)[0]
	if !v.Self || v.Kills != 1 || v.Deaths != 1 || v.Assists != 1 || v.Headshots != 1 || v.Flashes != 1 {
		t.Fatalf("wrong scoreboard: %+v", v)
	}
}

func TestPlayerFirstMatchPage(t *testing.T) {
	st := coachedStore(t)
	s, _ := newServer(t, st)
	u := &store.User{SteamID: st.m.Party[0], Name: st.analysis.Match.PlayerName(st.m.Party[0])}
	st.m.UploadedBy = u.SteamID
	st.m.UploadedAt = time.Date(2026, 10, 30, 18, 0, 0, 0, time.UTC)
	p := matchPage{Viewer: Viewer{User: u}, Match: st.m}
	p.build(st.analysis)
	if len(p.Players) == 0 || !p.Players[0].Self {
		t.Fatal("personal recommendations must precede teammate advice")
	}
	r := httptest.NewRequest(http.MethodGet, "/matches/7?diagnostics=1", http.NoBody)
	r.SetPathValue("id", "7")
	w := httptest.NewRecorder()
	s.match(w, r, u)
	body := w.Body.String()
	if w.Code != http.StatusOK {
		t.Fatalf("render failed: %d", w.Code)
	}
	for _, unwanted := range []string{st.m.Filename, "80 units", "80 единиц", "New recommendations use", "Diagnostics", "Диагностика"} {
		if strings.Contains(body, unwanted) {
			t.Errorf("player page contains %q", unwanted)
		}
	}
	if !strings.Contains(body, "Рекомендации") || !strings.Contains(body, "Ход матча") || !strings.Contains(body, "Изменить состав") {
		t.Fatal("missing player-facing content")
	}
	if strings.Index(body, "Рекомендации") > strings.Index(body, "Ход матча") {
		t.Fatal("statistics precede recommendations")
	}
	if dir := os.Getenv("CSDC_PREVIEW_DIR"); dir != "" {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "match.html"), w.Body.Bytes(), 0o600); err != nil {
			t.Fatal(err)
		}
	}
}

func TestPartyDefaultsToViewer(t *testing.T) {
	st := coachedStore(t)
	id := st.m.Party[0]
	p := matchPage{Viewer: Viewer{User: &store.User{SteamID: id}}, Match: &store.Match{}}
	p.build(st.analysis)
	selected := 0
	for _, team := range p.Teams {
		for _, player := range team.Players {
			if player.InParty {
				selected++
				if player.ID != id {
					t.Fatal("automatically selected another player without party metadata")
				}
			}
		}
	}
	if selected != 1 {
		t.Fatalf("selected %d players, want only the viewer", selected)
	}
}

func TestPlayerFirstHome(t *testing.T) {
	s, _ := newServer(t, coachedStore(t))
	data := struct {
		Viewer
		Matches            []matchListView
		Error, BriefNotice string
		Briefs             []store.Brief
		Quota              *quotaView
		Telegram           *telegramView
	}{
		Viewer:   Viewer{User: &store.User{Name: "player01", SteamID: 1}},
		Matches:  []matchListView{{Match: store.Match{ID: 7, Map: "Mirage", Filename: "private-demo-id.dem", UploadedAt: time.Date(2026, 10, 30, 18, 0, 0, 0, time.UTC), Status: store.StatusCoached}, Result: resultView{Known: true, Won: 13, Lost: 9, Outcome: "Victory", Symbol: "✓"}}},
		Telegram: &telegramView{Linked: true},
	}
	w := httptest.NewRecorder()
	s.render(w, httptest.NewRequest(http.MethodGet, "/", http.NoBody), "index", data)
	body := w.Body.String()
	last := -1
	for _, label := range []string{"Последние матчи", "Недельный разбор", "Настройки и импорт", "Загрузить демо вручную"} {
		i := strings.Index(body, label)
		if i <= last {
			t.Fatalf("home page order is wrong at %q", label)
		}
		last = i
	}
	if !strings.Contains(body, "13 : 9") || strings.Contains(body, "private-demo-id") {
		t.Fatal("home does not show a readable match result")
	}
	if dir := os.Getenv("CSDC_PREVIEW_DIR"); dir != "" {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "index.html"), w.Body.Bytes(), 0o600); err != nil {
			t.Fatal(err)
		}
	}
}
