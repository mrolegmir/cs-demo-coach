# 17. Roles: what each player does for the team

**Status:** accepted (v0.8)

## Context

SCOPE's team principle mentions the role split. Mistakes alone do not show
it: a player who dies first a lot may be a bad entry or the party's only
entry. The coach needed to know who takes the first fights, who throws
the flashes that work, and who plays the AWP, to say "Carl, flash for
Anna" instead of "trade more".

## Decision

- **Count roles from the match, next to findings.** Per player and match:
  rounds played, first duels taken and won, kills, AWP kills, flashes, and
  flashes that blinded an enemy for at least a second (the flash
  detector's threshold). They go into the report and are filled in for
  older reports from stored matches, like trade pairs (ADR 16).
- **Flashes that blind, not flash assists.** The first version counted
  kills on blinded enemies; real matches gave 0 to 4 per match for all ten
  players, too few to say anything. Flashes that blind an enemy are common
  enough to compare and still measure useful utility.
- **Labels only when clear.** Across a player's matches, against the
  players in the same lobbies: entry at 1.5 times the lobby's share of
  first fights, support at 1.5 times its blinding flashes (at least 10),
  both over at least 30 rounds (and five first duels for an entry); AWPer
  at 40% of kills with the AWP (at least 10). A single match is shorter
  than 30 rounds, so labels, and the role advice that needs them, come
  from history: the progress page and the weekly brief. Coaching for one
  match gets the numbers without labels. Otherwise the page says "no clear role yet" and shows the
  numbers.
- **The coach uses roles for team advice.** Coach input carries party
  members' roles. The rule-based coach asks the party's best flasher, if
  they blind enemies at least as often as the lobby, to flash for an entry
  whose dry first deaths stand out, citing them, and then leaves out the
  generic first-deaths item; the
  LLM is told to do the same and to name role overlaps or gaps.

## Consequences

- On the owner's 13 local matches: first fights in 17% of rounds against a
  20% lobby, won 64% of them, and 0.7 blinding flashes per 10 rounds
  against 1.5. No label yet, which is the honest answer.
- Team advice stays capped at two items: pairs first, then roles, then
  shared mistakes.
