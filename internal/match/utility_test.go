package match

import "testing"

func TestUtilitySummaryUsesEventSidesAndSeparatesMissing(t *testing.T) {
	m := &Match{UtilityVersion: 1, Players: []Player{{ID: 111, Name: "private"}, {ID: 222, Name: "other"}}, Rounds: []Round{{
		Number: 1, Players: []PlayerRound{{Player: 111, Side: SideT}},
		Grenades: []Grenade{{Thrower: 111, Type: "HE Grenade"}, {Thrower: 111, Type: "Molotov"}, {Thrower: 222, Type: "HE Grenade"}},
		UtilityDamage: []UtilityDamage{
			{Attacker: 111, Victim: 222, Weapon: "HE Grenade", AttackerSide: SideT, VictimSide: SideCT, HealthDamage: new(17)},
			{Attacker: 111, Victim: 222, Weapon: "HE Grenade", AttackerSide: SideCT, VictimSide: SideCT, HealthDamage: new(3)},
			{Attacker: 111, Victim: 111, Weapon: "Molotov", HealthDamage: new(7)},
			{Attacker: 111, Victim: 222, Weapon: "Incendiary Grenade", AttackerSide: SideCT, VictimSide: SideT, HealthDamage: new(4)},
			{Attacker: 111, Victim: 222, Weapon: "Incendiary Grenade", AttackerSide: SideCT, VictimSide: SideT, ReportedDamage: 50},
			{Attacker: 111, Victim: 222, Weapon: "HE Grenade", HealthDamage: new(10)},
			{Attacker: 222, Victim: 111, Weapon: "HE Grenade", HealthDamage: new(99)},
			{Attacker: 111, Victim: 222, Weapon: "AK-47", HealthDamage: new(100)},
		},
	}}}
	rep := m.UtilityFor(111)
	if !rep.Captured || len(rep.Evidence) != 6 || len(rep.Summary) != 2 {
		t.Fatalf("report: %+v", rep)
	}
	he, fire := rep.Summary[0], rep.Summary[1]
	if he.Throws != 1 || he.EnemyDamage != 17 || he.TeamDamage != 3 || he.UnclassifiedEvents != 1 || fire.Throws != 1 || fire.EnemyDamage != 4 || fire.SelfDamage != 7 || fire.MissingDamageEvents != 1 {
		t.Fatalf("summary: %+v", rep.Summary)
	}
	m.Anonymize()
	for _, d := range m.Rounds[0].UtilityDamage {
		if d.Attacker > 2 || d.Victim > 2 {
			t.Fatal("utility identities not anonymized")
		}
	}
	if m.UtilityFor(1).Summary[0].EnemyDamage != 17 {
		t.Fatal("anonymization changed utility outcomes")
	}
	if len(m.UtilityFor(99).Summary) != 0 {
		t.Fatal("unknown participant assigned outcomes")
	}
	m.UtilityVersion = 0
	if m.UtilityFor(1).Captured || len(m.UtilityFor(1).Evidence) != 0 {
		t.Fatal("legacy capture treated as measured")
	}
}

func TestUtilityZeroIsDifferentFromMissing(t *testing.T) {
	m := &Match{UtilityVersion: 1, Rounds: []Round{{Players: []PlayerRound{{Player: 1, Side: SideCT}}, UtilityDamage: []UtilityDamage{
		{Attacker: 1, Victim: 2, Weapon: "HE Grenade", AttackerSide: SideCT, VictimSide: SideT, HealthDamage: new(0)},
	}}}}
	rep := m.UtilityFor(1)
	if !rep.Captured || rep.Summary[0].EnemyDamage != 0 || rep.Summary[0].MissingDamageEvents != 0 || rep.Evidence[0].HealthDamage == nil {
		t.Fatal("measured zero became unavailable")
	}
}
