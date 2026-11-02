package match

// Damage is a recorded firearm damage event. It does not imply visibility or
// identify the intended target of shots without damage.
type Damage struct {
	Tick         int             `json:"tick"`
	Attacker     PlayerID        `json:"attacker"`
	Victim       PlayerID        `json:"victim"`
	Weapon       string          `json:"weapon"`
	HealthDamage int             `json:"health_damage"`
	HitGroup     int             `json:"hit_group"`
	Distance     float64         `json:"distance"`
	Friendly     bool            `json:"friendly,omitzero"`
	Preparation  []ContactSample `json:"preparation,omitempty"`
}

// ShotOutcome contains only damage uniquely linked to a shot. No event does
// not mean a miss: prefires, smoke spam and missing events are indistinguishable.
type ShotOutcome struct {
	Events, HealthDamage int
}

// DamageLinks matches only a unique same-tick, same-attacker, same-weapon shot.
// A wider time window would risk assigning damage to a neighbouring bullet.
// -1 represents unmatched or ambiguous damage; friendly damage is excluded.
// Matching after parsing is independent of event callback ordering within a tick.
func (r *Round) DamageLinks() []int {
	type key struct {
		tick   int
		player PlayerID
		weapon string
	}
	shots := map[key]int{}
	for i, s := range r.Shots {
		k := key{s.Tick, s.Player, s.Weapon}
		if _, exists := shots[k]; exists {
			shots[k] = -1
		} else {
			shots[k] = i
		}
	}
	links := make([]int, len(r.Damage))
	for i, d := range r.Damage {
		links[i] = -1
		if d.Friendly || d.Attacker == 0 || d.Victim == 0 || d.Attacker == d.Victim || d.HealthDamage <= 0 {
			continue
		}
		if shot, ok := shots[key{d.Tick, d.Attacker, d.Weapon}]; ok {
			links[i] = shot
		}
	}
	return links
}

func (r *Round) ShotOutcomes() []ShotOutcome {
	out := make([]ShotOutcome, len(r.Shots))
	for i, shot := range r.DamageLinks() {
		if shot < 0 {
			continue
		}
		out[shot].Events++
		out[shot].HealthDamage += r.Damage[i].HealthDamage
	}
	return out
}
