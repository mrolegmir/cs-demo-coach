package detect

import (
	"fmt"
	"testing"
	"time"

	"github.com/mrolegmir/cs-demo-coach/internal/match"
)

// Players of the synthetic test match: t1..t3 are terrorists, c1..c3
// counter-terrorists.
const (
	t1 match.PlayerID = iota + 1
	t2
	t3
	c1
	c2
	c3
)

const tickRate = 64

// sec converts seconds since the round went live to a tick.
func sec(s float64) int { return int(s * tickRate) }

func newMatch(rounds ...match.Round) *match.Match {
	m := &match.Match{TickRate: tickRate}
	for _, p := range []match.PlayerID{t1, t2, t3, c1, c2, c3} {
		m.Players = append(m.Players, match.Player{ID: p, Name: name(p)})
	}
	for i := range rounds {
		rounds[i].Number = i + 1
		if rounds[i].Players == nil {
			rounds[i].Players = roster(3000)
		}
	}
	m.Rounds = rounds
	return m
}

func name(p match.PlayerID) string {
	return [...]string{"", "t1", "t2", "t3", "c1", "c2", "c3"}[p]
}

func sideOf(p match.PlayerID) match.Side {
	if p <= t3 {
		return match.SideT
	}
	return match.SideCT
}

// roster puts every player in the round with the same equipment value.
func roster(equipment int) []match.PlayerRound {
	var ps []match.PlayerRound
	for _, p := range []match.PlayerID{t1, t2, t3, c1, c2, c3} {
		ps = append(ps, match.PlayerRound{Player: p, Side: sideOf(p), EquipmentValue: equipment})
	}
	return ps
}

// at places a player at x on a line.
func at(p match.PlayerID, x float64) match.PlayerState {
	return match.PlayerState{Player: p, Side: sideOf(p), Pos: match.Vec{X: x}}
}

func kill(s float64, killer, victim match.PlayerID, alive ...match.PlayerState) match.Kill {
	return match.Kill{Tick: sec(s), Killer: killer, Victim: victim, Weapon: "AK-47", Alive: alive}
}

func blind(p match.PlayerID, d time.Duration) match.Blinding {
	return match.Blinding{Player: p, Side: sideOf(p), Duration: d}
}

func flash(s float64, thrower match.PlayerID, blinded ...match.Blinding) match.Flash {
	return match.Flash{Tick: sec(s), Thrower: thrower, Side: sideOf(thrower), Blinded: blinded}
}

// kinds returns the kind and player of each finding, in order.
func kinds(fs []Finding) []string {
	var out []string
	for _, f := range fs {
		out = append(out, string(f.Kind)+":"+name(f.Player))
	}
	return out
}

func assertFindings(t *testing.T, got []Finding, want ...string) {
	t.Helper()
	g := kinds(got)
	if len(g) != len(want) {
		t.Fatalf("findings = %v, want %v", g, want)
	}
	for i := range g {
		if g[i] != want[i] {
			t.Fatalf("findings = %v, want %v", g, want)
		}
	}
}

func TestRunNumbersFindingsAndCountsCleanPlayers(t *testing.T) {
	m := newMatch(match.Round{
		Flashes: []match.Flash{
			flash(10, t1),                           // blinded nobody
			flash(5, t2, blind(c1, 2*time.Second)),  // good flash
			flash(20, c1, blind(t1, 3*time.Second)), // good flash
			flash(30, c1, blind(c2, 2*time.Second)), // team flash, no enemy
		},
	})
	rep := Run(m, []Detector{Flash{MinEnemyBlind: time.Second, MinTeamBlind: time.Second}})

	assertFindings(t, rep.Findings, "flash_no_enemy:t1", "flash_no_enemy:c1", "team_flash:c1")
	for i, f := range rep.Findings {
		if want := fmt.Sprintf("f%d", i+1); f.ID != want {
			t.Errorf("finding %d ID = %q, want %q", i, f.ID, want)
		}
	}

	stat := func(p match.PlayerID, k Kind) Stat {
		for _, s := range rep.Stats {
			if s.Player == p && s.Kind == k {
				return s
			}
		}
		t.Fatalf("no stat for %s %s", name(p), k)
		return Stat{}
	}
	if s := stat(t2, KindFlashNoEnemy); s.Count != 0 || s.Chances != 1 {
		t.Errorf("t2 flash stat = %d/%d, want 0/1", s.Count, s.Chances)
	}
	if s := stat(c1, KindFlashNoEnemy); s.Count != 1 || s.Chances != 2 || s.Rate() != 0.5 {
		t.Errorf("c1 flash stat = %d/%d, want 1/2", s.Count, s.Chances)
	}
	if s := stat(c1, KindTeamFlash); len(s.Findings) != 1 || s.Findings[0] != "f3" {
		t.Errorf("c1 team flash findings = %v, want [f3]", s.Findings)
	}
}

func TestTrade(t *testing.T) {
	d := Trade{Window: 5 * time.Second, Distance: 700}
	tests := []struct {
		name  string
		kills []match.Kill
		want  []string
	}{
		{
			name:  "isolated death",
			kills: []match.Kill{kill(10, c1, t1, at(t1, 0), at(t2, 2000), at(c1, 300))},
			want:  []string{"isolated_death:t1"},
		},
		{
			name:  "close teammate did not trade",
			kills: []match.Kill{kill(10, c1, t1, at(t1, 0), at(t2, 400), at(t3, 600), at(c1, 300))},
			want:  []string{"missed_trade:t2"},
		},
		{
			name: "traded in time",
			kills: []match.Kill{
				kill(10, c1, t1, at(t1, 0), at(t2, 400), at(c1, 300)),
				kill(13, t2, c1, at(t2, 400), at(c1, 300)),
			},
		},
		{
			name: "traded too late",
			kills: []match.Kill{
				kill(10, c1, t1, at(t1, 0), at(t2, 400), at(c1, 300)),
				kill(16, t2, c1, at(t2, 400), at(c1, 300)),
			},
			want: []string{"missed_trade:t2"},
		},
		{
			name:  "last alive cannot be traded",
			kills: []match.Kill{kill(10, c1, t1, at(t1, 0), at(c1, 300))},
		},
		{
			name:  "team kill is not a trade situation",
			kills: []match.Kill{{Tick: sec(10), Killer: t2, Victim: t1, TeamKill: true, Alive: []match.PlayerState{at(t1, 0), at(t2, 10000)}}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res := d.Detect(newMatch(match.Round{Kills: tt.kills}))
			assertFindings(t, res.Findings, tt.want...)
		})
	}
}

func TestFlash(t *testing.T) {
	d := Flash{MinEnemyBlind: time.Second, MinTeamBlind: 1500 * time.Millisecond}
	tests := []struct {
		name  string
		flash match.Flash
		want  []string
	}{
		{"blinds enemy", flash(5, t1, blind(c1, 2*time.Second)), nil},
		{"blinds nobody", flash(5, t1), []string{"flash_no_enemy:t1"}},
		{"enemy barely blinded", flash(5, t1, blind(c1, 500*time.Millisecond)), []string{"flash_no_enemy:t1"}},
		{"self flash", flash(5, t1, blind(c1, 2*time.Second), blind(t1, 2*time.Second)), []string{"team_flash:t1"}},
		{"short team blind is fine", flash(5, t1, blind(c1, 2*time.Second), blind(t2, time.Second)), nil},
		{"team flash only", flash(5, t1, blind(t2, 3*time.Second)), []string{"flash_no_enemy:t1", "team_flash:t1"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res := d.Detect(newMatch(match.Round{Flashes: []match.Flash{tt.flash}}))
			assertFindings(t, res.Findings, tt.want...)
		})
	}
}

func TestUnusedUtility(t *testing.T) {
	withUtil := func(victim match.PlayerID, util ...string) match.Kill {
		k := kill(20, c1, victim)
		k.VictimUtility = util
		return k
	}
	m := newMatch(match.Round{Kills: []match.Kill{
		withUtil(t1, "Flashbang", "Smoke Grenade"),
		withUtil(t2, "Decoy Grenade"),
		withUtil(t3),
	}})
	res := UnusedUtility{}.Detect(m)
	assertFindings(t, res.Findings, "unused_utility:t1")
	if got := res.Findings[0].Detail; got != "died holding Flashbang, Smoke Grenade" {
		t.Errorf("detail = %q", got)
	}
	if res.Chances[KindUnusedUtility][t3] != 1 {
		t.Errorf("t3 chances = %d, want 1", res.Chances[KindUnusedUtility][t3])
	}
}

func TestBuyDesync(t *testing.T) {
	d := BuyDesync{FullBuy: 3700, Eco: 1500, Under: 2500, DropMoney: 3000}
	// team builds a five-player T side (IDs 100..104) and a CT side that
	// always full-buys.
	team := func(ts ...match.PlayerRound) []match.PlayerRound {
		ps := []match.PlayerRound{}
		for i, p := range ts {
			p.Player = match.PlayerID(100 + i)
			p.Side = match.SideT
			ps = append(ps, p)
		}
		for i := range 5 {
			ps = append(ps, match.PlayerRound{Player: match.PlayerID(200 + i), Side: match.SideCT, EquipmentValue: 5000})
		}
		return ps
	}
	full := match.PlayerRound{EquipmentValue: 4500, Money: 1000}
	rich := match.PlayerRound{EquipmentValue: 4500, Money: 5000}
	eco := match.PlayerRound{EquipmentValue: 800}
	broke := match.PlayerRound{EquipmentValue: 800, Money: 1000}
	tests := []struct {
		name   string
		round  []match.PlayerRound
		want   []string // kind:player
		detail string
	}{
		{"everyone buys", team(full, full, full, full, full), nil, ""},
		{"everyone saves", team(eco, eco, eco, eco, eco), nil, ""},
		{
			"under-buys with money", team(full, full, full, full, match.PlayerRound{EquipmentValue: 2000, Money: 4200}),
			[]string{"buy_desync:104"},
			"bought only $2000 of equipment with $4200 available while 4 teammates full-bought",
		},
		{
			"broke, a teammate could drop", team(full, rich, full, full, broke),
			[]string{"missed_drop:101"},
			"kept $5000 after buying while 104 ($800) stayed without a rifle buy",
		},
		{"broke, nobody could drop", team(full, full, full, full, broke), nil, ""},
		{"near-full buy is not out of line", team(full, full, full, full, match.PlayerRound{EquipmentValue: 3350, Money: 650}), nil, ""},
		{
			"full buy alone", team(eco, eco, eco, eco, full),
			[]string{"buy_desync:104"},
			"full-bought ($4500 of equipment) while 4 teammates saved",
		},
		{"two and three half buys", team(full, full, eco, eco, match.PlayerRound{EquipmentValue: 2500}), nil, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Round 1 is a pistol round and always skipped.
			m := newMatch(match.Round{Players: tt.round}, match.Round{Players: tt.round})
			res := d.Detect(m)
			var got []string
			for _, f := range res.Findings {
				if f.Round != 2 {
					t.Fatalf("finding in pistol round: %+v", f)
				}
				got = append(got, fmt.Sprintf("%s:%d", f.Kind, f.Player))
			}
			if fmt.Sprint(got) != fmt.Sprint(tt.want) {
				t.Fatalf("findings = %v, want %v", got, tt.want)
			}
			if tt.detail != "" && res.Findings[0].Detail != tt.detail {
				t.Errorf("detail = %q, want %q", res.Findings[0].Detail, tt.detail)
			}
		})
	}
}

func TestMissedDropChances(t *testing.T) {
	ps := []match.PlayerRound{
		{Player: t1, Side: match.SideT, EquipmentValue: 4500, Money: 3000},
		{Player: t2, Side: match.SideT, EquipmentValue: 4500, Money: 5000},
		{Player: t3, Side: match.SideT, EquipmentValue: 4500, Money: 100},
		{Player: c1, Side: match.SideT, EquipmentValue: 800, Money: 0},
	}
	res := BuyDesync{FullBuy: 3700, Eco: 1500, Under: 2500, DropMoney: 3000}.Detect(newMatch(match.Round{Players: ps}, match.Round{Players: ps}))
	got := res.Chances[KindMissedDrop]
	if got[t1] != 1 || got[t2] != 1 || got[t3] != 0 {
		t.Errorf("missed drop chances = %v, want t1 and t2 once, t3 never", got)
	}
}

func TestNoUtility(t *testing.T) {
	d := NoUtility{FullBuy: 3700, MinLife: 30 * time.Second}
	ps := roster(4000)
	ps[2].EquipmentValue = 1000 // t3 on an eco: not counted
	m := newMatch(match.Round{
		EndTick:  sec(90),
		Players:  ps,
		Grenades: []match.Grenade{{Tick: sec(20), Thrower: t1, Type: "Flashbang"}},
		Kills: []match.Kill{
			kill(10, c1, t2), // t2 died early: not counted
			kill(40, t1, c2), // c2 lived 40s without throwing: flagged
		},
	})
	res := d.Detect(m)
	assertFindings(t, res.Findings, "no_utility:c1", "no_utility:c2", "no_utility:c3")
	ch := res.Chances[KindNoUtility]
	if ch[t1] != 1 || ch[t2] != 0 || ch[t3] != 0 || ch[c2] != 1 {
		t.Errorf("chances = %v", ch)
	}
}

func TestPistolRoundAfterSideSwitch(t *testing.T) {
	first := roster(3000)
	switched := roster(3000)
	for i := range switched {
		if switched[i].Side == match.SideT {
			switched[i].Side = match.SideCT
		} else {
			switched[i].Side = match.SideT
		}
	}
	m := newMatch(match.Round{Players: first}, match.Round{Players: first}, match.Round{Players: switched}, match.Round{Players: switched})
	got := []bool{pistolRound(m, 0), pistolRound(m, 1), pistolRound(m, 2), pistolRound(m, 3)}
	want := []bool{true, false, true, false}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("pistol rounds = %v, want %v", got, want)
		}
	}
}

func TestOpeningDeath(t *testing.T) {
	d := OpeningDeath{SupportWindow: 2 * time.Second}
	tests := []struct {
		name    string
		kills   []match.Kill
		flashes []match.Flash
		want    []string
	}{
		{"dry opening death", []match.Kill{kill(15, c1, t1), kill(20, t2, c1)}, nil, []string{"opening_death:t1"}},
		{"flashed for", []match.Kill{kill(15, c1, t1)}, []match.Flash{flash(14, t2, blind(c1, 2*time.Second))}, nil},
		{"flash too early", []match.Kill{kill(15, c1, t1)}, []match.Flash{flash(10, t2, blind(c1, 2*time.Second))}, []string{"opening_death:t1"}},
		{"flash only hit own team", []match.Kill{kill(15, c1, t1)}, []match.Flash{flash(14, t2, blind(t1, 2*time.Second))}, []string{"opening_death:t1"}},
		{"round opened by team kill", []match.Kill{{Tick: sec(5), Killer: t2, Victim: t1, TeamKill: true}, kill(15, c1, t3)}, nil, nil},
		{"no kills", nil, nil, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res := d.Detect(newMatch(match.Round{Kills: tt.kills, Flashes: tt.flashes}))
			assertFindings(t, res.Findings, tt.want...)
		})
	}
}

func TestNoUtilityIgnoresShortRounds(t *testing.T) {
	m := newMatch(match.Round{EndTick: sec(20), Players: roster(4000)})
	res := NoUtility{FullBuy: 3700, MinLife: 30 * time.Second}.Detect(m)
	if len(res.Findings) != 0 || len(res.Chances[KindNoUtility]) != 0 {
		t.Fatalf("a 20 s round produced %d findings", len(res.Findings))
	}
}
