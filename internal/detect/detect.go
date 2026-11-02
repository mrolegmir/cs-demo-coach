// Package detect finds recurring mistakes in a parsed match. Detectors are
// deterministic: the same match always yields the same findings, and every
// finding points at the round and moment it happened.
package detect

import (
	"cmp"
	"fmt"
	"maps"
	"slices"
	"time"

	"github.com/mrolegmir/cs-demo-coach/internal/match"
)

// Kind identifies the type of mistake a finding describes.
type Kind string

const (
	KindIsolatedDeath Kind = "isolated_death"
	KindMissedTrade   Kind = "missed_trade"
	KindFlashNoEnemy  Kind = "flash_no_enemy"
	KindTeamFlash     Kind = "team_flash"
	KindUnusedUtility Kind = "unused_utility"
	KindBuyDesync     Kind = "buy_desync"
	KindMissedDrop    Kind = "missed_drop"
	KindNoUtility     Kind = "no_utility"
	KindOpeningDeath  Kind = "opening_death"
)

// Finding is one occurrence of a mistake.
type Finding struct {
	ID     string         `json:"id"`
	Kind   Kind           `json:"kind"`
	Player match.PlayerID `json:"player"`
	Side   match.Side     `json:"side"`
	Round  int            `json:"round"`
	Tick   int            `json:"tick"`
	// Time is the moment since the round went live.
	Time time.Duration `json:"time"`
	// Related are other players involved, e.g. the teammate who could have
	// traded.
	Related []match.PlayerID `json:"related,omitempty"`
	// Detail is a short human-readable description of the evidence.
	Detail string `json:"detail"`
	// Place is the map's name for where the player was (match.PlayerState),
	// for findings about a moment; empty for findings about a whole round.
	Place string `json:"place,omitzero"`
}

// Result is the output of one detector.
type Result struct {
	Findings []Finding
	// Chances counts, per kind and player, how often the mistake could have
	// happened (deaths, flashes thrown, rounds bought...). It turns raw
	// counts into rates that are comparable between players and matches.
	Chances map[Kind]map[match.PlayerID]int
	// Pairs is how teammates trade each other (Trade only).
	Pairs []Pair
}

// Pair is how often one player's deaths were traded when a given teammate
// was the nearest one, within trading distance.
type Pair struct {
	// Player died; Partner was the nearest teammate within trading distance.
	Player  match.PlayerID `json:"player"`
	Partner match.PlayerID `json:"partner"`
	Chances int            `json:"chances"`
	Traded  int            `json:"traded"`
}

// Rate returns Traded / Chances, or 0 when there were no chances.
func (p Pair) Rate() float64 {
	if p.Chances == 0 {
		return 0
	}
	return float64(p.Traded) / float64(p.Chances)
}

func (r *Result) chance(kind Kind, p match.PlayerID) {
	if r.Chances == nil {
		r.Chances = map[Kind]map[match.PlayerID]int{}
	}
	if r.Chances[kind] == nil {
		r.Chances[kind] = map[match.PlayerID]int{}
	}
	r.Chances[kind][p]++
}

// Detector finds one or more kinds of mistakes.
type Detector interface {
	Detect(m *match.Match) Result
}

// Default returns all detectors with their default thresholds.
func Default() []Detector {
	return []Detector{
		DefaultTrade(),
		Flash{MinEnemyBlind: MinEnemyBlind, MinTeamBlind: 1500 * time.Millisecond},
		UnusedUtility{},
		NoUtility{FullBuy: 3700, MinLife: 30 * time.Second},
		BuyDesync{FullBuy: 3700, Eco: 1500, Under: 2500, DropMoney: 3000},
		OpeningDeath{SupportWindow: 2 * time.Second},
	}
}

// MinEnemyBlind is how long a flash must blind an enemy to count, for the
// flash detector and for roles.
const MinEnemyBlind = time.Second

// DefaultTrade is the trade detector with its default thresholds.
func DefaultTrade() Trade { return Trade{Window: 5 * time.Second, Distance: 700} }

// Report is the combined output of all detectors for a match.
type Report struct {
	Findings []Finding `json:"findings"`
	Stats    []Stat    `json:"stats"`
	// Pairs is nil in reports made before pairs were counted, and empty in
	// a match without any; Roles likewise.
	Pairs []Pair `json:"pairs"`
	Roles []Role `json:"roles"`
}

// Stat is how often one player made one kind of mistake.
type Stat struct {
	Player  match.PlayerID `json:"player"`
	Kind    Kind           `json:"kind"`
	Count   int            `json:"count"`
	Chances int            `json:"chances"`
	// Rounds lists the rounds the mistake happened in.
	Rounds []int `json:"rounds"`
	// Findings lists the IDs of the findings behind this stat.
	Findings []string `json:"findings"`
}

// Rate returns Count / Chances, or 0 when there were no chances.
func (s Stat) Rate() float64 {
	if s.Chances == 0 {
		return 0
	}
	return float64(s.Count) / float64(s.Chances)
}

// Run runs the detectors on a match. Findings are sorted by round and time
// and numbered f1, f2, ... in that order.
func Run(m *match.Match, detectors []Detector) Report {
	rep := Report{Pairs: []Pair{}}
	chances := map[Kind]map[match.PlayerID]int{}
	for _, d := range detectors {
		res := d.Detect(m)
		rep.Findings = append(rep.Findings, res.Findings...)
		rep.Pairs = append(rep.Pairs, res.Pairs...)
		maps.Copy(chances, res.Chances)
	}
	slices.SortStableFunc(rep.Findings, func(a, b Finding) int {
		return cmp.Or(
			cmp.Compare(a.Round, b.Round),
			cmp.Compare(a.Tick, b.Tick),
			cmp.Compare(a.Kind, b.Kind),
			cmp.Compare(a.Player, b.Player),
		)
	})
	for i := range rep.Findings {
		rep.Findings[i].ID = fmt.Sprintf("f%d", i+1)
	}
	rep.Stats = stats(rep.Findings, chances)
	rep.Roles = DefaultRoles().Count(m)
	return rep
}

func stats(findings []Finding, chances map[Kind]map[match.PlayerID]int) []Stat {
	type key struct {
		player match.PlayerID
		kind   Kind
	}
	byKey := map[key]*Stat{}
	get := func(p match.PlayerID, kind Kind) *Stat {
		k := key{p, kind}
		s, ok := byKey[k]
		if !ok {
			s = &Stat{Player: p, Kind: kind, Chances: chances[kind][p], Rounds: []int{}, Findings: []string{}}
			byKey[k] = s
		}
		return s
	}
	// Players who had chances but made no mistakes get a zero stat: a clean
	// record is evidence too.
	for kind, perPlayer := range chances {
		for p := range perPlayer {
			get(p, kind)
		}
	}
	for _, f := range findings {
		s := get(f.Player, f.Kind)
		s.Count++
		if !slices.Contains(s.Rounds, f.Round) {
			s.Rounds = append(s.Rounds, f.Round)
		}
		s.Findings = append(s.Findings, f.ID)
	}
	out := make([]Stat, 0, len(byKey))
	for _, s := range byKey {
		out = append(out, *s)
	}
	slices.SortFunc(out, func(a, b Stat) int {
		return cmp.Or(cmp.Compare(a.Player, b.Player), cmp.Compare(a.Kind, b.Kind))
	})
	return out
}

// finding builds a finding at a tick of a round.
func finding(m *match.Match, r *match.Round, kind Kind, player match.PlayerID, tick int, detail string, related ...match.PlayerID) Finding {
	return Finding{
		Kind:    kind,
		Player:  player,
		Side:    r.Side(player),
		Round:   r.Number,
		Tick:    tick,
		Time:    m.Time(r, tick),
		Related: related,
		Detail:  detail,
	}
}

// placed sets where a finding happened.
func placed(f Finding, place string) Finding {
	f.Place = place
	return f
}

func ticks(m *match.Match, d time.Duration) int {
	return int(d.Seconds() * m.TickRate)
}
