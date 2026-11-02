// Package history tracks a player's mistakes across matches. A single match
// is noisy; history tells a habit (active), a habit that is fading
// (improving) or gone (resolved), and one bad game apart.
package history

import (
	"cmp"
	"math"
	"slices"
	"time"

	"github.com/mrolegmir/cs-demo-coach/internal/detect"
	"github.com/mrolegmir/cs-demo-coach/internal/match"
)

// Status is where an issue stands across a player's matches.
type Status string

const (
	// Active: above the lobby rate overall and still in recent matches.
	Active Status = "active"
	// Improving: above the lobby rate overall, but not in recent matches.
	Improving Status = "improving"
	// Resolved: stood out in earlier matches and is at or below the lobby
	// rate in recent ones.
	Resolved Status = "resolved"
	// OneOff: stood out in some matches, but not across them: bad games,
	// not a habit.
	OneOff Status = "one_off"
	// Fine: never stood out.
	Fine Status = "fine"
	// NotEnoughData: too few chances to tell.
	NotEnoughData Status = "not_enough_data"
)

// Counts is how often a mistake happened out of how many chances.
type Counts struct {
	Count   int `json:"count"`
	Chances int `json:"chances"`
}

func (c Counts) add(o Counts) Counts { return Counts{c.Count + o.Count, c.Chances + o.Chances} }

// Rate is Count / Chances, or 0 without chances.
func (c Counts) Rate() float64 {
	if c.Chances == 0 {
		return 0
	}
	return float64(c.Count) / float64(c.Chances)
}

// z is how many binomial standard deviations c is above the lobby rate p.
func (c Counts) z(p float64) float64 {
	n := float64(c.Chances)
	switch {
	case n == 0:
		return 0
	case p <= 0:
		if c.Count > 0 {
			return math.Inf(1)
		}
		return 0
	case p >= 1:
		return 0
	}
	return (float64(c.Count) - n*p) / math.Sqrt(n*p*(1-p))
}

// Match is one match's numbers for a player.
type Match struct {
	MatchID int64
	Played  time.Time
	Map     string
	// Player and Lobby hold counts per kind; the lobby is all ten players.
	Player map[detect.Kind]Counts
	Lobby  map[detect.Kind]Counts
	// Places counts the player's findings per kind by where they happened.
	Places map[detect.Kind][]PlaceCount
	// Pairs are the match's trade pairs that involve the player.
	Pairs []detect.Pair
	// Role is the player's role in the match, and LobbyRole everyone's
	// summed, to compare with.
	Role, LobbyRole detect.Role
}

// Issue is one kind of mistake across a player's matches.
type Issue struct {
	Kind   detect.Kind `json:"kind"`
	Status Status      `json:"status"`
	// Total and Lobby are summed over all matches.
	Total Counts `json:"total"`
	Lobby Counts `json:"lobby"`
	// Recent covers the last RecentMatches matches.
	Recent      Counts `json:"recent"`
	RecentLobby Counts `json:"recent_lobby"`
	// PerMatch is the player's count per match, oldest first.
	PerMatch []MatchPoint `json:"per_match"`
	// StoodOut counts matches where this issue stood out on its own.
	StoodOut int `json:"stood_out"`
	// Places is where the player's findings happened, most first.
	Places []PlaceCount `json:"places,omitempty"`
}

// PlaceCount is how many findings happened at one place of one map
// (match.PlaceName turns it into words). Place names repeat across maps
// ("BombsiteB"), so places are only added up within a map.
type PlaceCount struct {
	Map   string `json:"map"`
	Place string `json:"place"`
	Count int    `json:"count"`
}

// placeKey identifies a place on a map.
type placeKey struct{ Map, Place string }

// CountPlaces counts a player's findings of a kind by place.
func CountPlaces(mapName string, findings []detect.Finding, player match.PlayerID, kind detect.Kind) []PlaceCount {
	counts := map[placeKey]int{}
	for _, f := range findings {
		if f.Player == player && f.Kind == kind && f.Place != "" {
			counts[placeKey{mapName, f.Place}]++
		}
	}
	return sortPlaces(counts)
}

// sortPlaces orders place counts, most first, by map and place on ties.
func sortPlaces(counts map[placeKey]int) []PlaceCount {
	var out []PlaceCount
	for k, n := range counts {
		out = append(out, PlaceCount{k.Map, k.Place, n})
	}
	slices.SortFunc(out, func(a, b PlaceCount) int {
		return cmp.Or(cmp.Compare(b.Count, a.Count), cmp.Compare(a.Map, b.Map), cmp.Compare(a.Place, b.Place))
	})
	return out
}

// MatchPoint is one match on an issue's timeline.
type MatchPoint struct {
	MatchID  int64     `json:"match_id"`
	Played   time.Time `json:"played"`
	Counts   Counts    `json:"counts"`
	Lobby    Counts    `json:"lobby"`
	StandOut bool      `json:"stand_out"`
}

// Rules are the thresholds that decide statuses.
type Rules struct {
	// MinZ and MinCount decide whether counts stand out against the lobby,
	// like the per-match coach does.
	MinZ     float64
	MinCount int
	// MinChances is the least total chances needed to judge at all.
	MinChances int
	// RecentMatches is how many of the latest matches count as recent.
	RecentMatches int
}

// Default returns the thresholds used by the service.
func Default() Rules {
	return Rules{MinZ: 1, MinCount: 3, MinChances: 10, RecentMatches: 3}
}

func (r Rules) standsOut(c, lobby Counts) bool {
	return c.Count >= r.MinCount && c.z(lobby.Rate()) >= r.MinZ
}

// Build summarizes a player's matches into one issue per kind, most
// pressing first: active, improving, one-off, resolved, fine, not enough
// data; within a status by how far the player is above the lobby.
func (r Rules) Build(matches []Match) []Issue {
	ms := slices.Clone(matches)
	slices.SortStableFunc(ms, func(a, b Match) int { return a.Played.Compare(b.Played) })

	kinds := map[detect.Kind]bool{}
	for _, m := range ms {
		for k := range m.Player {
			kinds[k] = true
		}
	}

	var issues []Issue
	for k := range kinds {
		is := Issue{Kind: k}
		places := map[placeKey]int{}
		for i, m := range ms {
			for _, p := range m.Places[k] {
				places[placeKey{p.Map, p.Place}] += p.Count
			}
			c, l := m.Player[k], m.Lobby[k]
			is.Total, is.Lobby = is.Total.add(c), is.Lobby.add(l)
			if i >= len(ms)-r.RecentMatches {
				is.Recent, is.RecentLobby = is.Recent.add(c), is.RecentLobby.add(l)
			}
			p := MatchPoint{MatchID: m.MatchID, Played: m.Played, Counts: c, Lobby: l, StandOut: r.standsOut(c, l)}
			if p.StandOut {
				is.StoodOut++
			}
			is.PerMatch = append(is.PerMatch, p)
		}
		is.Status = r.status(is, len(ms))
		is.Places = sortPlaces(places)
		issues = append(issues, is)
	}

	order := map[Status]int{Active: 0, Improving: 1, OneOff: 2, Resolved: 3, Fine: 4, NotEnoughData: 5}
	slices.SortFunc(issues, func(a, b Issue) int {
		return cmp.Or(
			cmp.Compare(order[a.Status], order[b.Status]),
			cmp.Compare(b.excess(), a.excess()),
			cmp.Compare(a.Kind, b.Kind),
		)
	})
	return issues
}

// Effect compares an issue before and after the player started practicing
// it.
type Effect struct {
	Before, After           Counts
	LobbyBefore, LobbyAfter Counts
	// MatchesAfter is how many matches were played since the practice
	// started.
	MatchesAfter int
}

// MinMatchesAfter is how many matches after starting practice it takes to
// judge whether it works.
const MinMatchesAfter = 3

// Since splits the issue's matches at t, when practice started.
func (is Issue) Since(t time.Time) Effect {
	var e Effect
	for _, p := range is.PerMatch {
		if p.Played.Before(t) {
			e.Before, e.LobbyBefore = e.Before.add(p.Counts), e.LobbyBefore.add(p.Lobby)
			continue
		}
		e.After, e.LobbyAfter = e.After.add(p.Counts), e.LobbyAfter.add(p.Lobby)
		e.MatchesAfter++
	}
	return e
}

// Verdicts of Effect.Verdict.
const (
	// TooEarly: fewer than MinMatchesAfter matches since practice started.
	TooEarly = "too_early"
	// NoBaseline: no matches before practice started to compare with.
	NoBaseline = "no_baseline"
	// Working: the gap to the lobby shrank by at least a third, or the
	// player was not above the lobby and did not get worse.
	Working = "working"
	// NotYet: no such change.
	NotYet = "not_yet"
)

// Verdict says whether practice is paying off. It compares the player's gap
// to the lobby rather than raw rates, so easier or harder lobbies after the
// practice do not count as progress.
func (e Effect) Verdict() string {
	switch {
	case e.MatchesAfter < MinMatchesAfter || e.After.Chances == 0:
		return TooEarly
	case e.Before.Chances == 0:
		return NoBaseline
	}
	gapBefore := e.Before.Rate() - e.LobbyBefore.Rate()
	gapAfter := e.After.Rate() - e.LobbyAfter.Rate()
	if gapBefore > 0 && gapAfter <= gapBefore*2/3 || gapBefore <= 0 && gapAfter <= gapBefore {
		return Working
	}
	return NotYet
}

// Practice is a player's practice on one issue and its effect so far.
type Practice struct {
	Sessions int
	Since    time.Time // the first session
	Effect
}

// Practiced summarizes practice on the issue, given the days the player
// practiced it in any order; ok is false if there are none.
func (is Issue) Practiced(days []time.Time) (p Practice, ok bool) {
	if len(days) == 0 {
		return Practice{}, false
	}
	since := slices.MinFunc(days, func(a, b time.Time) int { return a.Compare(b) })
	return Practice{Sessions: len(days), Since: since, Effect: is.Since(since)}, true
}

// excess is how many more mistakes than a lobby-average player.
func (is Issue) excess() float64 {
	return float64(is.Total.Count) - float64(is.Total.Chances)*is.Lobby.Rate()
}

func (r Rules) status(is Issue, matches int) Status {
	if is.Total.Chances < r.MinChances {
		return NotEnoughData
	}
	if !r.standsOut(is.Total, is.Lobby) || r.oneBadGame(is) {
		if is.StoodOut > 0 {
			return OneOff
		}
		return Fine
	}
	// Recent counts are judged only when there is enough of them and some
	// history before them; otherwise "recent" is just "overall".
	if matches <= r.RecentMatches || is.Recent.Chances < r.MinChances || r.standsOut(is.Recent, is.RecentLobby) {
		return Active
	}
	if is.Recent.Rate() <= is.RecentLobby.Rate() && r.earlierHabit(is) {
		return Resolved
	}
	return Improving
}

// oneBadGame reports whether a single match makes the issue stand out: without
// it, the player is not above the lobby.
func (r Rules) oneBadGame(is Issue) bool {
	if is.StoodOut != 1 {
		return false
	}
	for _, p := range is.PerMatch {
		if p.StandOut {
			rest := Counts{is.Total.Count - p.Counts.Count, is.Total.Chances - p.Counts.Chances}
			return rest.Chances < r.MinChances || !r.standsOut(rest, is.Lobby)
		}
	}
	return false
}

// earlierHabit reports whether the issue stood out across the matches
// before the recent window, so "resolved" means a habit went away rather
// than one bad game being forgotten.
func (r Rules) earlierHabit(is Issue) bool {
	earlier := Counts{is.Total.Count - is.Recent.Count, is.Total.Chances - is.Recent.Chances}
	lobby := Counts{is.Lobby.Count - is.RecentLobby.Count, is.Lobby.Chances - is.RecentLobby.Chances}
	return earlier.Chances >= r.MinChances && r.standsOut(earlier, lobby)
}

// FromReport extracts one player's counts and the lobby's from a match's
// detector report.
func FromReport(matchID int64, played time.Time, mapName string, player match.PlayerID, rep detect.Report) Match {
	m := Match{
		MatchID: matchID,
		Played:  played,
		Map:     mapName,
		Player:  map[detect.Kind]Counts{},
		Lobby:   map[detect.Kind]Counts{},
		Places:  map[detect.Kind][]PlaceCount{},
	}
	for _, p := range rep.Pairs {
		if p.Player == player || p.Partner == player {
			m.Pairs = append(m.Pairs, p)
		}
	}
	for _, r := range rep.Roles {
		if r.Player == player {
			m.Role = r
		}
		m.LobbyRole = addRole(m.LobbyRole, r)
	}
	for _, s := range rep.Stats {
		if s.Player == player {
			if ps := CountPlaces(mapName, rep.Findings, player, s.Kind); ps != nil {
				m.Places[s.Kind] = ps
			}
		}
		c := Counts{Count: s.Count, Chances: s.Chances}
		m.Lobby[s.Kind] = m.Lobby[s.Kind].add(c)
		if s.Player == player {
			m.Player[s.Kind] = c
		}
	}
	return m
}

// TrendPoint is an issue's rolling rate at one match: the player's and the
// lobby's over that match and the ones just before it.
type TrendPoint struct {
	MatchID     int64
	Played      time.Time
	Rate, Lobby float64
}

// Trend returns the rolling rates over window matches, oldest first, so one
// noisy match does not swing the line. It starts at the first full window;
// windows with no chances are left out.
func (is Issue) Trend(window int) []TrendPoint {
	window = max(window, 1)
	var out []TrendPoint
	for i, p := range is.PerMatch {
		if i < window-1 {
			continue
		}
		var c, l Counts
		for _, q := range is.PerMatch[i-window+1 : i+1] {
			c, l = c.add(q.Counts), l.add(q.Lobby)
		}
		if c.Chances == 0 {
			continue
		}
		out = append(out, TrendPoint{MatchID: p.MatchID, Played: p.Played, Rate: c.Rate(), Lobby: l.Rate()})
	}
	return out
}

// Partner is how a player and one teammate trade each other across matches.
// Counts are traded deaths out of deaths with the other one nearest.
type Partner struct {
	ID match.PlayerID
	// Covered: the player died with this teammate nearest and was traded.
	Covered Counts
	// Covering: the teammate died with the player nearest and was traded.
	Covering Counts
}

// Total is how many deaths of either one the other could have traded.
func (p Partner) Total() int { return p.Covered.Chances + p.Covering.Chances }

// Partners sums a player's trade pairs across matches, by teammate, most
// shared deaths first.
func Partners(player match.PlayerID, matches []Match) []Partner {
	byID := map[match.PlayerID]*Partner{}
	get := func(id match.PlayerID) *Partner {
		if byID[id] == nil {
			byID[id] = &Partner{ID: id}
		}
		return byID[id]
	}
	for _, m := range matches {
		for _, p := range m.Pairs {
			c := Counts{Count: p.Traded, Chances: p.Chances}
			switch player {
			case p.Player:
				pt := get(p.Partner)
				pt.Covered = pt.Covered.add(c)
			case p.Partner:
				pt := get(p.Player)
				pt.Covering = pt.Covering.add(c)
			}
		}
	}
	var out []Partner
	for _, p := range byID {
		out = append(out, *p)
	}
	slices.SortFunc(out, func(a, b Partner) int { return cmp.Or(cmp.Compare(b.Total(), a.Total()), cmp.Compare(a.ID, b.ID)) })
	return out
}

func addRole(a, b detect.Role) detect.Role {
	return detect.Role{
		Player: a.Player, Rounds: a.Rounds + b.Rounds,
		Openings: a.Openings + b.Openings, OpeningWins: a.OpeningWins + b.OpeningWins,
		Kills: a.Kills + b.Kills, AWPKills: a.AWPKills + b.AWPKills,
		Flashes: a.Flashes + b.Flashes, GoodFlashes: a.GoodFlashes + b.GoodFlashes,
	}
}

// Role labels.
const (
	Entry   = "entry"
	Support = "support"
	AWPer   = "awper"
)

// RoleSummary is a player's role across matches against their lobbies.
type RoleSummary struct {
	Player, Lobby detect.Role
	// Labels are the roles the numbers show, if any: Entry, Support, AWPer.
	Labels []string
}

// OpeningRate is the share of rounds whose first duel the player was in.
func (s RoleSummary) OpeningRate() float64 { return per(s.Player.Openings, s.Player.Rounds) }

// LobbyOpeningRate is the same for an average player of the lobby.
func (s RoleSummary) LobbyOpeningRate() float64 { return per(s.Lobby.Openings, s.Lobby.Rounds) }

// OpeningWinRate is the share of first duels the player won.
func (s RoleSummary) OpeningWinRate() float64 { return per(s.Player.OpeningWins, s.Player.Openings) }

// FlashesPer10 is flashes that blinded an enemy per ten rounds.
func (s RoleSummary) FlashesPer10() float64 { return 10 * per(s.Player.GoodFlashes, s.Player.Rounds) }

// LobbyFlashesPer10 is the same for an average player of the lobby.
func (s RoleSummary) LobbyFlashesPer10() float64 {
	return 10 * per(s.Lobby.GoodFlashes, s.Lobby.Rounds)
}

// AWPShare is the share of the player's kills made with the AWP.
func (s RoleSummary) AWPShare() float64 { return per(s.Player.AWPKills, s.Player.Kills) }

func per(n, d int) float64 {
	if d == 0 {
		return 0
	}
	return float64(n) / float64(d)
}

// Thresholds for role labels: a role needs enough rounds or kills to judge,
// and clearly more than an average player of the same lobbies.
const (
	minRoleRounds  = 30
	minOpenings    = 5
	roleFactor     = 1.5
	minGoodFlashes = 10
	minAWPKills    = 10
	minAWPShare    = 0.4
)

// Roles sums a player's roles across matches and labels them.
func Roles(matches []Match) RoleSummary {
	var s RoleSummary
	for _, m := range matches {
		s.Player = addRole(s.Player, m.Role)
		s.Lobby = addRole(s.Lobby, m.LobbyRole)
	}
	if s.Player.Rounds >= minRoleRounds {
		if s.Player.Openings >= minOpenings && s.OpeningRate() >= roleFactor*s.LobbyOpeningRate() {
			s.Labels = append(s.Labels, Entry)
		}
		if s.Player.GoodFlashes >= minGoodFlashes && s.FlashesPer10() >= roleFactor*s.LobbyFlashesPer10() {
			s.Labels = append(s.Labels, Support)
		}
	}
	if s.Player.AWPKills >= minAWPKills && s.AWPShare() >= minAWPShare {
		s.Labels = append(s.Labels, AWPer)
	}
	return s
}
