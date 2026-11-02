package match

import (
	"math"
	"testing"
)

func TestViewToEyeGeometryAndUnknown(t *testing.T) {
	for _, tt := range []struct {
		yaw, pitch float64
		target     Vec
		want       float64
	}{
		{0, 0, Vec{X: 100}, 0},
		{90, 0, Vec{X: 100}, 90},
		{359, 0, Vec{X: math.Cos(math.Pi/180) * 100, Y: math.Sin(math.Pi/180) * 100}, 2},
		{0, 90, Vec{Z: -100}, 0},
		{0, 270, Vec{Z: 100}, 0},
	} {
		got := ViewToEyeAngle(&ViewSample{Yaw: tt.yaw, Pitch: tt.pitch, Eye: &Vec{}}, &tt.target)
		if got == nil || math.Abs(*got-tt.want) > 1e-5 {
			t.Fatalf("angle %v want %v", got, tt.want)
		}
	}
	for _, view := range []*ViewSample{nil, {}, {Eye: &Vec{}}, {Eye: &Vec{}, Yaw: math.NaN()}} {
		if ViewToEyeAngle(view, &Vec{}) != nil {
			t.Fatal("unknown/degenerate geometry became a measured angle")
		}
	}
	if ViewToEyeAngle(&ViewSample{Eye: &Vec{}}, nil) != nil {
		t.Fatal("missing target became zero angle")
	}
}

func aimFixture() *Match {
	return &Match{MechanicsVersion: 4, TickRate: 64, Players: []Player{{ID: 1, Name: "private"}, {ID: 2, Name: "target"}, {ID: 3, Name: "other"}}, Rounds: []Round{{Number: 1, Players: []PlayerRound{{Player: 1, Side: SideT}}, Shots: []Shot{
		{Tick: 100, Player: 1, Weapon: "AK-47", Speed: new(200.0), Preparation: []MovementSample{{Tick: 99, Speed: new(220.0)}}},
		{Tick: 106, Player: 1, Weapon: "AK-47", Speed: new(0.0)},
	}, Damage: []Damage{{Tick: 106, Attacker: 1, Victim: 2, Weapon: "AK-47", HealthDamage: 20, Preparation: []ContactSample{{MovementSample: MovementSample{Tick: 105, Speed: new(0.0), View: &ViewSample{Eye: &Vec{}}}, TargetEye: &Vec{X: 100}}}}}}}}
}

func TestRifleEpisodesPreserveContactAndMovement(t *testing.T) {
	m := aimFixture()
	e := m.RifleEpisodes(1)
	if len(e) != 1 || e[0].Contact == nil || e[0].Contact.Victim != 2 || e[0].FirstHitShot != 2 || len(e[0].Fires) != 2 || *e[0].Fires[1].Speed != 0 || e[0].Fires[1].HealthDamage != 20 || len(e[0].Preparation) != 1 {
		t.Fatalf("episode: %+v", e)
	}
	m.Rounds[0].Damage = append(m.Rounds[0].Damage, Damage{Tick: 106, Attacker: 1, Victim: 3, Weapon: "AK-47", HealthDamage: 10})
	if got := m.RifleEpisodes(1)[0]; got.Contact != nil || !got.ContactAmbiguous {
		t.Fatal("multiple first-hit targets silently chose one")
	}
	if len(m.RifleEpisodes(99)) != 0 {
		t.Fatal("another participant assigned to viewer")
	}
	m = aimFixture()
	m.MechanicsVersion = 3
	if got := m.RifleEpisodes(1)[0]; got.Contact == nil || len(got.Contact.Preparation) != 0 || len(got.Preparation) != 1 {
		t.Fatal("legacy contact history guessed or movement lost")
	}
	m.MechanicsVersion = 1
	if got := m.RifleEpisodes(1)[0]; got.Contact != nil || got.Fires[1].HealthDamage != 0 {
		t.Fatal("legacy damage presented as captured")
	}
	m.MechanicsVersion = 0
	if len(m.RifleEpisodes(1)) != 0 {
		t.Fatal("uncaptured shots assessed")
	}
	m = aimFixture()
	m.Anonymize()
	if got := m.RifleEpisodes(1)[0]; got.Contact.Victim != 2 || m.PlayerName(1) != "player01" {
		t.Fatal("contact identities not anonymized")
	}
}
