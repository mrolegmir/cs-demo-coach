package demo

import "github.com/mrolegmir/cs-demo-coach/internal/match"

// Keep only half a second per living player. History is copied into shots,
// never retained as a full-match frame stream.
func (b *builder) recordMovement(id match.PlayerID, now positionSample, airborne bool, tickRate float64, view *match.ViewSample) {
	if id == 0 {
		return
	}
	s := match.MovementSample{Tick: now.tick, Airborne: airborne, View: view}
	if prev, ok := b.positions[id]; ok {
		if now.tick <= prev.tick {
			return // Duplicate frames must not replace the last distinct position.
		}
		s.Speed = match.ShotSpeed(prev.pos, now.pos, now.tick-prev.tick, tickRate)
		if s.Speed != nil {
			s.WindowTicks = now.tick - prev.tick
		}
	}
	b.positions[id] = now
	history := b.movement[id]
	start := 0
	for start < len(history) && float64(now.tick-history[start].Tick)/tickRate > 0.5 {
		start++
	}
	// Reuse the bounded buffer for samples still inside the window.
	history = append(history[:0], history[start:]...)
	b.movement[id] = append(history, s)
}

func movementBefore(history []match.MovementSample, shot match.Shot, tickRate float64) []match.MovementSample {
	if tickRate <= 0 {
		return nil
	}
	var out []match.MovementSample
	for _, s := range history {
		if s.Tick < shot.Tick && float64(shot.Tick-s.Tick)/tickRate <= 0.5 {
			out = append(out, s)
		}
	}
	return append(out, match.MovementSample{Tick: shot.Tick, WindowTicks: shot.SpeedWindowTicks, Speed: shot.Speed, Airborne: shot.Airborne, View: shot.View})
}

// Only completed frames before damage are retained. Pair exact ticks without
// interpolation or using death-time eye positions as historical positions.
func contactBefore(attacker, target []match.MovementSample, tick int, rate float64) []match.ContactSample {
	if rate <= 0 {
		return nil
	}
	var out []match.ContactSample
	for _, s := range attacker {
		if s.Tick >= tick || float64(tick-s.Tick)/rate > 0.5 {
			continue
		}
		v := match.ContactSample{MovementSample: s}
		for _, t := range target {
			if t.Tick == s.Tick && t.View != nil {
				v.TargetEye = t.View.Eye
				break
			}
		}
		out = append(out, v)
	}
	return out
}
