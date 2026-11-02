// Package coach turns detector findings into prioritized advice: at most
// three focus areas per player plus a team section. Every piece of advice
// cites the findings it is based on, and Validate rejects advice that does not.
package coach

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"math"
	"slices"
	"strings"
	"time"

	"github.com/mrolegmir/cs-demo-coach/internal/detect"
	"github.com/mrolegmir/cs-demo-coach/internal/drills"
	"github.com/mrolegmir/cs-demo-coach/internal/history"
	"github.com/mrolegmir/cs-demo-coach/internal/match"
)

// MaxFocus is the most focus areas a player gets.
const MaxFocus = 3

// Coach produces advice for a party from detector output.
type Coach interface {
	Advise(ctx context.Context, in *Input) (*Advice, error)
}

// Advice is the coaching output for one match.
type Advice struct {
	// Language is stamped by the coach, never trusted from model output.
	Language string         `json:"language,omitempty"`
	Players  []PlayerAdvice `json:"players"`
	Team     []TeamAdvice   `json:"team"`
	// Notes holds caveats, e.g. too little data for a player.
	Notes []string `json:"notes,omitempty"`
}

// PlayerAdvice is the focus list for one player, most important first.
type PlayerAdvice struct {
	PlayerID match.PlayerID `json:"player_id,omitzero"`
	Player   string         `json:"player"`
	Focus    []Focus        `json:"focus"`
}

// Focus is one thing a player should work on.
type Focus struct {
	Kind  detect.Kind `json:"kind"`
	Title string      `json:"title"`
	Why   string      `json:"why"`
	// Priority is computed from detector counts, never supplied by the model.
	Priority string `json:"priority,omitempty"`
	// DrillID is the library drill to practice (package drills), and Drill
	// says how it applies to this player. Advice made before the library
	// has no DrillID.
	DrillID string `json:"drill_id,omitzero"`
	Drill   string `json:"drill"`
	// Evidence lists finding IDs that support this focus area.
	Evidence []string `json:"evidence"`
}

// TeamAdvice is a suggestion for the party as a whole.
type TeamAdvice struct {
	Title      string   `json:"title"`
	Why        string   `json:"why"`
	Suggestion string   `json:"suggestion"`
	Evidence   []string `json:"evidence"`
}

// Input is everything a coach sees: aggregated stats for the party and the
// findings behind them. It never contains raw demo data, and it is plain
// data, so it can be stored and handed to a remote worker.
type Input struct {
	// Language selects newly generated prose; empty means Russian.
	Language string `json:"language,omitempty"`
	// Map and Rounds describe a single match; Period and Matches a range of
	// matches, e.g. a week.
	Map       string                    `json:"map,omitempty"`
	Rounds    int                       `json:"rounds,omitempty"`
	Period    string                    `json:"period,omitempty"`
	Matches   int                       `json:"matches,omitempty"`
	Party     []string                  `json:"party"`
	PlayerIDs map[string]match.PlayerID `json:"player_ids,omitempty"`
	Stats     []PlayerStats             `json:"players"`
	// Evidence holds every finding advice may cite, by ID. It backs
	// validation and display and is not sent to the LLM in full.
	Evidence map[string]Evidence `json:"evidence"`
	// Pairs is how party members trade each other, most shared deaths
	// first.
	Pairs []Pair `json:"pairs,omitempty"`
	// Roles is what each party member does for the team, in party order.
	Roles []Role `json:"roles,omitempty"`
}

// Role is a party member's role numbers against the lobby (history.Roles).
type Role struct {
	Player string `json:"player"`
	// Labels are "entry", "support" and "awper" when the numbers show them.
	Labels           []string `json:"labels,omitempty"`
	OpeningRate      float64  `json:"opening_rate"`
	LobbyOpeningRate float64  `json:"lobby_opening_rate"`
	OpeningWinRate   float64  `json:"opening_win_rate"`
	// Flashes10 is flashes that blinded an enemy per ten rounds.
	Flashes10      float64 `json:"good_flashes_per_10"`
	LobbyFlashes10 float64 `json:"lobby_good_flashes_per_10"`
	AWPShare       float64 `json:"awp_share"`
}

// NewRole converts a role summary.
func NewRole(player string, s history.RoleSummary) Role {
	return Role{
		Player: player, Labels: s.Labels,
		OpeningRate: round2(s.OpeningRate()), LobbyOpeningRate: round2(s.LobbyOpeningRate()),
		OpeningWinRate: round2(s.OpeningWinRate()),
		Flashes10:      round2(s.FlashesPer10()), LobbyFlashes10: round2(s.LobbyFlashesPer10()),
		AWPShare: round2(s.AWPShare()),
	}
}

// Pair is how often Player's deaths were traded when Partner, another party
// member, was the nearest teammate.
type Pair struct {
	Player  string  `json:"player"`
	Partner string  `json:"partner"`
	Chances int     `json:"chances"`
	Traded  int     `json:"traded"`
	Rate    float64 `json:"rate"`
}

// NewPair converts a detector pair, with names.
func NewPair(player, partner string, chances, traded int) Pair {
	rate := detect.Pair{Chances: chances, Traded: traded}.Rate()
	return Pair{Player: player, Partner: partner, Chances: chances, Traded: traded, Rate: round2(rate)}
}

// SortPairs orders pairs by shared deaths, most first.
func SortPairs(ps []Pair) {
	slices.SortFunc(ps, func(a, b Pair) int {
		return cmp.Or(cmp.Compare(b.Chances, a.Chances), cmp.Compare(a.Player, b.Player), cmp.Compare(a.Partner, b.Partner))
	})
}

// Evidence is a finding as the coach and the reader see it, with players by
// name.
type Evidence struct {
	PlayerID   match.PlayerID   `json:"player_id,omitzero"`
	RelatedIDs []match.PlayerID `json:"related_ids,omitempty"`
	Kind       detect.Kind      `json:"kind"`
	Player     string           `json:"player"`
	Related    []string         `json:"related,omitempty"`
	Match      int64            `json:"match,omitzero"`
	Round      int              `json:"round"`
	Time       time.Duration    `json:"time"`
	Place      string           `json:"place,omitzero"` // in words
	Detail     string           `json:"detail"`
}

// evidence converts a finding of match m.
func evidence(m *match.Match, f detect.Finding) Evidence {
	e := Evidence{PlayerID: f.Player, RelatedIDs: slices.Clone(f.Related), Kind: f.Kind, Player: m.PlayerName(f.Player), Round: f.Round, Time: f.Time, Place: match.PlaceName(f.Place), Detail: f.Detail}
	for _, r := range f.Related {
		e.Related = append(e.Related, m.PlayerName(r))
	}
	return e
}

// PlayerStats is one party member's mistakes.
type PlayerStats struct {
	Player string  `json:"player"`
	Issues []Issue `json:"issues"`
}

// Issue is how often a player made one kind of mistake, compared with the
// whole lobby, with examples.
type Issue struct {
	Kind    detect.Kind `json:"kind"`
	Count   int         `json:"count"`
	Chances int         `json:"chances"`
	Rate    float64     `json:"rate"`
	// LobbyRate is the rate across all ten players of the match.
	LobbyRate float64 `json:"lobby_rate"`
	// StandsOut marks issues that happened at least MinCount times and are
	// at least MinZ standard deviations above what a lobby-average player
	// would do with the same chances. Only these may become focus areas.
	StandsOut bool `json:"stands_out"`
	// Status is the issue's history status in period input (see package
	// history); empty for a single match.
	Status string `json:"status,omitempty"`
	// Practice is set when the player logged practice for this issue.
	Practice *Practice `json:"practice,omitempty"`
	// Places is where the findings happened, most first, at most maxPlaces;
	// Placed is how many findings have a place at all.
	Places   []Place   `json:"places,omitempty"`
	Placed   int       `json:"placed,omitzero"`
	Examples []Example `json:"examples"`
}

// Place is how many of an issue's findings happened at one spot of the map.
type Place struct {
	// Name is in words, with the map when input spans several matches:
	// "B site" or "B site, Mirage".
	Name  string `json:"name"`
	Count int    `json:"count"`
}

// maxPlaces caps places per issue; the rest are scattered.
const maxPlaces = 3

// TopPlaces converts place counts sorted most first into an issue's top
// places, and returns how many findings have a place. withMap adds the map
// to each name.
func TopPlaces(sorted []history.PlaceCount, withMap bool) (top []Place, placed int) {
	for i, p := range sorted {
		placed += p.Count
		if i >= maxPlaces {
			continue
		}
		name := match.PlaceName(p.Place)
		if withMap {
			name += ", " + match.MapName(p.Map)
		}
		top = append(top, Place{Name: name, Count: p.Count})
	}
	return top, placed
}

// Shown returns the places worth showing to players: those with two
// findings or more.
func Shown(places []Place) []Place {
	var out []Place
	for _, p := range places {
		if p.Count >= 2 {
			out = append(out, p)
		}
	}
	return out
}

// Practice summarizes a player's logged practice on one issue and how the
// issue changed since it started.
type Practice struct {
	Sessions int    `json:"sessions"`
	Since    string `json:"since"` // date of the first session, e.g. "20.10.2026"
	// Notes are the player's latest notes on what they practiced, and
	// Drills the library drills among them.
	Notes        []string `json:"notes,omitempty"`
	Drills       []string `json:"drills,omitempty"`
	MatchesAfter int      `json:"matches_after"`
	RateBefore   float64  `json:"rate_before"`
	RateAfter    float64  `json:"rate_after"`
	// Verdict is "working", "not_yet", "too_early" or "no_baseline" (see
	// history.Effect.Verdict).
	Verdict string `json:"verdict"`
}

// MinCount is how many times a mistake must happen to stand out.
const MinCount = 3

// MinZ is how far above the lobby rate, in standard deviations of a
// binomial with the lobby rate, a player must be for an issue to stand out.
// One standard deviation keeps the bar low enough for a single match while
// filtering out differences that are plain noise.
const MinZ = 1.0

// Example is one finding as shown to the coach.
type Example struct {
	ID string `json:"id"`
	// Match names the match ("de_dust2, Oct 24") when input spans several.
	Match string `json:"match,omitempty"`
	Round int    `json:"round"`
	// Time is empty for findings about a whole round, such as buys.
	Time string `json:"time,omitempty"`
	// Place is where on the map, in words; empty for whole-round findings.
	Place  string `json:"place,omitzero"`
	Detail string `json:"detail"`
}

// maxExamples caps examples per issue to keep LLM prompts small; counts and
// rates still cover every occurrence.
const maxExamples = 6

// NewInput prepares coach input for the party members of a match.
func NewInput(m *match.Match, rep detect.Report, party []match.PlayerID) *Input {
	names := map[match.PlayerID]string{}
	for _, p := range m.Players {
		names[p.ID] = p.Name
	}
	labels := PlayerLabels(names)
	copyMatch := *m
	copyMatch.Players = slices.Clone(m.Players)
	for i := range copyMatch.Players {
		copyMatch.Players[i].Name = labels[copyMatch.Players[i].ID]
	}
	m = &copyMatch

	in := &Input{
		Map:      m.Map,
		Rounds:   len(m.Rounds),
		Evidence: map[string]Evidence{},
	}
	for _, p := range party {
		in.Party = append(in.Party, m.PlayerName(p))
		if in.PlayerIDs == nil {
			in.PlayerIDs = map[string]match.PlayerID{}
		}
		in.PlayerIDs[m.PlayerName(p)] = p
	}
	for _, f := range rep.Findings {
		in.Evidence[f.ID] = evidence(m, f)
	}
	for _, p := range rep.Pairs {
		if slices.Contains(party, p.Player) && slices.Contains(party, p.Partner) {
			in.Pairs = append(in.Pairs, NewPair(m.PlayerName(p.Player), m.PlayerName(p.Partner), p.Chances, p.Traded))
		}
	}
	SortPairs(in.Pairs)
	// One match is too short for role labels (history.Roles needs more
	// rounds); the numbers still go to the coach.
	for _, p := range party {
		hm := history.FromReport(0, time.Time{}, m.Map, p, rep)
		if hm.Role.Rounds > 0 {
			in.Roles = append(in.Roles, NewRole(m.PlayerName(p), history.Roles([]history.Match{hm})))
		}
	}

	lobby := map[detect.Kind][2]int{}
	for _, s := range rep.Stats {
		l := lobby[s.Kind]
		lobby[s.Kind] = [2]int{l[0] + s.Count, l[1] + s.Chances}
	}
	for _, p := range party {
		ps := PlayerStats{Player: m.PlayerName(p), Issues: []Issue{}}
		for _, s := range rep.Stats {
			if s.Player != p || s.Count == 0 {
				continue
			}
			l := lobby[s.Kind]
			is := Issue{
				Kind:      s.Kind,
				Count:     s.Count,
				Chances:   s.Chances,
				Rate:      round2(s.Rate()),
				LobbyRate: round2(ratio(l[0], l[1])),
				Examples:  []Example{},
			}
			is.StandsOut = is.Count >= MinCount && is.z() >= MinZ
			is.Places, is.Placed = TopPlaces(history.CountPlaces(m.Map, rep.Findings, p, s.Kind), false)
			for _, id := range s.Findings[:min(len(s.Findings), maxExamples)] {
				is.Examples = append(is.Examples, example(id, in.Evidence[id]))
			}
			ps.Issues = append(ps.Issues, is)
		}
		slices.SortFunc(ps.Issues, compareIssues)
		in.Stats = append(in.Stats, ps)
	}
	return in
}

func example(id string, e Evidence) Example {
	ex := Example{ID: id, Round: e.Round, Place: e.Place, Detail: e.Detail}
	if e.Time > 0 {
		ex.Time = match.Clock(e.Time)
	}
	return ex
}

// excess is how many more mistakes than a lobby-average player would have
// made with the same chances.
func (is Issue) excess() float64 {
	return float64(is.Count) - is.LobbyRate*float64(is.Chances)
}

// z is how many standard deviations the player's count is above the count
// expected at the lobby rate.
func (is Issue) z() float64 {
	n, p := float64(is.Chances), is.LobbyRate
	if n == 0 {
		return 0
	}
	if p <= 0 || p >= 1 {
		// Nobody else made this mistake (or everybody did): any occurrence
		// is unusual (or none is).
		if p <= 0 {
			return math.Inf(1)
		}
		return 0
	}
	return (float64(is.Count) - n*p) / math.Sqrt(n*p*(1-p))
}

// Validate checks that advice is grounded: every player is a party member,
// nobody gets more than MaxFocus focus areas, every focus area is about an
// issue that stands out for that player, and every focus area and team
// suggestion cites existing findings about the right players.
func (in *Input) Validate(a *Advice) error {
	if a == nil {
		return errors.New("advice is required")
	}
	if err := in.ValidateIdentity(); err != nil {
		return err
	}
	var errs []error
	if len(a.Team) > maxTeam {
		errs = append(errs, fmt.Errorf("%d team items, max %d", len(a.Team), maxTeam))
	}
	seen := map[string]bool{}
	for _, pa := range a.Players {
		if !slices.Contains(in.Party, pa.Player) {
			errs = append(errs, fmt.Errorf("player %q is not in the party", pa.Player))
			continue
		}
		if seen[pa.Player] {
			errs = append(errs, fmt.Errorf("player %q appears twice", pa.Player))
		}
		seen[pa.Player] = true
		if in.PlayerIDs != nil && pa.PlayerID != 0 && pa.PlayerID != in.PlayerIDs[pa.Player] {
			errs = append(errs, fmt.Errorf("player %q has a mismatched ID", pa.Player))
		}
		if len(pa.Focus) > MaxFocus {
			errs = append(errs, fmt.Errorf("player %q has %d focus areas, max %d", pa.Player, len(pa.Focus), MaxFocus))
		}
		focusKinds := map[detect.Kind]bool{}
		for i, f := range pa.Focus {
			where := fmt.Sprintf("player %q focus %d", pa.Player, i+1)
			if focusKinds[f.Kind] {
				errs = append(errs, fmt.Errorf("%s: duplicate kind %q", where, f.Kind))
			}
			focusKinds[f.Kind] = true
			if _, ok := kinds[f.Kind]; !ok {
				errs = append(errs, fmt.Errorf("%s: unknown kind %q", where, f.Kind))
			}
			if strings.TrimSpace(f.Title) == "" || strings.TrimSpace(f.Why) == "" || strings.TrimSpace(f.Drill) == "" {
				errs = append(errs, fmt.Errorf("%s: title, why and drill are required", where))
			}
			errs = append(errs, in.checkDrill(where, pa.Player, f))
			errs = append(errs, in.checkEvidence(where, f.Evidence, func(e Evidence) bool {
				return in.involves(e, pa.Player)
			}))
			if !in.standsOut(pa.Player, f.Kind) {
				errs = append(errs, fmt.Errorf("%s: %q does not stand out for this player", where, f.Kind))
			}
			if !slices.ContainsFunc(f.Evidence, func(id string) bool {
				e, ok := in.Evidence[id]
				return ok && e.Kind == f.Kind && in.primary(e, pa.Player)
			}) {
				errs = append(errs, fmt.Errorf("%s: no evidence of kind %q for this player", where, f.Kind))
			}
		}
	}
	for i, t := range a.Team {
		where := fmt.Sprintf("team advice %d", i+1)
		if strings.TrimSpace(t.Title) == "" || strings.TrimSpace(t.Why) == "" || strings.TrimSpace(t.Suggestion) == "" {
			errs = append(errs, fmt.Errorf("%s: title, why and suggestion are required", where))
		}
		errs = append(errs, in.checkEvidence(where, t.Evidence, func(e Evidence) bool {
			return slices.ContainsFunc(in.Party, func(p string) bool { return in.involves(e, p) })
		}))
	}
	return errors.Join(errs...)
}

func (in *Input) checkEvidence(where string, ids []string, relevant func(Evidence) bool) error {
	if len(ids) == 0 {
		return fmt.Errorf("%s: evidence is required", where)
	}
	var errs []error
	for _, id := range ids {
		f, ok := in.Evidence[id]
		switch {
		case !ok:
			errs = append(errs, fmt.Errorf("%s: unknown finding %q", where, id))
		case !relevant(f):
			errs = append(errs, fmt.Errorf("%s: finding %q is about someone else", where, id))
		}
	}
	return errors.Join(errs...)
}

// checkDrill requires a library drill for the focus area's kind, and a
// different one than the player has been practicing when that is not
// working yet.
func (in *Input) checkDrill(where, player string, f Focus) error {
	d, ok := drills.ByID(f.DrillID)
	switch {
	case f.DrillID == "":
		return fmt.Errorf("%s: drill_id is required", where)
	case !ok:
		return fmt.Errorf("%s: unknown drill_id %q", where, f.DrillID)
	case d.Kind != f.Kind:
		return fmt.Errorf("%s: drill %q is for %q, not %q", where, d.ID, d.Kind, f.Kind)
	}
	if is := in.issue(player, f.Kind); is != nil && is.Practice != nil && is.Practice.Verdict == "not_yet" &&
		slices.Contains(is.Practice.Drills, d.ID) && untried(f.Kind, is.Practice.Drills) != nil {
		return fmt.Errorf("%s: the player has practiced drill %q without effect; pick another drill for %q", where, d.ID, f.Kind)
	}
	return nil
}

// untried returns the first library drill for kind that is not in tried, or
// nil if the player has tried them all.
func untried(kind detect.Kind, tried []string) *drills.Drill {
	for _, d := range drills.ForKind(kind) {
		if !slices.Contains(tried, d.ID) {
			return &d
		}
	}
	return nil
}

// issue returns a player's issue of a kind, or nil.
func (in *Input) issue(player string, kind detect.Kind) *Issue {
	for i := range in.Stats {
		if in.Stats[i].Player != player {
			continue
		}
		for j := range in.Stats[i].Issues {
			if in.Stats[i].Issues[j].Kind == kind {
				return &in.Stats[i].Issues[j]
			}
		}
	}
	return nil
}

func (in *Input) standsOut(player string, kind detect.Kind) bool {
	is := in.issue(player, kind)
	return is != nil && is.StandsOut
}

// involves reports whether a finding is about the named player, either as
// the one who made the mistake or as a related player.
func (e Evidence) involves(player string) bool {
	return e.Player == player || slices.Contains(e.Related, player)
}

func ratio(a, b int) float64 {
	if b == 0 {
		return 0
	}
	return float64(a) / float64(b)
}

func round2(x float64) float64 {
	return math.Round(x*100) / 100
}

func (in *Input) primary(e Evidence, player string) bool {
	if in.PlayerIDs != nil {
		return e.PlayerID != 0 && e.PlayerID == in.PlayerIDs[player]
	}
	return e.Player == player
}

func (in *Input) involves(e Evidence, player string) bool {
	if in.PlayerIDs != nil {
		id := in.PlayerIDs[player]
		return id != 0 && (e.PlayerID == id || slices.Contains(e.RelatedIDs, id))
	}
	return e.involves(player)
}

// ValidateIdentity rejects legacy collisions before any coach uses the input.
func (in *Input) ValidateIdentity() error {
	if in == nil {
		return errors.New("coach input is required")
	}
	partySeen := map[string]bool{}
	idsSeen := map[match.PlayerID]bool{}
	for _, p := range in.Party {
		if partySeen[p] {
			return errors.New("ambiguous legacy party; request a new analysis")
		}
		partySeen[p] = true
		if in.PlayerIDs != nil {
			id := in.PlayerIDs[p]
			if id == 0 || idsSeen[id] {
				return errors.New("invalid player identity mapping")
			}
			idsSeen[id] = true
		}
	}
	return nil
}
