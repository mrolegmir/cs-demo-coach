package match

import "math"

type RifleFire struct {
	Tick, Ordinal int
	Speed         *float64
	WindowTicks   int
	Airborne      bool
	HealthDamage  int
}

type RifleEpisode struct {
	ShootingSequence
	Side  Side
	Fires []RifleFire
	// Contact is the only positive enemy damage event linked to the first
	// damaging shot. Multiple victims/events do not choose a guessed target.
	Contact          *Damage
	ContactAmbiguous bool
}

func (m *Match) RifleEpisodes(player PlayerID) []RifleEpisode {
	var out []RifleEpisode
	if player == 0 || m.StartSide(player) == SideNone {
		return out
	}
	for _, seq := range m.ShootingSequences() {
		if seq.Player != player {
			continue
		}
		for _, r := range m.Rounds {
			if r.Number != seq.Round {
				continue
			}
			e := RifleEpisode{ShootingSequence: seq, Side: r.Side(player)}
			outcomes, links := r.ShotOutcomes(), r.DamageLinks()
			contactChecked := false
			for ordinal, si := range seq.ShotIndices {
				s := r.Shots[si]
				hp := 0
				if m.MechanicsVersion >= 2 {
					hp = outcomes[si].HealthDamage
				}
				e.Fires = append(e.Fires, RifleFire{Tick: s.Tick, Ordinal: ordinal + 1, Speed: s.Speed, WindowTicks: s.SpeedWindowTicks, Airborne: s.Airborne, HealthDamage: hp})
				if contactChecked || m.MechanicsVersion < 2 {
					continue
				}
				var candidates []Damage
				for di, link := range links {
					d := r.Damage[di]
					if link == si && !d.Friendly && d.Attacker == player && d.Victim != 0 && d.Victim != player && d.HealthDamage > 0 {
						candidates = append(candidates, d)
					}
				}
				if len(candidates) == 0 {
					continue
				}
				contactChecked = true
				if len(candidates) == 1 {
					e.Contact = new(candidates[0])
					if m.MechanicsVersion < 4 {
						e.Contact.Preparation = nil
					}
				} else {
					e.ContactAmbiguous = true
				}
			}
			out = append(out, e)
			break
		}
	}
	return out
}

// ViewToEyeAngle measures geometric separation from a sampled view direction
// to the later damaged player's eye point. It is not a hitbox, visibility,
// recoil-compensated bullet ray, intended target or aim-quality measurement.
func ViewToEyeAngle(view *ViewSample, target *Vec) *float64 {
	if view == nil || view.Eye == nil || target == nil {
		return nil
	}
	for _, v := range []float64{view.Yaw, view.Pitch, view.Eye.X, view.Eye.Y, view.Eye.Z, target.X, target.Y, target.Z} {
		if math.IsNaN(v) || math.IsInf(v, 0) {
			return nil
		}
	}
	dx, dy, dz := target.X-view.Eye.X, target.Y-view.Eye.Y, target.Z-view.Eye.Z
	n := math.Hypot(dx, math.Hypot(dy, dz))
	if n == 0 || math.IsInf(n, 0) {
		return nil
	}
	yaw, pitch := view.Yaw*math.Pi/180, view.Pitch*math.Pi/180
	dot := (math.Cos(pitch)*math.Cos(yaw)*dx + math.Cos(pitch)*math.Sin(yaw)*dy - math.Sin(pitch)*dz) / n
	angle := math.Acos(max(-1, min(1, dot))) * 180 / math.Pi
	return new(angle)
}
