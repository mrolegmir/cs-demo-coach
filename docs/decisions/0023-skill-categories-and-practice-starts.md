# 23. Match skill categories and explicit practice completion

**Status:** accepted.

## Decision

The signed-in participant sees six categories: utility, teamplay, positioning,
economy, shooting and movement. Existing deterministic checks populate the
first four. Shooting and movement explicitly remain unassessed. A viewer not
present in the demo receives no substituted participant's assessment.

Amber means a recorded pattern worth reviewing, green means zero occurrences
in the checked situations, and gray means no eligible situations or no
implemented assessment. These are not skill grades. No uncalibrated red
severity threshold, numeric confidence, HE damage or mechanics diagnosis is
introduced. Category counts count checks with findings, not independent errors.

Each check shows its own numerator and denominator, per-side occurrence
counts, round/time/place evidence, a bounded explanation, and library drills.
The personal action list preserves the existing coach's focus order and cap;
raw counts with different denominators do not create a new priority ranking.
Previously saved narrative remains in a collapsed report. It is not rewritten
or silently presented as newly validated prose.

Practice starts are persisted separately in `practice_sessions` (migration 11).
The unique owner/drill key makes repeated starts idempotent. Starts appear on
match pages and the owner's practice log, but are absent from completed
practice and before/after coaching calculations, including in older workers.
An explicit finish records actual minutes and atomically moves the owned
session to completed practice. A repeated or foreign finish creates no entry.
All actions use the existing authenticated, same-origin POST protection.

Scoreboard groups follow starting teams and sort by kills within each team,
with the viewer's team first. Round history marks observed side changes rather
than assuming MR12, so historical MR15 demos remain supported.

## Limits

Spacing still ignores walls; no communication or premade inference is added.
Grenade retention and short blinds are observations, not proof of a bad play.
No match reparsing, LLM request or worker replacement is needed for categories.
There is no automatic practice completion or background timer.
