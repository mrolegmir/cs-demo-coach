package main

import (
	"bytes"
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/mrolegmir/cs-demo-coach/internal/match"
)

func TestMechanicsExport(t *testing.T) {
	m := &match.Match{MechanicsVersion: 2, TickRate: 64, Players: []match.Player{{ID: 123456, Name: "private name"}, {ID: 654321, Name: "other name"}}, Rounds: []match.Round{{Number: 1, Shots: []match.Shot{{Tick: 10, Player: 123456, Weapon: "AK-47"}}, Damage: []match.Damage{{Tick: 10, Attacker: 123456, Victim: 654321, Weapon: "AK-47", HealthDamage: 20}}}}}
	path := filepath.Join(t.TempDir(), "match.json.gz")
	if err := match.SaveFile(path, m); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err := run(t.Context(), []string{"mechanics", path}, &out); err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(out.Bytes(), []byte("123456")) || bytes.Contains(out.Bytes(), []byte("private name")) {
		t.Fatal("private data in export")
	}
	var rep match.MechanicsReport
	if err := json.Unmarshal(out.Bytes(), &rep); err != nil {
		t.Fatal(err)
	}
	if rep.LinkedEnemyDamageEvents != 1 || len(rep.Buckets) != 1 || rep.Buckets[0].HitShots != 1 || rep.Buckets[0].FirstShotMovement != "unknown" {
		t.Fatalf("report = %+v", rep)
	}
	if rep.DamageEvidence[0].Attacker != 1 || rep.DamageEvidence[0].Victim != 2 || rep.DamageEvidence[0].Shot == nil || *rep.DamageEvidence[0].Shot != 0 {
		t.Fatal("incorrect damage evidence")
	}
}
