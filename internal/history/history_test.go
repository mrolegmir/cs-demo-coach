package history

import (
	"math"
	"slices"
	"testing"
	"time"

	"github.com/mrolegmir/cs-demo-coach/internal/detect"
)

const kind = detect.KindNoUtility

// series builds matches where the player made count[i] mistakes out of 10
// chances in match i, against a lobby that makes 2 out of 10.
func series(counts ...int) []Match {
	start := time.Date(2026, 9, 1, 20, 0, 0, 0, time.UTC)
	var ms []Match
	for i, c := range counts {
		ms = append(ms, Match{
			MatchID: int64(i + 1),
			Played:  start.Add(time.Duration(i) * 24 * time.Hour),
			Player:  map[detect.Kind]Counts{kind: {Count: c, Chances: 10}},
			Lobby:   map[detect.Kind]Counts{kind: {Count: 20, Chances: 100}},
		})
	}
	return ms
}

func TestStatus(t *testing.T) {
	tests := []struct {
		name   string
		counts []int
		want   Status
	}{
		{"habit that persists", []int{6, 5, 7, 6, 5, 6}, Active},
		{"habit in only a couple of matches so far", []int{6, 7}, Active},
		{"habit fading: still above lobby overall, not recently", []int{8, 8, 8, 8, 3, 2, 3}, Improving},
		{"habit gone: earlier habit, now at lobby level", []int{6, 7, 6, 2, 1, 2}, Resolved},
		{"one bad game", []int{2, 1, 8, 2, 2, 1}, OneOff},
		{"never an issue", []int{2, 2, 1, 3, 2, 2}, Fine},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			issues := Default().Build(series(tt.counts...))
			if len(issues) != 1 {
				t.Fatalf("issues = %d", len(issues))
			}
			if got := issues[0].Status; got != tt.want {
				is := issues[0]
				t.Fatalf("status = %s, want %s (total %d/%d, recent %d/%d, stood out %d)",
					got, tt.want, is.Total.Count, is.Total.Chances, is.Recent.Count, is.Recent.Chances, is.StoodOut)
			}
		})
	}
}

func TestNotEnoughData(t *testing.T) {
	ms := series(1)
	ms[0].Player[kind] = Counts{Count: 1, Chances: 2}
	if got := Default().Build(ms)[0].Status; got != NotEnoughData {
		t.Fatalf("status = %s", got)
	}
}

func TestBuildOrdersAndTotals(t *testing.T) {
	ms := series(6, 6, 6, 6)
	// A second kind that is fine.
	for i := range ms {
		ms[i].Player[detect.KindTeamFlash] = Counts{Count: 0, Chances: 5}
		ms[i].Lobby[detect.KindTeamFlash] = Counts{Count: 10, Chances: 50}
	}
	// Out of order input is sorted by time.
	ms[0], ms[3] = ms[3], ms[0]

	issues := Default().Build(ms)
	if len(issues) != 2 || issues[0].Kind != kind || issues[1].Kind != detect.KindTeamFlash {
		t.Fatalf("order = %v", issues)
	}
	is := issues[0]
	if is.Total != (Counts{24, 40}) || is.Recent != (Counts{18, 30}) || is.Lobby != (Counts{80, 400}) {
		t.Errorf("totals = %+v recent %+v lobby %+v", is.Total, is.Recent, is.Lobby)
	}
	if is.PerMatch[0].MatchID != 1 || is.PerMatch[3].MatchID != 4 || is.StoodOut != 4 {
		t.Errorf("per match = %+v, stood out %d", is.PerMatch, is.StoodOut)
	}
}

func TestPracticeWithoutBaseline(t *testing.T) {
	is := Default().Build(series(6, 3, 3, 2))[0]
	p, ok := is.Practiced([]time.Time{time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)})
	if !ok || p.MatchesAfter != 4 || p.Verdict() != NoBaseline {
		t.Fatalf("practice before every match = %+v, verdict %s; want no_baseline", p, p.Verdict())
	}
}

func TestPracticeEffect(t *testing.T) {
	// Practice starts before the fourth match (matches are a day apart).
	practiced := time.Date(2026, 9, 3, 22, 0, 0, 0, time.UTC)
	tests := []struct {
		name   string
		counts []int
		want   string
	}{
		{"fewer mistakes since practice", []int{6, 6, 6, 3, 3, 2}, "working"},
		{"no change", []int{6, 6, 6, 6, 5, 6}, "not_yet"},
		{"only two matches since", []int{6, 6, 6, 2, 2}, "too_early"},
		{"was fine, got worse", []int{1, 1, 1, 2, 2, 2}, "not_yet"},
		{"was fine, still fine", []int{1, 1, 1, 1, 0, 1}, "working"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			is := Default().Build(series(tt.counts...))[0]
			e := is.Since(practiced)
			if e.Before.Chances != 30 || e.MatchesAfter != len(tt.counts)-3 {
				t.Fatalf("effect = %+v, want three matches before", e)
			}
			if got := e.Verdict(); got != tt.want {
				t.Fatalf("verdict = %s, want %s (before %.0f%%, after %.0f%%)", got, tt.want, e.Before.Rate()*100, e.After.Rate()*100)
			}
		})
	}
}

func TestTrend(t *testing.T) {
	ms := series(8, 6, 4, 2)
	ms[1].Player[kind] = Counts{} // no chances in match 2
	is := Default().Build(ms)[0]
	tr := is.Trend(2)
	// The first point is the first full window, matches 1 and 2.
	want := []float64{0.8, 0.4, 0.3}
	if len(tr) != len(want) {
		t.Fatalf("trend = %+v", tr)
	}
	for i, p := range tr {
		if math.Abs(p.Rate-want[i]) > 1e-9 || math.Abs(p.Lobby-0.2) > 1e-9 || p.MatchID != int64(i+2) {
			t.Errorf("point %d = %+v, want rate %.1f against 0.2", i, p, want[i])
		}
	}
	if got := is.Trend(0); len(got) != 3 {
		t.Errorf("a window below 1 should mean one match (match 2 has no chances): %+v", got)
	}
	if got := is.Trend(5); len(got) != 0 {
		t.Errorf("a window longer than the history has no full point: %+v", got)
	}
}

func TestPlaces(t *testing.T) {
	ms := series(3, 3, 3)
	ms[0].Places = map[detect.Kind][]PlaceCount{kind: {{"de_inferno", "Banana", 2}, {"de_inferno", "BombsiteB", 1}}}
	ms[1].Places = map[detect.Kind][]PlaceCount{kind: {{"de_inferno", "BombsiteB", 2}, {"de_inferno", "Arch", 1}}}
	// The same name on another map is another place.
	ms[2].Places = map[detect.Kind][]PlaceCount{kind: {{"de_mirage", "BombsiteB", 2}}}
	got := Default().Build(ms)[0].Places
	want := []PlaceCount{
		{"de_inferno", "BombsiteB", 3}, {"de_inferno", "Banana", 2}, {"de_mirage", "BombsiteB", 2}, {"de_inferno", "Arch", 1},
	}
	if !slices.Equal(got, want) {
		t.Errorf("places = %+v, want %+v", got, want)
	}
}

func TestPartners(t *testing.T) {
	const me, anna, bob = 1, 2, 3
	ms := []Match{
		{Pairs: []detect.Pair{{Player: me, Partner: anna, Chances: 3, Traded: 2}, {Player: anna, Partner: me, Chances: 2, Traded: 2}}},
		{Pairs: []detect.Pair{{Player: me, Partner: anna, Chances: 1, Traded: 0}, {Player: bob, Partner: me, Chances: 6, Traded: 1}}},
	}
	got := Partners(me, ms)
	want := []Partner{
		// Six shared deaths each: ties go by ID.
		{ID: anna, Covered: Counts{Count: 2, Chances: 4}, Covering: Counts{Count: 2, Chances: 2}},
		{ID: bob, Covering: Counts{Count: 1, Chances: 6}},
	}
	if !slices.Equal(got, want) {
		t.Errorf("partners = %+v, want %+v", got, want)
	}
}

func TestRoles(t *testing.T) {
	lobby := detect.Role{Rounds: 200, Openings: 40, OpeningWins: 20, Kills: 140, GoodFlashes: 40}
	m := func(r detect.Role) Match { return Match{Role: r, LobbyRole: lobby} }
	entry := Roles([]Match{
		m(detect.Role{Rounds: 20, Openings: 7, OpeningWins: 3, Kills: 15}),
		m(detect.Role{Rounds: 20, Openings: 6, OpeningWins: 4, Kills: 14, AWPKills: 1}),
	})
	if !slices.Equal(entry.Labels, []string{Entry}) || math.Abs(entry.OpeningRate()-13.0/40) > 1e-9 {
		t.Errorf("entry = %+v, opening rate %.2f", entry.Labels, entry.OpeningRate())
	}
	support := Roles([]Match{m(detect.Role{Rounds: 40, Openings: 4, Kills: 20, GoodFlashes: 14})})
	if !slices.Equal(support.Labels, []string{Support}) {
		t.Errorf("support labels = %v", support.Labels)
	}
	awp := Roles([]Match{m(detect.Role{Rounds: 20, Openings: 4, Kills: 20, AWPKills: 12})})
	if !slices.Equal(awp.Labels, []string{AWPer}) {
		t.Errorf("awper labels = %v (too few rounds for the others)", awp.Labels)
	}
}
