package coach

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/mrolegmir/cs-demo-coach/internal/detect"
	"github.com/mrolegmir/cs-demo-coach/internal/drills"
	"github.com/mrolegmir/cs-demo-coach/internal/history"
	"github.com/mrolegmir/cs-demo-coach/internal/match"
)

// fixtureInput builds coach input from a committed match fixture, with the
// team that started as T as the party.
func fixtureInput(t *testing.T) *Input {
	t.Helper()
	m, err := match.LoadFile(filepath.Join("..", "..", "testdata", "matches", "arsham_infernocs2_1stSept23.json.gz"))
	if err != nil {
		t.Fatal(err)
	}
	var party []match.PlayerID
	for _, p := range m.Rounds[0].Players {
		if p.Side == match.SideT {
			party = append(party, p.Player)
		}
	}
	in := NewInput(m, detect.Run(m, detect.Default()), party)
	in.Language = "en"
	return in
}

// standout returns a party player with a standing-out issue and that issue.
func standout(t *testing.T, in *Input) (string, Issue) {
	t.Helper()
	for _, ps := range in.Stats {
		for _, is := range ps.Issues {
			if is.StandsOut {
				return ps.Player, is
			}
		}
	}
	t.Fatal("fixture has no standing-out issue")
	return "", Issue{}
}

func TestNewInput(t *testing.T) {
	in := fixtureInput(t)
	if len(in.Party) != 5 || len(in.Stats) != 5 {
		t.Fatalf("party = %v, stats for %d players", in.Party, len(in.Stats))
	}
	for _, ps := range in.Stats {
		for i, is := range ps.Issues {
			if is.Count == 0 {
				t.Errorf("%s: zero-count issue %s", ps.Player, is.Kind)
			}
			if len(is.Examples) > maxExamples {
				t.Errorf("%s %s: %d examples", ps.Player, is.Kind, len(is.Examples))
			}
			if i > 0 && is.excess() > ps.Issues[i-1].excess() {
				t.Errorf("%s: issues not sorted by excess", ps.Player)
			}
			for _, ex := range is.Examples {
				if f, ok := in.Evidence[ex.ID]; !ok || f.Kind != is.Kind {
					t.Errorf("%s: example %s does not match a %s finding", ps.Player, ex.ID, is.Kind)
				}
			}
		}
	}
}

func TestValidate(t *testing.T) {
	in := fixtureInput(t)
	player, is := standout(t, in)
	good := Focus{Kind: is.Kind, Title: "t", Why: "w", DrillID: drillFor(is.Kind), Drill: "d", Evidence: []string{is.Examples[0].ID}}

	// someone else's finding of any kind.
	var foreign string
	for id, f := range in.Evidence {
		if !f.involves(player) {
			foreign = id
			break
		}
	}

	// an issue this player has that does not stand out, if any.
	var quiet Focus
	for _, ps := range in.Stats {
		for _, other := range ps.Issues {
			if ps.Player == player && !other.StandsOut {
				quiet = Focus{Kind: other.Kind, Title: "t", Why: "w", DrillID: drillFor(other.Kind), Drill: "d", Evidence: []string{other.Examples[0].ID}}
			}
		}
	}

	tests := []struct {
		name    string
		advice  Advice
		wantErr string
	}{
		{name: "grounded", advice: Advice{Players: []PlayerAdvice{{Player: player, Focus: []Focus{good}}}}},
		{name: "empty advice is valid", advice: Advice{}},
		{
			name:    "unknown player",
			advice:  Advice{Players: []PlayerAdvice{{Player: "s1mple", Focus: []Focus{good}}}},
			wantErr: "not in the party",
		},
		{
			name:    "too many focus areas",
			advice:  Advice{Players: []PlayerAdvice{{Player: player, Focus: []Focus{good, good, good, good}}}},
			wantErr: "max 3",
		},
		{
			name: "no evidence",
			advice: Advice{Players: []PlayerAdvice{{Player: player, Focus: []Focus{
				{Kind: is.Kind, Title: "t", Why: "w", DrillID: drillFor(is.Kind), Drill: "d"},
			}}}},
			wantErr: "evidence is required",
		},
		{
			name: "invented finding",
			advice: Advice{Players: []PlayerAdvice{{Player: player, Focus: []Focus{
				{Kind: is.Kind, Title: "t", Why: "w", DrillID: drillFor(is.Kind), Drill: "d", Evidence: []string{is.Examples[0].ID, "f9999"}},
			}}}},
			wantErr: `unknown finding "f9999"`,
		},
		{
			name: "someone else's finding",
			advice: Advice{Players: []PlayerAdvice{{Player: player, Focus: []Focus{
				{Kind: is.Kind, Title: "t", Why: "w", DrillID: drillFor(is.Kind), Drill: "d", Evidence: []string{is.Examples[0].ID, foreign}},
			}}}},
			wantErr: "about someone else",
		},
		{
			name: "missing text",
			advice: Advice{Players: []PlayerAdvice{{Player: player, Focus: []Focus{
				{Kind: is.Kind, Title: "t", Evidence: []string{is.Examples[0].ID}},
			}}}},
			wantErr: "title, why and drill are required",
		},
		{
			name: "no drill",
			advice: Advice{Players: []PlayerAdvice{{Player: player, Focus: []Focus{
				{Kind: is.Kind, Title: "t", Why: "w", Drill: "d", Evidence: []string{is.Examples[0].ID}},
			}}}},
			wantErr: "drill_id is required",
		},
		{
			name: "invented drill",
			advice: Advice{Players: []PlayerAdvice{{Player: player, Focus: []Focus{
				{Kind: is.Kind, Title: "t", Why: "w", DrillID: "jump-throw-everything", Drill: "d", Evidence: []string{is.Examples[0].ID}},
			}}}},
			wantErr: `unknown drill_id "jump-throw-everything"`,
		},
		{
			name: "drill for another mistake",
			advice: Advice{Players: []PlayerAdvice{{Player: player, Focus: []Focus{
				{Kind: is.Kind, Title: "t", Why: "w", DrillID: otherDrill(is.Kind), Drill: "d", Evidence: []string{is.Examples[0].ID}},
			}}}},
			wantErr: "is for",
		},
		{
			name:    "team advice without evidence",
			advice:  Advice{Team: []TeamAdvice{{Title: "t", Why: "w", Suggestion: "s"}}},
			wantErr: "team advice 1: evidence is required",
		},
	}
	if quiet.Kind != "" {
		tests = append(tests, struct {
			name    string
			advice  Advice
			wantErr string
		}{"issue does not stand out", Advice{Players: []PlayerAdvice{{Player: player, Focus: []Focus{quiet}}}}, "does not stand out"})
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := in.Validate(&tt.advice)
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("error = %v, want it to contain %q", err, tt.wantErr)
			}
		})
	}
}

func TestRulesAdviceIsValid(t *testing.T) {
	in := fixtureInput(t)
	a, err := Rules{PerPlayer: 2}.Advise(t.Context(), in)
	if err != nil {
		t.Fatal(err)
	}
	if err := in.Validate(a); err != nil {
		t.Fatalf("rules coach produced invalid advice: %v", err)
	}
	focus := 0
	for _, pa := range a.Players {
		if len(pa.Focus) > 2 {
			t.Errorf("%s: %d focus areas, want at most 2", pa.Player, len(pa.Focus))
		}
		focus += len(pa.Focus)
	}
	if focus == 0 {
		t.Error("no focus areas at all")
	}
}

// scripted is a Provider that returns canned replies in order.
type scripted struct {
	replies []string
	prompts []string
	systems []string
}

func (s *scripted) Complete(_ context.Context, system, user string) (string, error) {
	s.prompts = append(s.prompts, user)
	s.systems = append(s.systems, system)
	if len(s.replies) == 0 {
		return "", errors.New("no more replies")
	}
	r := s.replies[0]
	s.replies = s.replies[1:]
	return r, nil
}

// drillFor returns the first library drill for a kind.
func drillFor(k detect.Kind) string { return drills.ForKind(k)[0].ID }

// otherDrill returns a library drill for some other kind.
func otherDrill(k detect.Kind) string {
	for _, d := range drills.All() {
		if d.Kind != k {
			return d.ID
		}
	}
	return ""
}

// TestPracticedDrill checks that a drill that is not working is replaced:
// Validate rejects it and the rule-based coach skips it.
func TestPracticedDrill(t *testing.T) {
	in := fixtureInput(t)
	player, is := standout(t, in)
	tried := drillFor(is.Kind)
	for i := range in.Stats {
		for j := range in.Stats[i].Issues {
			if in.Stats[i].Player == player && in.Stats[i].Issues[j].Kind == is.Kind {
				in.Stats[i].Issues[j].Practice = &Practice{Verdict: "not_yet", Drills: []string{tried}}
			}
		}
	}
	same := Advice{Players: []PlayerAdvice{{Player: player, Focus: []Focus{
		{Kind: is.Kind, Title: "t", Why: "w", DrillID: tried, Drill: "d", Evidence: []string{is.Examples[0].ID}},
	}}}}
	if err := in.Validate(&same); err == nil || !strings.Contains(err.Error(), "pick another drill") {
		t.Errorf("repeating a drill that is not working: err = %v", err)
	}

	// With every drill tried, repeating one is the only option and is fine.
	all := in.issue(player, is.Kind).Practice
	all.Drills = nil
	for _, d := range drills.ForKind(is.Kind) {
		all.Drills = append(all.Drills, d.ID)
	}
	if err := in.Validate(&same); err != nil {
		t.Errorf("every drill tried: %v", err)
	}
	all.Drills = []string{tried}

	a, err := Rules{PerPlayer: 3}.Advise(t.Context(), in)
	if err != nil {
		t.Fatal(err)
	}
	if err := in.Validate(a); err != nil {
		t.Fatalf("rules advice is invalid: %v", err)
	}
	for _, pa := range a.Players {
		for _, f := range pa.Focus {
			if pa.Player == player && f.Kind == is.Kind && f.DrillID == tried {
				t.Errorf("rules coach repeated %q", tried)
			}
		}
	}
}

func TestLLM(t *testing.T) {
	in := fixtureInput(t)
	player, is := standout(t, in)
	valid, err := json.Marshal(Advice{Players: []PlayerAdvice{{Player: player, Focus: []Focus{
		{Kind: is.Kind, Title: "t", Why: "w", DrillID: drillFor(is.Kind), Drill: "d", Evidence: []string{is.Examples[0].ID}},
	}}}})
	if err != nil {
		t.Fatal(err)
	}
	invented := `{"players":[{"player":"` + player + `","focus":[{"kind":"` + string(is.Kind) + `","title":"t","why":"w","drill":"d","evidence":["f9999"]}]}]}`

	t.Run("valid first reply", func(t *testing.T) {
		p := &scripted{replies: []string{"```json\n" + string(valid) + "\n```"}}
		a, err := LLM{Provider: p}.Advise(t.Context(), in)
		if err != nil {
			t.Fatal(err)
		}
		if len(p.prompts) != 1 || a.Players[0].Player != player {
			t.Fatalf("prompts = %d, advice = %+v", len(p.prompts), a)
		}
		if !strings.Contains(p.prompts[0], `"stands_out"`) || !strings.Contains(p.prompts[0], `"kinds"`) {
			t.Error("prompt is missing stats or kind descriptions")
		}
		if !strings.Contains(p.prompts[0], `"id":"`+drillFor(is.Kind)+`"`) {
			t.Error("prompt is missing the drills for a kind that stands out")
		}
	})

	t.Run("retries with the validation errors", func(t *testing.T) {
		p := &scripted{replies: []string{invented, string(valid)}}
		if _, err := (LLM{Provider: p}).Advise(t.Context(), in); err != nil {
			t.Fatal(err)
		}
		if len(p.prompts) != 2 || !strings.Contains(p.prompts[1], `unknown finding "f9999"`) {
			t.Fatalf("retry prompt does not explain the error:\n%s", p.prompts[len(p.prompts)-1])
		}
	})

	t.Run("gives up after a second invalid reply", func(t *testing.T) {
		p := &scripted{replies: []string{invented, "not json"}}
		_, err := LLM{Provider: p}.Advise(t.Context(), in)
		if !errors.Is(err, ErrUngrounded) {
			t.Fatalf("error = %v, want ErrUngrounded", err)
		}
	})
}

func TestProviders(t *testing.T) {
	tests := []struct {
		name     string
		path     string
		response string
		provider func(url string) Provider
	}{
		{
			name:     "openai",
			path:     "/v1/chat/completions",
			response: `{"choices":[{"message":{"role":"assistant","content":"{\"ok\":true}"}}]}`,
			provider: func(url string) Provider { return OpenAI{BaseURL: url + "/v1", Model: "m", APIKey: "k"} },
		},
		{
			name:     "ollama",
			path:     "/api/chat",
			response: `{"message":{"role":"assistant","content":"{\"ok\":true}"}}`,
			provider: func(url string) Provider { return Ollama{BaseURL: url, Model: "m"} },
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got map[string]any
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != tt.path {
					http.NotFound(w, r)
					return
				}
				body, _ := io.ReadAll(r.Body)
				_ = json.Unmarshal(body, &got)
				_, _ = io.WriteString(w, tt.response)
			}))
			defer srv.Close()

			reply, err := tt.provider(srv.URL).Complete(t.Context(), "sys", "usr")
			if err != nil {
				t.Fatal(err)
			}
			if reply != `{"ok":true}` {
				t.Errorf("reply = %q", reply)
			}
			msgs, _ := got["messages"].([]any)
			if got["model"] != "m" || len(msgs) != 2 {
				t.Errorf("request = %v", got)
			}
		})
	}

	t.Run("api error", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = io.WriteString(w, `{"error":{"message":"bad key"}}`)
		}))
		defer srv.Close()
		_, err := OpenAI{BaseURL: srv.URL, Model: "m"}.Complete(t.Context(), "s", "u")
		if err == nil || !strings.Contains(err.Error(), "bad key") {
			t.Fatalf("error = %v", err)
		}
	})
}

func TestPlacesInInput(t *testing.T) {
	in := fixtureInput(t)
	var placed, withPlace int
	for _, ps := range in.Stats {
		for _, is := range ps.Issues {
			if is.Kind != detect.KindIsolatedDeath {
				continue
			}
			if len(is.Places) == 0 || is.Places[0].Count < is.Places[len(is.Places)-1].Count {
				t.Errorf("%s: places = %+v", ps.Player, is.Places)
			}
			placed++
			for _, ex := range is.Examples {
				if ex.Place != "" {
					withPlace++
				}
			}
		}
	}
	if placed == 0 || withPlace == 0 {
		t.Fatalf("no isolated deaths with places (%d issues, %d examples)", placed, withPlace)
	}
	if _, ok := mainPlace(Issue{Count: 9, Placed: 9, Places: []Place{{"B site", 2}}}); ok {
		t.Error("2 of 9 is not where an issue clusters")
	}
	// Only findings with a place count: older matches have none.
	if p, ok := mainPlace(Issue{Count: 20, Placed: 5, Places: []Place{{"B site", 3}}}); !ok || p.Name != "B site" {
		t.Error("3 of 5 placed is where it clusters")
	}
}

func TestPairAdvice(t *testing.T) {
	in := &Input{
		Language: "en",
		Party:    []string{"anna", "bob"},
		Evidence: map[string]Evidence{
			"f1": {Kind: detect.KindMissedTrade, Player: "bob", Related: []string{"anna"}, Round: 3},
			"f2": {Kind: detect.KindMissedTrade, Player: "anna", Related: []string{"bob"}, Round: 5},
		},
		Pairs: []Pair{
			NewPair("bob", "anna", 5, 3),  // traded often enough
			NewPair("anna", "bob", 6, 1),  // the one to fix
			NewPair("anna", "carl", 2, 0), // too few to judge
		},
	}
	tm, ok := pairAdvice(in)
	if !ok || tm.Title != "Team: bob, trade anna" || !slices.Equal(tm.Evidence, []string{"f1"}) ||
		!strings.Contains(tm.Why, "1 of 6") {
		t.Fatalf("pair advice = %+v, %v", tm, ok)
	}
	if err := in.Validate(&Advice{Team: []TeamAdvice{tm}}); err != nil {
		t.Errorf("pair advice is not grounded: %v", err)
	}
	in.Language = "ru"
	russian, ok := pairAdvice(in)
	if !ok || russian.Title != "Команда: bob, разменивай anna" ||
		!strings.Contains(russian.Why, "1 из 6") || !slices.Equal(russian.Evidence, tm.Evidence) {
		t.Fatalf("Russian pair advice = %+v", russian)
	}
	delete(in.Evidence, "f1")
	if _, ok := pairAdvice(in); ok {
		t.Error("pair advice without a missed trade to cite")
	}
}

func TestPairsInInput(t *testing.T) {
	in := fixtureInput(t)
	for _, p := range in.Pairs {
		if !slices.Contains(in.Party, p.Player) || !slices.Contains(in.Party, p.Partner) {
			t.Errorf("pair %+v is not within the party", p)
		}
	}
	if len(in.Pairs) == 0 {
		t.Error("no pairs for the party")
	}
}

func TestEntryAdvice(t *testing.T) {
	in := &Input{
		Language: "en",
		Party:    []string{"anna", "bob", "carl"},
		Stats:    []PlayerStats{{Player: "anna", Issues: []Issue{{Kind: detect.KindOpeningDeath, StandsOut: true}}}},
		Evidence: map[string]Evidence{
			"m2-f9": {Kind: detect.KindOpeningDeath, Player: "anna", Match: 2, Round: 9},
			"m1-f3": {Kind: detect.KindOpeningDeath, Player: "anna", Match: 1, Round: 3},
		},
		Roles: []Role{
			{Player: "anna", Labels: []string{history.Entry}, OpeningRate: 0.4, LobbyOpeningRate: 0.2},
			{Player: "bob", Flashes10: 0.5, LobbyFlashes10: 1},
			{Player: "carl", Flashes10: 1.5, LobbyFlashes10: 1},
		},
	}
	tm, ok := entryAdvice(in)
	if !ok || tm.Title != "Team: carl, flash for anna" || !slices.Equal(tm.Evidence, []string{"m1-f3", "m2-f9"}) {
		t.Fatalf("entry advice = %+v, %v", tm, ok)
	}
	if !strings.Contains(tm.Suggestion, "bob follows close enough to trade") {
		t.Fatalf("three-player plan = %q", tm.Suggestion)
	}
	if err := in.Validate(&Advice{Team: []TeamAdvice{tm}}); err != nil {
		t.Errorf("entry advice is not grounded: %v", err)
	}
	in.Language = "ru"
	russian, ok := entryAdvice(in)
	if !ok || russian.Title != "Команда: carl, дай флешку для игрока anna" ||
		!strings.Contains(russian.Suggestion, "bob следует достаточно близко для размена") ||
		strings.Contains(russian.Why, "%!") || !slices.Equal(russian.Evidence, tm.Evidence) {
		t.Fatalf("Russian entry advice = %+v", russian)
	}
	in.Roles[2].Flashes10 = 0.9 // below the lobby: nobody is good enough to ask
	if _, ok := entryAdvice(in); ok {
		t.Error("asked a teammate who flashes below the lobby")
	}
	in.Roles[2].Flashes10 = 1.5
	in.Stats[0].Issues[0].StandsOut = false
	if _, ok := entryAdvice(in); ok {
		t.Error("advice for an entry whose first deaths do not stand out")
	}
}
