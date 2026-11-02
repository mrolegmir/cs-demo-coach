package match

import "testing"

func TestDamageLinksConservative(t *testing.T) {
	r := Round{Shots: []Shot{
		{Tick: 10, Player: 1, Weapon: "AK-47"},
		{Tick: 11, Player: 1, Weapon: "AK-47"},
		{Tick: 11, Player: 1, Weapon: "AK-47"},
	}, Damage: []Damage{
		{Tick: 10, Attacker: 1, Victim: 2, Weapon: "AK-47", HealthDamage: 20},
		{Tick: 10, Attacker: 1, Victim: 3, Weapon: "AK-47", HealthDamage: 10},
		{Tick: 11, Attacker: 1, Victim: 2, Weapon: "AK-47", HealthDamage: 20},
		{Tick: 12, Attacker: 1, Victim: 2, Weapon: "AK-47", HealthDamage: 20},
		{Tick: 10, Attacker: 1, Victim: 2, Weapon: "M4A1", HealthDamage: 20},
		{Tick: 10, Attacker: 1, Victim: 2, Weapon: "AK-47", HealthDamage: 20, Friendly: true},
		{Tick: 10, Attacker: 2, Victim: 1, Weapon: "AK-47", HealthDamage: 20},
	}}
	want := []int{0, 0, -1, -1, -1, -1, -1}
	for i, got := range r.DamageLinks() {
		if got != want[i] {
			t.Fatalf("event %d linked to %d, want %d", i, got, want[i])
		}
	}
	out := r.ShotOutcomes()
	if out[0].Events != 2 || out[0].HealthDamage != 30 || out[1].Events != 0 {
		t.Fatalf("outcomes = %+v", out)
	}
}

func TestSequenceDamageAndAnonymization(t *testing.T) {
	m := &Match{MechanicsVersion: 2, TickRate: 64, Players: []Player{{ID: 100}, {ID: 200}}, Rounds: []Round{{
		Number: 1,
		Shots:  []Shot{{Player: 100, Tick: 10, Weapon: "AK-47"}, {Player: 100, Tick: 15, Weapon: "AK-47"}},
		Damage: []Damage{{Tick: 15, Attacker: 100, Victim: 200, Weapon: "AK-47", HealthDamage: 40}},
	}}}
	m.Anonymize()
	if m.Rounds[0].Damage[0].Attacker != 1 || m.Rounds[0].Damage[0].Victim != 2 {
		t.Fatal("private IDs retained")
	}
	s := m.ShootingSequences()
	if len(s) != 1 || s[0].HitShots != 1 || s[0].FirstHitShot != 2 || s[0].HealthDamage != 40 {
		t.Fatalf("sequence = %+v", s)
	}
	m.MechanicsVersion = 1
	if s = m.ShootingSequences(); s[0].HitShots != 0 {
		t.Fatal("legacy capture must not imply damage coverage")
	}
}
