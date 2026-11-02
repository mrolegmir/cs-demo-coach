package match

import "math"

// Shot captures observations, not an aim or counter-strafe diagnosis.
type Shot struct {
	Tick   int      `json:"tick"`
	Player PlayerID `json:"player"`
	Weapon string   `json:"weapon"`
	Pos    Vec      `json:"pos"`
	Yaw    float64  `json:"yaw"`
	Pitch  float64  `json:"pitch"`
	// Speed is horizontal displacement per second since a recent demo frame.
	// Nil means unavailable; it must never be interpreted as standing still.
	Speed            *float64         `json:"speed,omitzero"`
	SpeedWindowTicks int              `json:"speed_window_ticks,omitzero"`
	Airborne         bool             `json:"airborne,omitzero"`
	Place            string           `json:"place,omitempty"`
	Preparation      []MovementSample `json:"preparation,omitempty"`
	View             *ViewSample      `json:"view,omitzero"`
}

// MovementSample is frame-averaged movement, not instantaneous velocity or input.
// Tick and WindowTicks identify the interval ending at this sample.
type MovementSample struct {
	Tick        int         `json:"tick"`
	WindowTicks int         `json:"window_ticks,omitzero"`
	Speed       *float64    `json:"speed,omitzero"`
	Airborne    bool        `json:"airborne,omitzero"`
	View        *ViewSample `json:"view,omitzero"`
}

// ViewSample is the sampled view direction, not a recoil-corrected bullet ray.
// Eye is nil unless the parser had networked eye-offset fields.
type ViewSample struct {
	Yaw   float64 `json:"yaw"`
	Pitch float64 `json:"pitch"`
	Eye   *Vec    `json:"eye,omitzero"`
}

// ContactSample pairs exact same-frame observations with a later damaged
// player's eye point. It does not establish visibility or intended target.
type ContactSample struct {
	MovementSample
	TargetEye *Vec `json:"target_eye,omitzero"`
}

// ShotSpeed estimates recent horizontal speed. Long gaps and teleports are
// unavailable rather than evidence of movement. This is not subtick velocity.
func ShotSpeed(before, after Vec, ticks int, tickRate float64) *float64 {
	if tickRate <= 0 || ticks <= 0 || float64(ticks)/tickRate > 0.1 {
		return nil
	}
	speed := math.Hypot(after.X-before.X, after.Y-before.Y) * tickRate / float64(ticks)
	if math.IsNaN(speed) || math.IsInf(speed, 0) || speed > 400 {
		return nil
	}
	return new(speed)
}

// ShootingSequence is a descriptive group of same-weapon shots separated by
// at most 250 ms. It does not establish trigger holds, targets or a duel.
type ShootingSequence struct {
	Player                                            PlayerID
	Round, Tick                                       int
	Weapon                                            string
	Shots, SpeedSamples, MovingSamples, AirborneShots int
	Place                                             string
	HitShots, HealthDamage                            int
	// FirstHitShot is one-based; zero means no linked enemy damage was recorded.
	FirstHitShot int
	Preparation  []MovementSample
	ShotIndices  []int
}

// ShootingSequences returns stable, round-local rifle sequences. Other weapon
// classes need their own interpretation and are outside this pilot.
func (m *Match) ShootingSequences() []ShootingSequence {
	var out []ShootingSequence
	if m.MechanicsVersion < 1 || m.MechanicsVersion > 4 || m.TickRate <= 0 {
		return out
	}
	for _, r := range m.Rounds {
		outcomes := r.ShotOutcomes()
		type lastShot struct{ index, shot int }
		last := map[PlayerID]lastShot{}
		for si, s := range r.Shots {
			if !ReviewRifle(s.Weapon) {
				delete(last, s.Player)
				continue
			}
			prev, ok := last[s.Player]
			if !ok || !SameRifleSequence(r.Shots[prev.shot], s, m.TickRate) {
				out = append(out, ShootingSequence{Player: s.Player, Round: r.Number, Tick: s.Tick, Weapon: s.Weapon, Place: s.Place})
				prev.index = len(out) - 1
				if m.MechanicsVersion >= 3 {
					out[prev.index].Preparation = s.Preparation
				}
			}
			seq := &out[prev.index]
			seq.Shots++
			seq.ShotIndices = append(seq.ShotIndices, si)
			if m.MechanicsVersion >= 2 && outcomes[si].Events > 0 {
				seq.HitShots++
				seq.HealthDamage += outcomes[si].HealthDamage
				if seq.FirstHitShot == 0 {
					seq.FirstHitShot = seq.Shots
				}
			}
			if s.Airborne {
				seq.AirborneShots++
			}
			if s.Speed != nil {
				seq.SpeedSamples++
				// A descriptive fixed cutoff, not a weapon accuracy threshold.
				if *s.Speed > 80 {
					seq.MovingSamples++
				}
			}
			last[s.Player] = lastShot{prev.index, si}
		}
	}
	return out
}

// ReviewRifle identifies assault rifles supported by the mechanics pilot.
func ReviewRifle(weapon string) bool {
	switch weapon {
	case "AK-47", "M4A4", "M4A1", "M4A1-S", "FAMAS", "Galil AR", "AUG", "SG 553":
		return true
	default:
		return false
	}
}

// SameRifleSequence is shared by capture and review so only first-shot history
// is retained without changing grouping semantics.
func SameRifleSequence(before, after Shot, rate float64) bool {
	return rate > 0 && ReviewRifle(before.Weapon) && before.Player == after.Player && before.Weapon == after.Weapon && after.Tick >= before.Tick && float64(after.Tick-before.Tick)/rate <= 0.25
}
