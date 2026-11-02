package web

import (
	"encoding/xml"
	"strings"
	"testing"
	"time"

	"github.com/mrolegmir/cs-demo-coach/internal/history"
)

func TestTrendSVG(t *testing.T) {
	day := time.Date(2026, 10, 1, 20, 0, 0, 0, time.UTC)
	points := []history.TrendPoint{
		{Played: day, Rate: 0.6, Lobby: 0.2},
		{Played: day.AddDate(0, 0, 2), Rate: 0.5, Lobby: 0.25},
		{Played: day.AddDate(0, 0, 4), Rate: 0.3, Lobby: 0.2},
	}
	if got := trendSVG(points[:1], 3, time.Time{}); got != "" {
		t.Errorf("one point should draw nothing, got %s", got)
	}

	svg := string(trendSVG(points, 3, day.AddDate(0, 0, 1)))
	if err := xml.Unmarshal([]byte(svg), new(struct{})); err != nil {
		t.Fatalf("not well-formed: %v\n%s", err, svg)
	}
	for _, want := range []string{
		`class="lobby"`, `class="player"`,
		"3 matches to Oct 1: 60%, lobby 20%; 3 matches to Oct 5: 30%, lobby 20%",
		// Practice started between the first and second match.
		`<line class="practice" x1="70.0"`,
		// The highest rate sits near the top, a falling line ends lower.
		`points="3.0,6.9 70.0,`,
	} {
		if !strings.Contains(svg, want) {
			t.Errorf("chart is missing %q:\n%s", want, svg)
		}
	}
	if strings.Contains(string(trendSVG(points, 3, time.Time{})), "practice") {
		t.Error("no practice, no mark")
	}
	if strings.Contains(string(trendSVG(points, 3, day.AddDate(0, 0, -5))), "practice") {
		t.Error("practice before the whole line has no before/after to mark")
	}
}
