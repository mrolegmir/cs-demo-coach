# 29. Review aim direction and movement around uniquely linked damage

**Status:** accepted for v1.1.

## Decision

Mechanics capture version 4 extends sampled movement with view yaw/pitch and
optional networked eye position. Eye coordinates are retained only when the
pinned parser's PositionEyes reports available eye-offset fields; feet plus a
guessed height are never substituted. View angles require their network field.

The half-second rolling buffers remain per living player and reset on death
and round start. Positive enemy firearm damage retains attacker samples from
completed frames strictly before the event, paired with the damaged player's
eye point at exactly the same frame tick. Missing pairs remain unknown. No
future samples or interpolation supply historical positions.

Personal rifle episodes are exposed inside Shooting and Movement. A sequence's
first shot with positive uniquely linked damage supplies a review target only
if exactly one eligible damage event belongs to that shot. Multiple events or
victims do not choose an arbitrary target. The recipient is a retrospectively
identified damage target, not a known intended or visible opponent.

The geometric angle is between the recorded view vector and the vector from
the shooter's eye to that player's eye at the earlier frame. Source pitch is
positive down; periodic trigonometry handles wrapped yaw/pitch. Degenerate or
missing coordinates produce unavailable angles, not zero. This is not a head
hitbox, recoil-corrected bullet ray, aim error or time-to-visible-contact.
Visibility, walls, smoke, recoil and player intent remain unobserved. Hit-only
selection cannot establish overall accuracy, reaction time or movement causation.

Movement review shows frame-averaged speed and airborne state before the first
shot and at every shot, alongside linked health damage when recorded. No speed
threshold, exact stop timing or key-press/counter-strafe diagnosis is introduced.
Both categories retain neutral state and expose available observations without
new findings, grades, priorities, drills, history categories or LLM claims.

## Retention and compatibility

Version 4 keeps full pre-shot preparation only at the first shot of each rifle
sequence; per-shot speed, airborne and view observations remain. Capture and
review share grouping rules (same player/weapon, non-negative gap <=250 ms).
This removes redundant histories without changing episode boundaries. Damage
context is bounded to 500 ms; full-match replay frames are not retained.

The real Dust2 detection request is 7,695,012 compact JSON bytes, below the
existing 32 MiB internal API limit. A regression marshals the actual request
shape and decodes it through the same bounded API decoder. Saved indented JSON
size is not the wire request size. The API limit is unchanged.

Versions 1–3 retain their available shot/damage/movement review, while new
view-to-player trajectories require a fresh parse. No database migration or
parser dependency update is needed. API must be updated before the home worker.
CLI mechanics exports include personal episode evidence after anonymization;
new sample geometry contains no additional player IDs.

## Availability audit

All four local demos supply eye-offset and view-angle fields. Inferno captures
339 positive enemy damage contexts with 3,390 paired samples; current Dust2
captures 497 with 15,904 paired samples. Ancient/Anubis have no usable firearm
damage and do not receive a guessed damage target. Tests cover geometry,
unknown/zero separation, exact-frame pairing, ambiguity, legacy capture,
personal filtering, privacy, grouping and the worker/API payload bound.
