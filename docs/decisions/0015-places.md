# 15. Where on the map: the game's place names, counted per mistake

**Status:** accepted (v0.6)

## Context

The library drills (ADR 14) are the same on every map. "Learn two
pop-flashes for your site" leaves the player to work out which site and
which flash, and players learn lineups map by map, from community utility
guides drawn on the real map in practice mode. The coach knew round and
time for every finding but not where it happened, so it could not say
"most of your dry first deaths are at Banana; learn the Banana flash".

## Decision

- **Use the game's own place names.** CS2 keeps a place name for every
  player (`m_szLastPlaceName`: "BombsiteB", "LongA", "Banana"), and
  demoinfocs exposes it. The parser stores it for every player alive at a
  kill and, for every flash, where the thrower stood when throwing it (by
  the time it goes off they may have moved or died). On the four test demos every
  death has one. No map data of our own, no polygons to maintain, and the
  names are the ones players see in game.
- **Findings carry the place** where the moment happened: the victim's for
  deaths, the would-be trader's for missed trades, the thrower's for
  flashes. Findings about a whole round (buys, no utility thrown) have none.
- **Counted per mistake, within a map.** Names repeat across maps ("B
  site" is on every map), so history sums a player's places per kind and
  map; briefs name the map with the place ("B site, Mirage"). Coach input
  carries the top three per issue, how many findings have a place at all,
  and the place of every example. Names are turned into words once (`match.PlaceName`:
  "BombsiteB" → "B site", "TopofMid" → "Top of Mid").
- **The drill names the spot.** The LLM is told to name the place where
  most findings happened in its `drill` text, and for a lineup drill the
  lineup for that spot. The rule-based coach adds "Start at Long A: 3 of
  your 5 were there" when one place has at least two findings and a third
  of those with a place (older matches have none). Drill cards and the
  progress page list places with two findings or more.
- **Lineup drills point to utility guides.** The four lineup drills now
  start with subscribing to a utility guide for the map in the Workshop and
  turning it on in practice mode, instead of "a video helps".

## Consequences

- On the 2026 Dust2 match with ChatGPT, the coach wrote "learn the two
  pop-flash lineups for your Outside Long approach" and "start with an
  Outside Long lineup, where 3 of your 5 findings happened".
- Matches analyzed before v0.6 have no places: demos are deleted after
  parsing, so the progress page's places fill in as new matches arrive.
- A place name is a coarse area ("A site" is big). Good enough to say where
  to start; not a lineup position.
