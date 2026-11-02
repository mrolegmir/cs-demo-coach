package telegram

import (
	"strings"
	"testing"
	"time"

	"github.com/mrolegmir/cs-demo-coach/internal/coach"
	"github.com/mrolegmir/cs-demo-coach/internal/store"
)

func TestBriefTextUsesStoredLanguage(t *testing.T) {
	br := &store.Brief{
		ID: 7, PeriodStart: time.Date(2026, 10, 23, 0, 0, 0, 0, time.UTC),
		PeriodEnd: time.Date(2026, 10, 30, 0, 0, 0, 0, time.UTC),
		Advice: &coach.Advice{
			Players: []coach.PlayerAdvice{{Player: "player01", Focus: []coach.Focus{{Title: "Разменивай напарников"}}}},
			Team:    []coach.TeamAdvice{{Title: "Играйте в паре"}},
		},
	}
	for _, language := range []string{"", "en", "ru"} {
		br.Advice.Language = language
		text := Text(br, "https://coach.example")
		want := "Your weekly brief"
		if language == "ru" {
			want = "Твой недельный разбор за 23.10.2026 – 30.10.2026 готов."
			if !strings.Contains(text, "\nКоманда\n") || !strings.Contains(text, "Объяснения, эпизоды и упражнения:") {
				t.Fatalf("Russian message = %s", text)
			}
		}
		if !strings.HasPrefix(text, want) || !strings.Contains(text, "• Разменивай напарников") ||
			!strings.HasSuffix(text, "https://coach.example/briefs/7") {
			t.Fatalf("message %q = %s", language, text)
		}
	}
}
