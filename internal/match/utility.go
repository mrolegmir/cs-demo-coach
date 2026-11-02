package match

// UtilityDamage retains grenade health damage separately from firearm evidence.
// Nil HealthDamage means the event's non-overkill damage was unavailable.
// Fire is a family: the game may label Molotov damage as incendiary damage.
type UtilityDamage struct {
	Tick           int      `json:"tick"`
	Attacker       PlayerID `json:"attacker"`
	Victim         PlayerID `json:"victim"`
	AttackerSide   Side     `json:"attacker_side"`
	VictimSide     Side     `json:"victim_side"`
	Weapon         string   `json:"weapon"`
	HealthDamage   *int     `json:"health_damage,omitzero"`
	ReportedDamage int      `json:"reported_damage"`
	VictimPlace    string   `json:"victim_place,omitempty"`
}

// UtilityFamily deliberately combines Molotov and incendiary damage. It does
// not attribute an event to a particular grenade throw.
func UtilityFamily(weapon string) string {
	switch weapon {
	case "HE Grenade":
		return "HE"
	case "Molotov", "Incendiary Grenade":
		return "Fire"
	default:
		return ""
	}
}

// Relation classifies using event-time sides, including after halftime.
func (d UtilityDamage) Relation() string {
	if d.Attacker == 0 || d.Victim == 0 {
		return "Unknown"
	}
	if d.Attacker == d.Victim {
		return "Self"
	}
	if (d.AttackerSide != SideT && d.AttackerSide != SideCT) || (d.VictimSide != SideT && d.VictimSide != SideCT) {
		return "Unknown"
	}
	if d.AttackerSide == d.VictimSide {
		return "Teammate"
	}
	return "Enemy"
}

type UtilitySummary struct {
	Family                                      string
	Throws, EnemyDamage, TeamDamage, SelfDamage int
	MissingDamageEvents, UnclassifiedEvents     int
}

type UtilityEvidence struct {
	Round int
	UtilityDamage
}

type UtilityReport struct {
	Captured        bool
	DamageAvailable bool
	Summary         []UtilitySummary
	Evidence        []UtilityEvidence
}

// UtilityFor is a personal description of recorded outcomes, not a grenade
// effectiveness score. Damage cannot be linked to throws by timing alone.
func (m *Match) UtilityFor(player PlayerID) UtilityReport {
	rep := UtilityReport{Captured: m.UtilityVersion == 1}
	if !rep.Captured || player == 0 || m.StartSide(player) == SideNone {
		return rep
	}
	rep.Summary = []UtilitySummary{{Family: "HE"}, {Family: "Fire"}}
	for _, r := range m.Rounds {
		for _, g := range r.Grenades {
			if g.Thrower != player {
				continue
			}
			for i := range rep.Summary {
				if rep.Summary[i].Family == UtilityFamily(g.Type) {
					rep.Summary[i].Throws++
				}
			}
		}
		for _, d := range r.UtilityDamage {
			if UtilityFamily(d.Weapon) != "" && d.HealthDamage != nil && *d.HealthDamage >= 0 {
				rep.DamageAvailable = true
			}
			if d.Attacker != player {
				continue
			}
			family := UtilityFamily(d.Weapon)
			for i := range rep.Summary {
				s := &rep.Summary[i]
				if s.Family != family {
					continue
				}
				rep.Evidence = append(rep.Evidence, UtilityEvidence{Round: r.Number, UtilityDamage: d})
				relation := d.Relation()
				if relation == "Unknown" {
					s.UnclassifiedEvents++
				}
				if d.HealthDamage == nil || *d.HealthDamage < 0 {
					s.MissingDamageEvents++
					continue
				}
				switch relation {
				case "Enemy":
					s.EnemyDamage += *d.HealthDamage
				case "Teammate":
					s.TeamDamage += *d.HealthDamage
				case "Self":
					s.SelfDamage += *d.HealthDamage
				}
			}
		}
	}
	return rep
}
