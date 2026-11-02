package match

import (
	"math"
	"path/filepath"
	"testing"
	"time"
)

func TestFlashFollowupAnonymizationAndRealDemoAudit(t *testing.T) {
	m := flashFixture()
	m.Players[0].ID = 123456
	m.Players[0].Name = "private name"
	m.Rounds[0].Players[0].Player = 123456
	m.Rounds[0].Flashes[0].Thrower = 123456
	m.Anonymize()
	if got := m.FlashSupportFor(1); got.TeammateKills != 1 || got.Kills[0].Sources[0].Thrower != 1 || m.Players[0].Name != "player01" {
		t.Fatal("anonymization changed or leaked flash evidence")
	}
	positive := 0
	for _, name := range []string{"cs2_valve_mm_8thAug23_ancient", "Anubis_ShortMatch_2023-08-04", "arsham_infernocs2_1stSept23", "mm_2026_dust2"} {
		m, err := LoadFile(filepath.Join("..", "..", "testdata", "matches", name+".json.gz"))
		if err != nil {
			t.Fatal(err)
		}
		outgoing, incoming, bursts := 0, 0, 0
		for _, p := range m.Players {
			rep := m.FlashSupportFor(p.ID)
			outgoing += rep.TeammateKills
			incoming += rep.SupportedKills
			bursts += len(rep.Flashes)
			for _, k := range rep.Kills {
				if k.State == FlashUnique && (len(k.Sources) != 1 || k.Delay <= 0) {
					t.Fatal("invalid unique match")
				}
			}
		}
		if outgoing != incoming {
			t.Fatal("team support counted differently for thrower and killer")
		}
		positive += outgoing
		t.Logf("%s: bursts=%d uniquely matched teammate kills=%d", name, bursts, outgoing)
	}
	if positive == 0 {
		t.Fatal("no real-demo flash support evidence")
	}
}

func flashFixture() *Match {
	return &Match{TickRate: 64, Players: []Player{{ID: 1, Name: "one"}, {ID: 2, Name: "two"}, {ID: 3, Name: "three"}, {ID: 4, Name: "four"}}, Rounds: []Round{{
		Number: 1, FreezeEndTick: 64, EndTick: 512,
		Players: []PlayerRound{{Player: 1, Side: SideT}, {Player: 2, Side: SideT}, {Player: 3, Side: SideCT}, {Player: 4, Side: SideCT}},
		Flashes: []Flash{{Tick: 100, Thrower: 1, Side: SideT, Blinded: []Blinding{{Player: 3, Side: SideCT, Duration: 2 * time.Second}}}},
		Kills:   []Kill{{Tick: 164, Killer: 2, Victim: 3, Alive: []PlayerState{{Player: 2, Side: SideT}, {Player: 3, Side: SideCT}}}},
	}}}
}

func TestFlashFollowupHasTwoPersonalPerspectives(t *testing.T) {
	m := flashFixture()
	thrower := m.FlashSupportFor(1)
	if len(thrower.Flashes) != 1 || thrower.Flashes[0].EnemyTargets != 1 || thrower.TeammateKills != 1 || thrower.SupportedKills != 0 || len(thrower.Kills) != 1 || thrower.Kills[0].State != FlashUnique || thrower.Kills[0].Delay != time.Second {
		t.Fatalf("thrower: %+v", thrower)
	}
	killer := m.FlashSupportFor(2)
	if killer.SupportedKills != 1 || killer.TeammateKills != 0 || len(killer.Flashes) != 0 {
		t.Fatalf("killer: %+v", killer)
	}
	if len(m.FlashSupportFor(3).Kills) != 0 || len(m.FlashSupportFor(99).Kills) != 0 {
		t.Fatal("another participant's support assigned to viewer")
	}
	// The thrower is absent from the kill's alive snapshot; posthumous support
	// remains valid when the recorded thrower side is known.
	if thrower.Kills[0].Sources[0].Thrower != 1 {
		t.Fatal("dead thrower discarded")
	}
	m.Rounds[0].Kills[0].Killer = 1
	m.Rounds[0].Kills[0].Alive[0].Player = 1
	self := m.FlashSupportFor(1)
	if self.TeammateKills != 0 || self.SupportedKills != 0 || self.Kills[0].State != FlashUnique {
		t.Fatal("own-flash kill counted as teammate support")
	}
}

func TestFlashOverlapBlocksIndividualCredit(t *testing.T) {
	for _, tt := range []struct {
		name  string
		flash Flash
	}{
		{"ally", Flash{Tick: 110, Thrower: 2, Side: SideT}},
		{"enemy", Flash{Tick: 110, Thrower: 4, Side: SideCT}},
		{"unknown", Flash{Tick: 110}},
		{"same tick", Flash{Tick: 164, Thrower: 4, Side: SideCT}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			m := flashFixture()
			f := tt.flash
			f.Blinded = []Blinding{{Player: 3, Side: SideCT, Duration: 2 * time.Second}}
			m.Rounds[0].Flashes = append(m.Rounds[0].Flashes, f)
			got := m.FlashSupportFor(1)
			if got.TeammateKills != 0 || got.AmbiguousKills != 1 || len(got.Kills) != 1 || len(got.Kills[0].Sources) != 2 {
				t.Fatalf("ambiguous: %+v", got)
			}
		})
	}
}

func TestFlashTemporalBoundariesAndMissingSides(t *testing.T) {
	for _, tt := range []struct {
		name                string
		mutate              func(*Match)
		links, unclassified int
	}{
		{"before burst", func(m *Match) { m.Rounds[0].Kills[0].Tick = 99 }, 0, 0},
		{"same tick", func(m *Match) { m.Rounds[0].Kills[0].Tick = 100 }, 1, 1},
		{"at interval end", func(m *Match) { m.Rounds[0].Kills[0].Tick = 228 }, 0, 0},
		{"after round end", func(m *Match) { m.Rounds[0].Kills[0].Tick = 513 }, 0, 0},
		{"zero duration", func(m *Match) { m.Rounds[0].Flashes[0].Blinded[0].Duration = 0 }, 0, 0},
		{"unknown victim side", func(m *Match) { m.Rounds[0].Flashes[0].Blinded[0].Side = SideNone }, 1, 1},
		{"unknown source", func(m *Match) { m.Rounds[0].Flashes[0].Side = SideNone }, 1, 1},
		{"missing killer snapshot", func(m *Match) { m.Rounds[0].Kills[0].Alive = m.Rounds[0].Kills[0].Alive[1:] }, 1, 1},
		{"duplicate killer", func(m *Match) {
			m.Rounds[0].Kills[0].Alive = append(m.Rounds[0].Kills[0].Alive, m.Rounds[0].Kills[0].Alive[0])
		}, 1, 1},
		{"teamkill", func(m *Match) { m.Rounds[0].Kills[0].TeamKill = true }, 0, 0},
		{"world damage", func(m *Match) { m.Rounds[0].Kills[0].Killer = 0 }, 0, 0},
	} {
		t.Run(tt.name, func(t *testing.T) {
			m := flashFixture()
			tt.mutate(m)
			got := m.FlashSupportFor(1)
			if got.TeammateKills != 0 || len(got.Kills) != tt.links || got.UnclassifiedKills != tt.unclassified {
				t.Fatalf("boundary: %+v", got)
			}
		})
	}
	for _, rate := range []float64{0, math.NaN(), math.Inf(1)} {
		m := flashFixture()
		m.TickRate = rate
		got := m.FlashSupportFor(1)
		if got.TimingAvailable || len(got.Flashes) != 1 || len(got.Kills) != 0 {
			t.Fatal("missing timing treated as measured zero")
		}
	}
}

func TestFlashRoundLocalityAndDuplicateVictims(t *testing.T) {
	m := flashFixture()
	m.Rounds[0].Flashes[0].Blinded = append(m.Rounds[0].Flashes[0].Blinded, m.Rounds[0].Flashes[0].Blinded[0])
	got := m.FlashSupportFor(1)
	if got.Flashes[0].EnemyTargets != 1 || got.TeammateKills != 1 || len(got.Kills[0].Sources) != 1 {
		t.Fatal("duplicate victim inflated support")
	}
	r := m.Rounds[0]
	r.Number = 2
	r.Flashes = nil
	m.Rounds[0].Kills = nil
	m.Rounds = append(m.Rounds, r)
	if len(m.FlashSupportFor(1).Kills) != 0 {
		t.Fatal("flash interval leaked into another round")
	}
}

func TestFlashSidesShortBlindsAndRepeatedSources(t *testing.T) {
	m := flashFixture()
	m.Rounds[0].Flashes[0].Blinded[0].Duration = 20 * time.Millisecond
	m.Rounds[0].Kills[0].Tick = 101
	if m.FlashSupportFor(1).TeammateKills != 1 {
		t.Fatal("invented minimum blind duration")
	}
	m = flashFixture()
	m.Rounds[0].Flashes = append(m.Rounds[0].Flashes, m.Rounds[0].Flashes[0])
	if got := m.FlashSupportFor(1); got.AmbiguousKills != 1 || got.TeammateKills != 0 {
		t.Fatal("overlapping own flashes credited or counted twice")
	}
	m = flashFixture()
	m.Rounds[0].Number = 2
	m.Rounds[0].Flashes[0].Side = SideCT
	m.Rounds[0].Flashes[0].Blinded[0].Side = SideT
	m.Rounds[0].Kills[0].Alive[0].Side = SideCT
	m.Rounds[0].Kills[0].Alive[1].Side = SideT
	m.Rounds[0].Players[0].Side = SideCT
	m.Rounds = append([]Round{{Number: 1, Players: []PlayerRound{{Player: 1, Side: SideT}}}}, m.Rounds...)
	if m.FlashSupportFor(1).TeammateKills != 1 {
		t.Fatal("starting side used across side switch")
	}
	m = flashFixture()
	m.Rounds[0].Flashes[0].Thrower = 4
	m.Rounds[0].Flashes[0].Side = SideCT
	if got := m.FlashSupportFor(2); got.SupportedKills != 0 || got.UnclassifiedKills != 1 {
		t.Fatal("enemy flash became allied support")
	}
}
