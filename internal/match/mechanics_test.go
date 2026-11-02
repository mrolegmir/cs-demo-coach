package match

import "testing"

func TestPreparationAndDamageVersion3(t *testing.T) {
	m := &Match{MechanicsVersion: 3, TickRate: 64, Rounds: []Round{{
		Number: 1,
		Shots: []Shot{
			{Player: 1, Tick: 64, Weapon: "AK-47", Preparation: []MovementSample{{Tick: 63, Speed: new(200.0)}, {Tick: 64, Speed: new(0.0)}}},
			{Player: 1, Tick: 70, Weapon: "AK-47", Preparation: []MovementSample{{Tick: 70, Speed: new(50.0)}}},
			{Player: 2, Tick: 71, Weapon: "AK-47"},
		},
		Damage: []Damage{{Tick: 70, Attacker: 1, Victim: 2, Weapon: "AK-47", HealthDamage: 25}},
	}}}
	rep := m.MechanicsReport()
	if len(rep.Sequences) != 2 || len(rep.Sequences[0].Preparation) != 2 || rep.Sequences[0].Preparation[0].Tick != 63 || len(rep.Sequences[1].Preparation) != 0 {
		t.Fatalf("first-shot preparation lost or mixed: %+v", rep.Sequences)
	}
	if !rep.DamageAvailable || rep.LinkedEnemyDamageEvents != 1 || rep.Sequences[0].HitShots != 1 {
		t.Fatal("capture version 3 lost existing damage evidence")
	}
	m.MechanicsVersion = 2
	if len(m.ShootingSequences()[0].Preparation) != 0 {
		t.Fatal("legacy capture presented as movement history")
	}
}

func TestShotSpeed(t *testing.T) {
	if got := ShotSpeed(Vec{}, Vec{X: 3, Y: 4, Z: 100}, 2, 64); got == nil || *got != 160 {
		t.Fatalf("horizontal speed = %v, want 160", got)
	}
	for _, ticks := range []int{0, -1, 7} {
		if got := ShotSpeed(Vec{}, Vec{}, ticks, 64); got != nil {
			t.Fatalf("gap %d should be unknown", ticks)
		}
	}
	if ShotSpeed(Vec{}, Vec{X: 100}, 1, 64) != nil {
		t.Fatal("teleport must be unknown")
	}
	if got := ShotSpeed(Vec{}, Vec{}, 1, 64); got == nil || *got != 0 {
		t.Fatal("measured zero must be distinct from unknown")
	}
}

func TestShootingSequencesBoundaries(t *testing.T) {
	m := &Match{MechanicsVersion: 1, TickRate: 64, Rounds: []Round{
		{Number: 1, Shots: []Shot{
			{Player: 1, Weapon: "AK-47", Tick: 64, Speed: new(100.0)},
			{Player: 2, Weapon: "AK-47", Tick: 65},
			{Player: 1, Weapon: "AK-47", Tick: 80, Speed: new(0.0)},
			{Player: 1, Weapon: "AK-47", Tick: 97},
			{Player: 1, Weapon: "M4A1", Tick: 98, Airborne: true},
		}},
		{Number: 2, Shots: []Shot{{Player: 1, Weapon: "M4A1", Tick: 99}}},
	}}
	seq := m.ShootingSequences()
	if len(seq) != 5 {
		t.Fatalf("sequences = %+v", seq)
	}
	if seq[0].Shots != 2 || seq[0].SpeedSamples != 2 || seq[0].MovingSamples != 1 {
		t.Fatalf("first = %+v", seq[0])
	}
	if seq[1].SpeedSamples != 0 || seq[3].AirborneShots != 1 || seq[4].Round != 2 {
		t.Fatalf("sequences = %+v", seq)
	}
	m.MechanicsVersion = 0
	if len(m.ShootingSequences()) != 0 {
		t.Fatal("legacy analysis must be unavailable")
	}
}

func TestAnonymizeShots(t *testing.T) {
	m := &Match{Players: []Player{{ID: 123, Name: "private"}}, Rounds: []Round{{Shots: []Shot{{Player: 123}}}}}
	m.Anonymize()
	if m.Rounds[0].Shots[0].Player != 1 {
		t.Fatal("shot retained private ID")
	}
}
