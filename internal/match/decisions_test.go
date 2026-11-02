package match

import "testing"

func decisionFixture() *Match {
	return &Match{Rounds: []Round{{
		Number: 1, FreezeEndTick: 10, EndTick: 200, Winner: SideCT,
		Players: []PlayerRound{{Player: 1, Side: SideT}},
		Kills: []Kill{
			{Tick: 20, Victim: 11, Alive: []PlayerState{{Player: 1, Side: SideT}, {Player: 2, Side: SideT}, {Player: 3, Side: SideT}, {Player: 10, Side: SideCT}, {Player: 11, Side: SideCT}, {Player: 12, Side: SideCT}}},
			{Tick: 30, Victim: 12, Alive: []PlayerState{{Player: 1, Side: SideT}, {Player: 2, Side: SideT}, {Player: 3, Side: SideT}, {Player: 10, Side: SideCT}, {Player: 12, Side: SideCT}}},
		},
	}}}
}

func TestLostAdvantagesSelectsOneLargestRecordedSnapshot(t *testing.T) {
	m := decisionFixture()
	m.Rounds[0].Kills = append(m.Rounds[0].Kills, Kill{Tick: 40, Victim: 2}, Kill{Tick: 210, Victim: 3})
	got := m.LostAdvantages(1)
	if len(got) != 1 || got[0].Tick != 30 || got[0].OwnAlive != 3 || got[0].EnemyAlive != 1 || len(got[0].Alive) != 4 {
		t.Fatalf("review: %+v", got)
	}
	if len(m.Rounds[0].Kills[1].Alive) != 5 {
		t.Fatal("review mutated source snapshot")
	}
	if len(got[0].After) != 1 || got[0].After[0].Tick != 40 {
		t.Fatal("follow-up deaths crossed the review or round boundaries")
	}
	m.Rounds[0].Winner = SideT
	if len(m.LostAdvantages(1)) != 0 {
		t.Fatal("won round presented as a lost advantage")
	}
	m.Rounds[0].Winner = SideCT
	if len(m.LostAdvantages(99)) != 0 || len(m.LostAdvantages(0)) != 0 {
		t.Fatal("non-participant given another player's situation")
	}
}

func TestLostAdvantagesUsesRoundSideAfterSwitch(t *testing.T) {
	m := decisionFixture()
	m.Rounds[0].Players[0].Side = SideCT
	m.Rounds[0].Winner = SideT
	for i := range m.Rounds[0].Kills {
		for j := range m.Rounds[0].Kills[i].Alive {
			a := &m.Rounds[0].Kills[i].Alive[j]
			if a.Side == SideT {
				a.Side = SideCT
			} else {
				a.Side = SideT
			}
		}
	}
	got := m.LostAdvantages(1)
	if len(got) != 1 || got[0].Side != SideCT || got[0].OwnAlive != 3 {
		t.Fatal("starting team used instead of round side")
	}
	m.Rounds[0].Number = 2
	m.Rounds = append([]Round{{Number: 1, Players: []PlayerRound{{Player: 1, Side: SideT}}, Winner: SideT}}, m.Rounds...)
	if got = m.LostAdvantages(1); len(got) != 1 || got[0].Round != 2 {
		t.Fatal("starting side incorrectly used across halftime")
	}
}

func TestLostAdvantagesRejectsAmbiguousSnapshots(t *testing.T) {
	for _, tt := range []struct {
		name   string
		mutate func(*Round)
	}{
		{"viewer killed", func(r *Round) { r.Kills[0].Victim = 1 }},
		{"viewer absent", func(r *Round) { r.Kills[0].Alive = r.Kills[0].Alive[1:] }},
		{"victim absent", func(r *Round) { r.Kills[0].Victim = 99 }},
		{"duplicate player", func(r *Round) { r.Kills[0].Alive = append(r.Kills[0].Alive, r.Kills[0].Alive[0]) }},
		{"bot", func(r *Round) { r.Kills[0].Alive[1].Player = 0 }},
		{"unknown side", func(r *Round) { r.Kills[0].Alive[1].Side = SideNone }},
		{"same tick kills", func(r *Round) { r.Kills = append(r.Kills, r.Kills[0]) }},
		{"after round", func(r *Round) { r.Kills[0].Tick = 201 }},
		{"before freeze end", func(r *Round) { r.Kills[0].Tick = 9 }},
		{"unknown winner", func(r *Round) { r.Winner = SideNone }},
		{"no living opponent", func(r *Round) { r.Kills[0].Alive = r.Kills[0].Alive[:4]; r.Kills[0].Victim = 10 }},
	} {
		t.Run(tt.name, func(t *testing.T) {
			m := decisionFixture()
			m.Rounds[0].Kills = m.Rounds[0].Kills[:1]
			tt.mutate(&m.Rounds[0])
			if len(m.LostAdvantages(1)) != 0 {
				t.Fatal("invalid snapshot became review evidence")
			}
		})
	}
}
