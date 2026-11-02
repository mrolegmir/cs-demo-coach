package coach

import (
	"encoding/json"
	"reflect"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/mrolegmir/cs-demo-coach/internal/detect"
)

func TestRulesLanguagesPreserveGrounding(t *testing.T) {
	for kind := range kinds {
		t.Run(string(kind), func(t *testing.T) {
			in := &Input{Rounds: 24, Party: []string{"player01", "player02"}, Evidence: map[string]Evidence{}}
			for _, player := range in.Party {
				id := "f-" + player
				in.Evidence[id] = Evidence{Kind: kind, Player: player}
				in.Stats = append(in.Stats, PlayerStats{Player: player, Issues: []Issue{{
					Kind: kind, Count: 10, Chances: 20, Rate: .5, LobbyRate: .1, StandsOut: true,
					Examples: []Example{{ID: id}}, Places: []Place{{Name: "A site", Count: 8}}, Placed: 9,
					Practice: &Practice{Verdict: "working", Since: "23.10.2026", RateBefore: .6, RateAfter: .5},
				}}})
			}
			var baseline *Advice
			for _, language := range []string{"", "ru", "en"} {
				in.Language = language
				a, err := (Rules{PerPlayer: 2}).Advise(t.Context(), in)
				if err != nil {
					t.Fatal(err)
				}
				if err := in.Validate(a); err != nil {
					t.Fatal(err)
				}
				if a.Language != Language(language) {
					t.Fatalf("language = %q", a.Language)
				}
				encoded, err := json.Marshal(a)
				if err != nil {
					t.Fatal(err)
				}
				if strings.Contains(string(encoded), "%!") {
					t.Fatalf("broken format arguments: %s", encoded)
				}
				f := a.Players[0].Focus[0]
				if language == "en" {
					if f.Title != kinds[kind].Title || !strings.Contains(f.Priority, "Priority 1 of") {
						t.Fatalf("English advice = %+v", f)
					}
				} else if f.Title != russianMessages[kinds[kind].Title] || !strings.Contains(f.Priority, "Приоритет 1 из") {
					t.Fatalf("Russian advice = %+v", f)
				}
				if baseline == nil {
					baseline = a
				} else {
					previous := baseline.Players[0].Focus[0]
					if f.Kind != previous.Kind || f.DrillID != previous.DrillID || !slices.Equal(f.Evidence, previous.Evidence) ||
						!slices.Equal(a.Team[0].Evidence, baseline.Team[0].Evidence) {
						t.Fatal("language changed evidence or drill selection")
					}
				}
			}
		})
	}
}

func TestRussianPracticeAndQuietReports(t *testing.T) {
	in := fixtureInput(t)
	in.Language = "ru"
	player, issue := standout(t, in)
	for _, verdict := range []string{"working", "not_yet", "no_baseline", "too_early"} {
		in.issue(player, issue.Kind).Practice = &Practice{Verdict: verdict, Since: "23.10.2026"}
		f := in.focus(*in.issue(player, issue.Kind))
		if strings.Contains(f.Why, "%!") || strings.Contains(f.Why, "You ") || strings.Contains(f.Why, "Your ") {
			t.Fatalf("practice %s: %s", verdict, f.Why)
		}
	}
	quiet := &Input{Party: []string{"player01"}, Stats: []PlayerStats{{Player: "player01", Issues: []Issue{
		{Kind: detect.KindUnusedUtility, Status: "resolved"},
	}}}}
	a, err := (LLM{Provider: &scripted{}}).Advise(t.Context(), quiet)
	if err != nil || a.Language != "ru" || len(a.Notes) != 3 || !strings.Contains(a.Notes[2], "больше не выделяется") {
		t.Fatalf("quiet Russian report: %+v, %v", a, err)
	}
}

func TestLLMOutputLanguageSurvivesRetry(t *testing.T) {
	for _, language := range []string{"", "en"} {
		in := fixtureInput(t)
		in.Language = language
		valid, err := (Rules{PerPlayer: 2}).Advise(t.Context(), in)
		if err != nil {
			t.Fatal(err)
		}
		valid.Language = "model-must-not-set-this"
		raw, err := json.Marshal(valid)
		if err != nil {
			t.Fatal(err)
		}
		p := &scripted{replies: []string{"null", string(raw)}}
		a, err := (LLM{Provider: p}).Advise(t.Context(), in)
		if err != nil {
			t.Fatal(err)
		}
		if a.Language != Language(language) || len(p.systems) != 2 || p.systems[0] != p.systems[1] {
			t.Fatalf("language lost during retry: %+v, %v", a, p.systems)
		}
		for i := range p.prompts {
			if !strings.Contains(p.prompts[i], `"language":"`+Language(language)+`"`) ||
				!strings.Contains(p.systems[i], outputLanguage(language)) {
				t.Fatal("language missing from model request")
			}
		}
	}
}

func TestRussianCatalogFormatsAndKinds(t *testing.T) {
	// Keep format verbs and their order identical so translations cannot swap
	// counts, percentages, player names or dates by mistake.
	verbs := regexp.MustCompile(`%(?:[0-9]*\.)?[0-9]*[sdf%]`)
	for source, translated := range russianMessages {
		if !reflect.DeepEqual(verbs.FindAllString(source, -1), verbs.FindAllString(translated, -1)) {
			t.Errorf("format arguments differ: %q -> %q", source, translated)
		}
	}
	for _, info := range kinds {
		for _, source := range []string{info.Title, info.Meaning, info.Chances, info.Why, info.Drill} {
			if russianMessages[source] == "" {
				t.Errorf("missing Russian kind text: %q", source)
			}
		}
	}
}
