package web

import (
	"fmt"
	"html/template"
	"strings"
	"time"

	"github.com/mrolegmir/cs-demo-coach/internal/history"
)

// Trend chart size in CSS pixels.
const (
	trendW, trendH = 140, 36
	trendPad       = 3
)

// trendSVG draws an issue's rolling rate over window matches (accent)
// against the lobby's (dashed) as a small inline SVG, with a mark at the
// first match after practice started (zero if none). The mark is left out
// when practice began before the line does: there is no "before" to show.
// It returns "" for fewer than two points. The markup holds only numbers,
// dates and fixed text, so it is safe as HTML.
func trendSVG(points []history.TrendPoint, window int, practiceSince time.Time) template.HTML {
	if len(points) < 2 {
		return ""
	}
	top := 0.05
	for _, p := range points {
		top = max(top, p.Rate, p.Lobby)
	}
	top *= 1.15
	x := func(i int) float64 {
		return trendPad + float64(i)*(trendW-2*trendPad)/float64(len(points)-1)
	}
	y := func(r float64) float64 { return trendPad + (1-r/top)*(trendH-2*trendPad) }
	line := func(rate func(history.TrendPoint) float64) string {
		var sb strings.Builder
		for i, p := range points {
			fmt.Fprintf(&sb, "%.1f,%.1f ", x(i), y(rate(p)))
		}
		return strings.TrimSpace(sb.String())
	}

	first, last := points[0], points[len(points)-1]
	label := fmt.Sprintf("%d matches to %s: %.0f%%, lobby %.0f%%; %d matches to %s: %.0f%%, lobby %.0f%%",
		window, first.Played.Format("Jan 2"), first.Rate*100, first.Lobby*100,
		window, last.Played.Format("Jan 2"), last.Rate*100, last.Lobby*100)
	var sb strings.Builder
	fmt.Fprintf(&sb, `<svg class="trend" width="%d" height="%d" viewBox="0 0 %d %d" role="img" aria-label="%s"><title>%s</title>`,
		trendW, trendH, trendW, trendH, label, label)
	if !practiceSince.IsZero() && first.Played.Before(practiceSince) {
		for i, p := range points {
			if !p.Played.Before(practiceSince) {
				fmt.Fprintf(&sb, `<line class="practice" x1="%.1f" x2="%.1f" y1="0" y2="%d"/>`, x(i), x(i), trendH)
				break
			}
		}
	}
	fmt.Fprintf(&sb, `<polyline class="lobby" points="%s"/>`, line(func(p history.TrendPoint) float64 { return p.Lobby }))
	fmt.Fprintf(&sb, `<polyline class="player" points="%s"/>`, line(func(p history.TrendPoint) float64 { return p.Rate }))
	fmt.Fprintf(&sb, `<circle class="player" cx="%.1f" cy="%.1f" r="2.5"/>`, x(len(points)-1), y(last.Rate))
	sb.WriteString(`</svg>`)
	return template.HTML(sb.String()) //nolint:gosec // numbers and fixed text only
}
