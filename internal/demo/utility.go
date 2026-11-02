package demo

import (
	"github.com/markus-wa/demoinfocs-golang/v5/pkg/demoinfocs/events"
	"github.com/mrolegmir/cs-demo-coach/internal/match"
)

// Called only for HE/ fire PlayerHurt events in a live round. The thrower may
// already be dead when fire causes damage; do not filter on attacker life.
func (b *builder) onUtilityDamage(e events.PlayerHurt) {
	attacker, ok := b.track(e.Attacker)
	if !ok {
		return
	}
	victim, ok := b.track(e.Player)
	if !ok {
		return
	}
	b.cur.UtilityDamage = append(b.cur.UtilityDamage, match.UtilityDamage{
		Tick: b.tick(), Attacker: attacker, Victim: victim,
		AttackerSide: side(e.Attacker.Team), VictimSide: side(e.Player.Team),
		Weapon: e.Weapon.Type.String(), HealthDamage: utilityHealthDamage(e.HealthDamageTaken, e.HealthDamage),
		ReportedDamage: e.HealthDamage, VictimPlace: e.Player.LastPlaceName(),
	})
}

// A positive raw event with zero non-overkill damage occurs on unsupported
// fields/orderings. Keep it unknown rather than substituting overkill damage.
func utilityHealthDamage(taken, reported int) *int {
	if taken > 0 {
		return new(taken)
	}
	if taken == 0 && reported == 0 {
		return new(0)
	}
	return nil
}
