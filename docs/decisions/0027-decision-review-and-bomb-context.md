# 27. Review decision context without assigning a decision-quality score

**Status:** accepted for v1.1.

## Decision

A seventh personal match category, Decisions, presents two kinds of review
material. Its neutral status describes available situations, not mistakes,
severity or a tactical grade. Existing checks, priorities, drills, weekly
history and LLM input remain unchanged.

### Lost numerical advantages

Existing death snapshots support review without reparsing. Select a lost round
where the viewer was still alive after a recorded kill and their side had more
living players than the other side. Use the player's side in that round, not
their starting side. Pick the greatest observed difference in player counts;
keep the first recorded snapshot on a tie. Show one situation per round, its
alive teams, and subsequent deaths with time, killer and victim area.

Reject unknown winners/sides, missing or duplicate players, missing victims,
bot/unknown IDs, kills outside the live round, same-tick multiple kills and
snapshots after the viewer's death. Require at least one living opponent: a
1v0 lost to the bomb is not treated as a fighting advantage. These rules select
interpretable evidence, not a calibrated mistake threshold or win probability.
Follow-up death ordering within the same tick remains unknown.

### Bomb context

Independent ObjectiveVersion 1 captures plant starts/interruptions, completed
plants, defuse starts/interruptions/completions and explosions. Records carry
tick, optional human actor, allowlisted A/B site and explicit kit state on
defuse start. A plant also captures a sorted alive-player snapshot. Bots/unknown
sides make its completeness flag false. Missing actors remain unknown;
explosion has no attributed actor.

The timeline joins bomb actions with deaths at/after a uniquely recorded plant,
up to round end. Relative time means elapsed time since plant, not remaining
bomb time. Duplicate plants disable the reference and post-plant death selection.
Incomplete/invalid snapshots, or a death at the plant tick, suppress definite
alive counts. Zero tick rate suppresses times. Known kit absence stays distinct
from unavailable kit information.

No default C4 timer, travel/retake estimate, defuse feasibility, intent behind
an interrupted attempt, or ordering within a tick is guessed. Both positive and
negative round results are shown; absence of an event does not prove that the
action never occurred. Player communication and knowledge are not reconstructed.

## Compatibility and verification

Old parsed matches keep lost-advantage review but need a fresh parse for the
bomb timeline. New IDs and snapshots are anonymized. No database migration or
parser dependency change is needed. Deployment later needs API and home worker.

All four anonymized fixtures capture bomb actions and complete plant snapshots:
Ancient 35/10, Anubis 21/8, Inferno 33/12 and current Dust2 29/10 (events/plants).
Tests cover selection boundaries, side changes, ambiguity, missing data,
non-participants, identity retention, kit state and post-plant death merging.
