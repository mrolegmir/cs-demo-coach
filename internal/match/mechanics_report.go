package match

import (
	"cmp"
	"slices"
)

// MechanicsReport is a reproducible exploration export, not coaching input.
// IDs in CLI exports are anonymized before this method is called.
type MechanicsReport struct {
	Version                                    int
	DamageAvailable                            bool
	EnemyDamageEvents, LinkedEnemyDamageEvents int
	Buckets                                    []ShootingBucket
	Sequences                                  []ShootingSequence
	DamageEvidence                             []DamageEvidence
	Episodes                                   []RifleEpisode
}

// DamageEvidence retains targets, distance and failed links for review.
// Shot is the zero-based index within the round, nil for an unmatched event.
type DamageEvidence struct {
	Round int
	Shot  *int
	Damage
}

// ShootingBucket separates weapon, sequence length and first-shot movement.
// It does not control target visibility, distance, rank or intent.
type ShootingBucket struct {
	Weapon, Length, FirstShotMovement        string
	Sequences, Shots, HitShots, HealthDamage int
}

func (m *Match) MechanicsReport() MechanicsReport {
	rep := MechanicsReport{Version: m.MechanicsVersion, Sequences: m.ShootingSequences()}
	for _, p := range m.Players {
		rep.Episodes = append(rep.Episodes, m.RifleEpisodes(p.ID)...)
	}
	for _, r := range m.Rounds {
		if m.MechanicsVersion < 2 || m.MechanicsVersion > 4 {
			continue
		}
		if len(r.Damage) > 0 {
			rep.DamageAvailable = true
		}
		for i, link := range r.DamageLinks() {
			d := r.Damage[i]
			e := DamageEvidence{Round: r.Number, Damage: d}
			if link >= 0 {
				e.Shot = new(link)
			}
			rep.DamageEvidence = append(rep.DamageEvidence, e)
			if d.Friendly || d.Attacker == 0 || d.Victim == 0 || d.Attacker == d.Victim || d.HealthDamage <= 0 {
				continue
			}
			rep.EnemyDamageEvents++
			if link >= 0 {
				rep.LinkedEnemyDamageEvents++
			}
		}
	}
	for _, s := range rep.Sequences {
		length := "single"
		if s.Shots > 5 {
			length = "long"
		} else if s.Shots > 1 {
			length = "short"
		}
		movement := "unknown"
		for _, r := range m.Rounds {
			if r.Number != s.Round {
				continue
			}
			for _, shot := range r.Shots {
				if shot.Player != s.Player || shot.Tick != s.Tick || shot.Weapon != s.Weapon {
					continue
				}
				switch {
				case shot.Airborne:
					movement = "airborne"
				case shot.Speed == nil:
				case *shot.Speed > 80:
					movement = "above_80"
				default:
					movement = "at_or_below_80"
				}
				break
			}
			break
		}
		index := slices.IndexFunc(rep.Buckets, func(b ShootingBucket) bool {
			return b.Weapon == s.Weapon && b.Length == length && b.FirstShotMovement == movement
		})
		if index < 0 {
			rep.Buckets = append(rep.Buckets, ShootingBucket{Weapon: s.Weapon, Length: length, FirstShotMovement: movement})
			index = len(rep.Buckets) - 1
		}
		b := &rep.Buckets[index]
		b.Sequences++
		b.Shots += s.Shots
		b.HitShots += s.HitShots
		b.HealthDamage += s.HealthDamage
	}
	slices.SortFunc(rep.Buckets, func(a, b ShootingBucket) int {
		return cmp.Or(cmp.Compare(a.Weapon, b.Weapon), cmp.Compare(a.Length, b.Length), cmp.Compare(a.FirstShotMovement, b.FirstShotMovement))
	})
	return rep
}
