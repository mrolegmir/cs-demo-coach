# 16. Trade pairs: the party as a unit

**Status:** accepted (v0.7)

## Context

SCOPE's third principle is that the party is analyzed as a unit, not only
as individuals. Until v0.7 the team section only appeared when several
players shared the same mistake. But trading, the core team skill in CS,
depends on the pair: "you are traded 20% of the time" hides that one
teammate trades you every time and another never does. The trade detector
already found the nearest teammate for every death; it just did not keep
who it was when the trade happened.

## Decision

- **Count pairs in the trade detector.** For every death with a teammate
  within trading distance, the pair (victim, nearest teammate) gets a
  chance, and a trade if the killer died within the window. Pairs go into
  the report next to findings and stats. A test checks they add up to the
  trade stats exactly: every chance belongs to one pair, every untraded one
  is a missed trade.
- **Backfill from stored matches.** The parsed match is kept in the
  database, so the api recomputes pairs for older reports once at start
  (reports without a `pairs` field), instead of needing the demos again.
  Saving a detection fills them in too, so a worker from before v0.7 still
  produces reports with pairs.
- **Two directions per teammate.** The progress page lists teammates with
  at least four deaths near each other: how often the killer is traded when
  the player dies with that teammate nearest, and the other way round. A
  trade counts whoever makes it, so the numbers describe the pair's
  spacing as much as the nearer player's swing. Teammates the viewer may
  not open (ADR 7) are left out, so the page does not reveal who else the
  player plays with.
- **The coach sees party pairs.** Coach input carries pairs within the
  party, summed over the period's matches for briefs, together with the
  partner's missed trades of that player as evidence. The LLM is told to turn a pair
  that trades badly into team advice. The rule-based coach does the same
  for the worst pair with at least four chances and at most a 25% trade
  rate, citing the partner's missed trades of that player. Team advice
  still needs evidence, so a pair without a missed trade to cite gives
  none. The rule-based coach keeps team advice to two items and drops the
  generic missed-trade item when the pair item covers it.

## Consequences

- On the owner's 13 local matches, the backfill filled every report, and
  the page showed the gap a per-player rate hides: traded 3 of 15 times
  near one teammate, while trading that teammate 7 of 21 times.
- One match rarely has four deaths near the same teammate, so pair advice
  mostly comes from weekly briefs, which is where team advice belongs.
- "Nearest teammate" is a straight-line distance, as for missed trades: it
  does not know about walls.
