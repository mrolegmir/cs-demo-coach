package demo

import (
	"testing"

	"github.com/mrolegmir/cs-demo-coach/internal/match"
)

func TestMovementBufferBoundedAndMissingIntervals(t *testing.T) {
	b := &builder{positions: map[match.PlayerID]positionSample{}, movement: map[match.PlayerID][]match.MovementSample{}}
	for tick := range 1000 {
		b.recordMovement(1, positionSample{tick: tick, pos: match.Vec{X: float64(tick)}}, false, 64, nil)
	}
	if len(b.movement[1]) != 33 || b.movement[1][0].Tick != 967 || *b.movement[1][32].Speed != 64 {
		t.Fatal("history is unbounded or speed is incorrect")
	}
	b.recordMovement(1, positionSample{tick: 999, pos: match.Vec{X: 9999}}, true, 64, nil)
	if b.positions[1].pos.X != 999 || len(b.movement[1]) != 33 {
		t.Fatal("duplicate tick overwrote movement state")
	}
	b.recordMovement(1, positionSample{tick: 1010, pos: match.Vec{X: 1010}}, false, 64, nil)
	last := b.movement[1][len(b.movement[1])-1]
	if last.Speed != nil || last.WindowTicks != 0 {
		t.Fatal("long gap presented as measured movement")
	}
	b.recordMovement(1, positionSample{tick: 1011, pos: match.Vec{X: 9999}}, true, 64, nil)
	last = b.movement[1][len(b.movement[1])-1]
	if last.Speed != nil || !last.Airborne {
		t.Fatal("teleport presented as speed or airborne state lost")
	}
	b.recordMovement(0, positionSample{tick: 1011}, false, 64, nil)
	if len(b.movement) != 1 {
		t.Fatal("invalid participant captured")
	}
}

func TestMovementBeforeCopiesOnlyRecentPast(t *testing.T) {
	history := []match.MovementSample{
		{Tick: 1, Speed: new(250.0)}, // Outside half-second window.
		{Tick: 32, Speed: new(200.0), WindowTicks: 1},
		{Tick: 63},                    // Unknown is preserved, not converted to zero.
		{Tick: 64, Speed: new(123.0)}, // Same tick replaced by shot-time sample.
		{Tick: 65, Speed: new(250.0)}, // Future must never leak into preparation.
	}
	shot := match.Shot{Tick: 64, Speed: new(0.0), SpeedWindowTicks: 1, Airborne: true}
	got := movementBefore(history, shot, 64)
	if len(got) != 3 || got[0].Tick != 32 || got[1].Speed != nil || got[2].Speed == nil || *got[2].Speed != 0 || !got[2].Airborne {
		t.Fatalf("preparation = %+v", got)
	}
	history[1].Tick = 999
	if got[0].Tick != 32 {
		t.Fatal("shot history aliases rolling buffer")
	}
	if movementBefore(history, shot, 0) != nil {
		t.Fatal("invalid tick rate must not produce measurements")
	}
}

func TestContactPairsOnlyExactEarlierFrames(t *testing.T) {
	a := []match.MovementSample{{Tick: 10}, {Tick: 40, Speed: new(0.0), View: &match.ViewSample{Eye: &match.Vec{}}}, {Tick: 63}, {Tick: 64}}
	v := []match.MovementSample{{Tick: 40, View: &match.ViewSample{Eye: &match.Vec{X: 100}}}, {Tick: 62, View: &match.ViewSample{Eye: &match.Vec{X: 200}}}}
	got := contactBefore(a, v, 64, 64)
	if len(got) != 2 || got[0].Tick != 40 || got[0].TargetEye == nil || got[1].TargetEye != nil || got[0].Speed == nil || *got[0].Speed != 0 {
		t.Fatalf("contact pairing: %+v", got)
	}
	a[1].Tick = 999
	if got[0].Tick != 40 {
		t.Fatal("rolling history aliases retained contact")
	}
	if contactBefore(a, v, 64, 0) != nil {
		t.Fatal("invalid time scale used")
	}
}
