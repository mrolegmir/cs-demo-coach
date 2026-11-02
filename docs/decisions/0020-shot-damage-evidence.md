# 20. Attribute damage conservatively before calibrating aim feedback

**Status:** accepted for local pilot; not deployed

## Decision

Capture version 2 adds firearm PlayerHurt events, with attacker/victim, weapon,
tick, health damage taken, hitgroup and event-time distance in world units.
Only valid human participants and firearm equipment instances are retained.
Grenade, world and knife damage are excluded. The pinned parser can report zero
HealthDamageTaken for some events; these events are retained for audit but do
not count as positive health damage. No raw overkill fallback is used.

Link a damage event only when exactly one shot in that round matches attacker,
weapon and tick. Resolve after parsing so callback order does not matter.
Ambiguous/missing candidates remain unmatched. Friendly, self, unknown-player
and zero-health-damage events cannot provide enemy-hit evidence. One bullet can
damage several enemies but counts once as a shot with damage.

Sequences now expose linked hit-shot count, actual health damage and the ordinal
of the first shot with linked damage. No linked damage is not a miss. The match
page offers round/time evidence and attribution coverage; capture versions 0/1
and demos with no usable firearm damage remain distinguishable.

The anonymized `csdc mechanics` JSON export includes sequence evidence, individual
damage events (including unmatched ones) and exploratory buckets by weapon,
sequence length and first-shot movement. Buckets do not control for visibility,
range, target movement or intended shots and must not be treated as accuracy
benchmarks. Damage distance exists only for recorded hits, so using it alone to
infer a distance-specific hit rate would bias the denominator.

## Validation outcome

Current Dust2 has 497 positive enemy firearm damage events, 495 uniquely linked;
Inferno has 339, 336 linked. Two older fixtures produce no usable firearm damage
events. They show unavailable outcomes rather than zero accuracy.

The current Dust2 AK-47 sample contains only one sequence starting above the
80 units/s descriptive cutoff while grounded. It cannot calibrate a movement
mistake threshold. Long sequences also include successful damage; length alone
does not justify a short-burst recommendation. No new mechanical finding, drill,
history category or LLM claim is enabled from this sample.

## Next evidence needed

Episode labels need intended target/contact, visibility/smoke/cover and independent
review of successes as well as failures. Then gather comparable samples across
weapons/ranges and validate on separate matches. This release provides reviewable
data for that work; it does not claim a completed recoil or counter-strafe model.
