# 7. Issue history: habits, not bad games

**Status:** accepted (v0.2)

## Context

One match is noisy. SCOPE asks the coach to recognize improvement, demote
one-off bad games and re-prioritize over time. That needs a per-player view
across matches with a small, explainable set of statuses.

## Decision

`internal/history` is pure logic, fed with the detector stats already stored
per match (`matches.report`). For each kind of mistake it sums the player's
counts and the lobby's (everyone else in the same matches) and assigns one
status:

| Status | Rule (stand out = count ≥ 3 and binomial z ≥ 1 vs. the lobby, as in the coach) |
|---|---|
| not enough data | fewer than 10 chances in total |
| one-off (bad games) | does not stand out overall, or stands out only because of a single match |
| fine | never stood out |
| active (habit) | stands out overall and in the last 3 matches, or there is no earlier history yet |
| improving | stands out overall, not in the last 3 matches, but still above the lobby there |
| resolved | stood out before the last 3 matches and is at or below the lobby in them |

"Recent" is judged only when there are more than 3 matches and enough
chances in them; otherwise the overall verdict stands.

## Consequences

- The progress page (`/players/{steamid}`) shows each mistake with its
  status, overall and recent rates against the lobby, and a per-match strip
  linking to the evidence. Players can see each other's pages only if they
  played together.
- Checked against the owner's 9 real matches, the rules produced:
  - one habit (no utility thrown, 38% vs. a 23% lobby rate, 41% recently);
  - one resolved issue (dying with utility, 60% overall but 44% recently);
  - isolated deaths as bad games, not a habit.
- Matches are ordered by upload time, which is when the demo was played
  closely enough for weekly use. A bulk upload of old demos would need the
  real match date, which CS2 demos do not carry in the header.
