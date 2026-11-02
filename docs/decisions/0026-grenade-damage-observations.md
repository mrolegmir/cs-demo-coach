# 26. Show recorded HE and fire damage without scoring grenade effectiveness

**Status:** accepted for v1.1.

## Decision

Utility capture version 1 stores HE, Molotov and incendiary PlayerHurt events
separately from firearm damage. Records contain attacker/victim IDs, event-time
sides, weapon, tick, victim's game-supplied area, non-overkill health damage when
available, and the raw reported amount for audit. Dead attackers are retained:
fire may keep causing damage after its thrower dies. Bots, unknown attackers,
world damage, bomb damage and other weapon types do not enter this capture.

Positive HealthDamageTaken is used as provided by the pinned parser. A positive
raw event with zero/unavailable non-overkill damage remains unknown; raw damage
does not substitute for actual health lost. A measured zero stays distinct from
unknown. No armor damage is counted.

The signed-in participant's Utility category adds a collapsed damage review:
throw counts and recorded enemy/teammate/self health damage by HE or Fire, plus
events with round/time, attacker side, victim, relation and victim's area.
Fire combines Molotov and incendiary because the network's inferno label does
not reliably distinguish their origins. Relations use sides at damage time,
not starting teams. Personal results do not depend on the selected party.

Unknown amounts and unclassified teams are excluded from totals and counted
separately; these counters can overlap. If no measured utility damage exists
anywhere in the demo, amounts are unavailable rather than presented as zero;
throw counts remain available. Known events do not prove complete coverage.

No attempt is made to link damage to individual throws. Repeated fire ticks are
events, not separate grenades, victims or successful throws. Area denial and
smoke quality remain unassessed. Zero recorded damage is not a bad-grenade
finding. Existing check statuses, coach priorities, drills, history and LLM
input are unchanged.

## Compatibility

Independent UtilityVersion avoids changing mechanics capture version 3.
Previously parsed matches lack utility damage and require a fresh parse.
Anonymization replaces both new event IDs. Stored parsed-match JSON carries the
new fields without a database migration. The parser dependency is unchanged.
New capture requires an updated home worker; the API supplies the new UI.

## Evidence

Four anonymized real demos were reparsed. Inferno yields 108 grenade damage
events and current Dust2 101, all with non-overkill amounts. Ancient and Anubis
yield no events and show unavailable damage totals. This establishes capture
on those fixtures, not universal format support or a calibrated utility score.
