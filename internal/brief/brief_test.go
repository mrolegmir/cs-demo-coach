package brief

import (
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/mrolegmir/cs-demo-coach/internal/coach"
	"github.com/mrolegmir/cs-demo-coach/internal/detect"
	"github.com/mrolegmir/cs-demo-coach/internal/match"
	"github.com/mrolegmir/cs-demo-coach/internal/store"
)

var week = time.Date(2026, 10, 23, 15, 0, 0, 0, time.UTC) // a Friday

// fixtureMatches loads the committed matches as one player's history, one
// per day before week+7d. Anonymized fixtures number players 1..10 in each
// match, so the same IDs act as a party that plays together.
func fixtureMatches(t *testing.T) []store.PlayerMatch {
	t.Helper()
	paths, err := filepath.Glob(filepath.Join("..", "..", "testdata", "matches", "*.json.gz"))
	if err != nil || len(paths) == 0 {
		t.Fatal("no fixtures")
	}
	var out []store.PlayerMatch
	for i, p := range paths {
		m, err := match.LoadFile(p)
		if err != nil {
			t.Fatal(err)
		}
		pm := store.PlayerMatch{
			ID:     int64(i + 1),
			Played: week.Add(time.Duration(i+1) * 24 * time.Hour),
			Map:    m.Map,
			Report: detect.Run(m, detect.Default()),
			Names:  map[match.PlayerID]string{},
			Sides:  map[match.PlayerID]match.Side{},
		}
		for _, pl := range m.Players {
			pm.Names[pl.ID] = pl.Name
			pm.Sides[pl.ID] = m.StartSide(pl.ID)
		}
		out = append(out, pm)
	}
	return out
}

func TestParty(t *testing.T) {
	ms := fixtureMatches(t)
	party := Party(1, ms, week, week.Add(Period), []match.PlayerID{2, 3, 4, 5, 6, 7, 8, 9, 10})
	if len(party) < 2 || party[0] != 1 {
		t.Fatalf("party = %v, want the owner and regular teammates", party)
	}
	// Every selected friend shared the owner's side in at least one match.
	for _, p := range party[1:] {
		n := 0
		for _, pm := range ms {
			if pm.Sides[p] == pm.Sides[1] {
				n++
			}
		}
		if n == 0 {
			t.Errorf("player %d was a teammate in only %d of %d matches", p, n, len(ms))
		}
	}
	if got := Party(1, ms, week.Add(-Period), week, []match.PlayerID{2, 3, 4, 5, 6, 7, 8, 9, 10}); len(got) != 1 {
		t.Fatalf("party with no matches in the period = %v, want only the owner", got)
	}
	// Someone in only one of several matches is not a regular.
	ms[0].Names[999], ms[0].Sides[999] = "guest", ms[0].Sides[1]
	for _, p := range Party(1, ms, week, week.Add(Period), []match.PlayerID{2, 3, 4, 5, 6, 7, 8, 9, 10}) {
		if p == 999 {
			t.Fatal("one-time guest joined the party")
		}
	}
}

func TestInputIsGroundedForRulesCoach(t *testing.T) {
	ms := fixtureMatches(t)
	var members []Member
	for _, p := range Party(1, ms, week, week.Add(Period), []match.PlayerID{2, 3, 4, 5, 6, 7, 8, 9, 10}) {
		members = append(members, Member{ID: p, Matches: ms})
	}
	in, ok := Input(members, week, week.Add(Period))
	if !ok {
		t.Fatal("no input")
	}
	if in.Matches != len(ms) || len(in.Party) != len(members) || in.Period != "23.10.2026 – 30.10.2026" {
		t.Fatalf("input = %d matches, party %v, period %q", in.Matches, in.Party, in.Period)
	}
	for _, ps := range in.Stats {
		for _, is := range ps.Issues {
			if is.Status == "" || is.Status == "fine" {
				t.Errorf("%s %s: status %q should not reach the coach", ps.Player, is.Kind, is.Status)
			}
			for _, ex := range is.Examples {
				e, ok := in.Evidence[ex.ID]
				if !ok || e.Kind != is.Kind || e.Player != ps.Player || e.Match == 0 {
					t.Errorf("example %s does not resolve to %s's %s finding: %+v", ex.ID, ps.Player, is.Kind, e)
				}
			}
		}
	}

	advice, err := coach.Rules{PerPlayer: 2}.Advise(t.Context(), in)
	if err != nil {
		t.Fatal(err)
	}
	if err := in.Validate(advice); err != nil {
		t.Fatalf("rules advice for a brief is not grounded: %v", err)
	}

	if _, ok := Input(members, week.Add(-Period), week); ok {
		t.Error("input for a period without matches")
	}
}

func TestInputDuplicateNamesPreservesIdentity(t *testing.T) {
	ms := fixtureMatches(t)
	for _, pm := range ms {
		pm.Names[1], pm.Names[2], pm.Names[3] = "same", "same", "same [player 1]"
	}
	in, ok := Input([]Member{{ID: 1, Matches: ms}, {ID: 2, Matches: ms}}, week, week.Add(Period))
	if !ok || len(in.Party) != 2 || in.Party[0] == in.Party[1] || in.Party[0] == "same [player 1]" {
		t.Fatalf("ambiguous brief party: %+v", in)
	}
	if len(in.Pairs) == 0 || len(in.Evidence) == 0 {
		t.Fatal("fixture must exercise both pair and personal evidence")
	}
	for _, ps := range in.Stats {
		for _, is := range ps.Issues {
			for _, ex := range is.Examples {
				if e := in.Evidence[ex.ID]; e.PlayerID != in.PlayerIDs[ps.Player] {
					t.Fatalf("example %s belongs to another player: %+v", ex.ID, e)
				}
			}
		}
	}
	for _, e := range in.Evidence {
		if e.Kind == detect.KindMissedTrade && (e.PlayerID == 1 || e.PlayerID == 2) {
			if len(e.RelatedIDs) == 0 {
				t.Fatal("missed trade lost related identity")
			}
		}
	}
	a, err := (coach.Rules{PerPlayer: 2}).Advise(t.Context(), in)
	if err != nil {
		t.Fatal(err)
	}
	if err := in.Validate(a); err != nil {
		t.Fatal(err)
	}
	if len(a.Players) != 2 || a.Players[0].PlayerID != 1 || a.Players[1].PlayerID != 2 {
		t.Fatalf("brief advice lost identity: %+v", a.Players)
	}
}

func TestScheduleLast(t *testing.T) {
	msk, err := time.LoadLocation("Europe/Moscow")
	if err != nil {
		t.Skip("no tz database")
	}
	s := Schedule{Weekday: time.Friday, Hour: 15, Location: msk}
	friday := time.Date(2026, 10, 30, 15, 0, 0, 0, msk)
	tests := []struct {
		now, want time.Time
	}{
		{friday, friday},
		{friday.Add(time.Minute), friday},
		{friday.Add(-time.Minute), friday.AddDate(0, 0, -7)},
		{friday.AddDate(0, 0, 3), friday}, // Monday after
		{friday.AddDate(0, 0, 6), friday}, // Thursday after
		{friday.AddDate(0, 0, -1), friday.AddDate(0, 0, -7)},
	}
	for _, tt := range tests {
		if got := s.Last(tt.now); !got.Equal(tt.want) {
			t.Errorf("Last(%v) = %v, want %v", tt.now, got, tt.want)
		}
	}
}

func TestPartyIgnoresOpponents(t *testing.T) {
	ms := fixtureMatches(t)
	// Put player 2 on the other side from the owner in every match.
	for _, pm := range ms {
		pm.Sides[2] = map[match.Side]match.Side{match.SideT: match.SideCT, match.SideCT: match.SideT}[pm.Sides[1]]
	}
	for _, p := range Party(1, ms, week, week.Add(Period), []match.PlayerID{2, 3, 4, 5, 6, 7, 8, 9, 10}) {
		if p == 2 {
			t.Fatal("an opponent from every match joined the party")
		}
	}
}

func TestPartyUsesSavedFriendsEvenAfterOneGameAndHonorsExclusions(t *testing.T) {
	ms := []store.PlayerMatch{
		{Played: week.Add(time.Hour), Sides: map[match.PlayerID]match.Side{1: match.SideT, 2: match.SideT, 3: match.SideCT, 4: match.SideT}},
		{Played: week.Add(2 * time.Hour), Sides: map[match.PlayerID]match.Side{1: match.SideT, 4: match.SideT}},
	}
	if got := Party(1, ms, week, week.Add(Period), []match.PlayerID{2, 3}); !slices.Equal(got, []match.PlayerID{1, 2}) {
		t.Fatalf("wrong friend party: %v", got)
	}
	ms[0].ManualParty = true
	ms[0].Party = []match.PlayerID{1}
	if got := Party(1, ms, week, week.Add(Period), []match.PlayerID{2, 3}); !slices.Equal(got, []match.PlayerID{1}) {
		t.Fatalf("manual exclusion ignored: %v", got)
	}
}

func TestInputCarriesPractice(t *testing.T) {
	ms := fixtureMatches(t)
	party := Party(1, ms, week, week.Add(Period), []match.PlayerID{2, 3, 4, 5, 6, 7, 8, 9, 10})
	first, ok := Input([]Member{{ID: party[0], Matches: ms}}, week, week.Add(Period))
	if !ok || len(first.Stats[0].Issues) == 0 {
		t.Fatal("no issues to practice")
	}
	kind := first.Stats[0].Issues[0].Kind

	// Practice starts after the first match; both players log the same.
	log := []store.Practice{
		{Kind: kind, Note: "deathmatch focus", On: week.Add(36 * time.Hour)},
		{Kind: "other_kind", Note: "unrelated", On: week},
	}
	members := []Member{
		{ID: party[0], Matches: ms, Practice: log, ShareNotes: true},
		{ID: party[1], Matches: ms, Practice: log},
	}
	in, _ := Input(members, week, week.Add(Period))
	for i, ps := range in.Stats {
		for _, is := range ps.Issues {
			p := is.Practice
			if is.Kind != kind {
				if p != nil {
					t.Errorf("%s %s carries practice logged for another mistake", ps.Player, is.Kind)
				}
				continue
			}
			if p == nil || p.Sessions != 1 || p.Since != log[0].On.Format("02.01.2006") || p.MatchesAfter != len(ms)-1 || p.Verdict == "" {
				t.Fatalf("%s practice = %+v", ps.Player, p)
			}
			if shared := len(p.Notes) > 0; shared != (i == 0) {
				t.Errorf("%s notes = %v: only the brief owner's notes are shared", ps.Player, p.Notes)
			}
		}
	}
}

func TestPartyPairs(t *testing.T) {
	const owner, anna, stranger match.PlayerID = 1, 2, 9
	week := time.Date(2026, 10, 23, 0, 0, 0, 0, time.UTC)
	names := map[match.PlayerID]string{owner: "oleg", anna: "anna", stranger: "x"}
	pm := func(id int64, played time.Time, traded int) store.PlayerMatch {
		return store.PlayerMatch{ID: id, Played: played, Names: names, Report: detect.Report{
			Pairs: []detect.Pair{
				{Player: owner, Partner: anna, Chances: 3, Traded: traded},
				{Player: owner, Partner: stranger, Chances: 5, Traded: 0}, // not in the party
			},
			Findings: []detect.Finding{
				{ID: "f4", Kind: detect.KindMissedTrade, Player: anna, Related: []match.PlayerID{owner}, Round: 4},
			},
		}}
	}
	ms := []store.PlayerMatch{
		pm(1, week.AddDate(0, 0, -30), 3), // an old match: not this week
		pm(2, week.AddDate(0, 0, 1), 0),
		pm(3, week.AddDate(0, 0, 2), 1),
	}
	members := []Member{{ID: owner, Matches: ms}, {ID: anna, Matches: ms}}
	ev := map[string]coach.Evidence{}
	got := partyPairs(members, week, week.AddDate(0, 0, 7), ev, names)
	if len(got) != 1 || got[0].Player != "oleg" || got[0].Partner != "anna" || got[0].Chances != 6 || got[0].Traded != 1 {
		t.Fatalf("pairs = %+v; want oleg/anna 1 of 6 this week", got)
	}
	if e, ok := ev["m2-f4"]; !ok || e.Player != "anna" || !slices.Equal(e.Related, []string{"oleg"}) {
		t.Errorf("evidence = %+v; want anna's missed trade of oleg", ev)
	}
	if _, ok := ev["m1-f4"]; ok {
		t.Error("evidence from outside the week")
	}
}
