package match

import "testing"

func TestObjectiveCaptureUniquePlantAndAnonymization(t *testing.T) {
	m := &Match{ObjectiveVersion: 1, Players: []Player{{ID: 111, Name: "private"}, {ID: 222, Name: "other"}}, Rounds: []Round{{Number: 1, Players: []PlayerRound{{Player: 111, Side: SideCT}}, BombEvents: []BombEvent{
		{Tick: 20, Kind: BombPlanted, Player: 222, Site: "A", AliveComplete: true, Alive: []PlayerState{{Player: 111, Side: SideCT}, {Player: 222, Side: SideT}}},
		{Tick: 30, Kind: BombDefuseBegin, Player: 111, HasKit: new(false)},
		{Tick: 40, Kind: BombExploded},
	}}}}
	got := m.ObjectiveRounds(111)
	if len(got) != 1 || got[0].Plant == nil || got[0].Side != SideCT || len(got[0].Events) != 3 || got[0].Events[1].HasKit == nil || *got[0].Events[1].HasKit {
		t.Fatal("objective evidence lost")
	}
	m.Anonymize()
	got = m.ObjectiveRounds(1)
	if got[0].Plant.Player != 2 || got[0].Plant.Alive[0].Player != 1 || got[0].Events[2].Player != 0 {
		t.Fatal("objective identities not anonymized")
	}
	m.Rounds[0].BombEvents = append(m.Rounds[0].BombEvents, m.Rounds[0].BombEvents[0])
	if m.ObjectiveRounds(1)[0].Plant != nil {
		t.Fatal("ambiguous plant assigned a time")
	}
	if len(m.ObjectiveRounds(99)) != 0 {
		t.Fatal("non-participant given objective rounds")
	}
	m.ObjectiveVersion = 0
	if len(m.ObjectiveRounds(1)) != 0 {
		t.Fatal("legacy capture treated as objectives")
	}
}
