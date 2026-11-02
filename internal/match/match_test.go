package match

import (
	"testing"
	"time"
)

func TestClock(t *testing.T) {
	for d, want := range map[time.Duration]string{
		0:                       "0:00",
		-time.Second:            "0:00",
		1500 * time.Millisecond: "0:02",
		83 * time.Second:        "1:23",
	} {
		if got := Clock(d); got != want {
			t.Errorf("Clock(%v) = %q, want %q", d, got, want)
		}
	}
}

func TestAnonymize(t *testing.T) {
	m := &Match{
		Players: []Player{{ID: 76561198000000002, Name: "alice"}, {ID: 76561198000000001, Name: "bob"}},
		Rounds: []Round{{
			Players: []PlayerRound{{Player: 76561198000000001}},
			Kills: []Kill{{
				Killer: 76561198000000002, Victim: 76561198000000001,
				Alive: []PlayerState{{Player: 76561198000000001}, {Player: 76561198000000002}},
			}},
			Flashes:  []Flash{{Thrower: 76561198000000002, Blinded: []Blinding{{Player: 76561198000000001}}}},
			Grenades: []Grenade{{Thrower: 76561198000000001}},
		}},
	}
	m.Anonymize()

	if m.Players[0] != (Player{ID: 1, Name: "player01"}) || m.Players[1] != (Player{ID: 2, Name: "player02"}) {
		t.Fatalf("players = %+v", m.Players)
	}
	r := m.Rounds[0]
	k := r.Kills[0]
	got := []PlayerID{
		r.Players[0].Player, k.Killer, k.Victim, k.Assister, k.Alive[0].Player, k.Alive[1].Player,
		r.Flashes[0].Thrower, r.Flashes[0].Blinded[0].Player, r.Grenades[0].Thrower,
	}
	want := []PlayerID{2, 1, 2, 0, 2, 1, 1, 2, 2}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("ids = %v, want %v", got, want)
		}
	}
}

func TestPlaceName(t *testing.T) {
	for raw, want := range map[string]string{
		"BombsiteB": "B site", "TopofMid": "Top of Mid", "LongA": "Long A", "CTSpawn": "CT Spawn",
		"TSideUpper": "T Side Upper", "Banana": "Banana", "ExtendedA": "Extended A", "": "",
	} {
		if got := PlaceName(raw); got != want {
			t.Errorf("PlaceName(%q) = %q, want %q", raw, got, want)
		}
	}
	for raw, want := range map[string]string{"de_mirage": "Mirage", "cs_office": "Office", "mirage": "mirage"} {
		if got := MapName(raw); got != want {
			t.Errorf("MapName(%q) = %q, want %q", raw, got, want)
		}
	}
}
