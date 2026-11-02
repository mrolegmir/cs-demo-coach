# 2. Detectors and how they are measured

**Status:** accepted (v0.1), recalibrated in v0.2

## Context

SCOPE.md asks for the detectors that matter most to casual parties. They have
to be deterministic, reproducible and easy to explain.

## Decision

Six detectors, nine kinds of mistakes:

| Kind | Chances (the denominator) | Default thresholds |
|---|---|---|
| `isolated_death`: untraded, no teammate within 700u | deaths with a teammate alive | trade window 5 s |
| `missed_trade`: nearest teammate within 700u did not trade | times you were that teammate | trade window 5 s |
| `flash_no_enemy` | flashes thrown | enemy blind < 1 s |
| `team_flash`: teammate or self blinded | flashes thrown | ≥ 1.5 s |
| `unused_utility`: died holding grenades (decoys excluded) | deaths | — |
| `no_utility`: full buy, lived 30+ s, threw nothing | such rounds (rounds shorter than 30 s are skipped) | full buy ≥ $3700 |
| `buy_desync`: full buy while the team saved, or no rifle buy (< $2500) despite the money while the team full-bought | non-pistol rounds | full ≥ $3700, eco < $1500, majority of 3 |
| `missed_drop`: you kept $3000+ after buying while a teammate who could not afford one stayed without a rifle buy | full-buy rounds you kept $3000+ | same |
| `opening_death`: first death without a teammate flash on an enemy | rounds | flash within 2 s |

Every detector also reports **chances**, so each mistake becomes a rate. The
coach compares that rate with the **lobby rate** (all ten players of the same
match) rather than with a fixed benchmark.

## Why these

- They are about decisions (spacing, utility, economy), not aim. Casual
  players can fix them by agreeing on a few simple rules.
- Four of them concern the team (trades, buys, drops, flash support), which
  follows the "party as a unit" principle.

## Calibration history

- **v0.1, 3 public 2023 demos.** A single "untraded death" detector flagged
  about 90% of deaths; only 12–13% of kills in these matches get traded. It was
  split into isolated deaths and missed trades.
- **v0.2, 9 real 2026 matchmaking demos from the target users.**
  - The old buy-desync detector (save vs. full buy) fired on about 1% of
    rounds: matchmaking teams almost always buy in sync. The common economy
    mistakes turned out to be different. In 54 rounds a teammate was short
    while someone else had money for a drop. In 35 rounds a player
    under-bought with full-buy money. The detector now covers both, and
    `missed_drop` is new.
  - Some players almost never throw utility, so `unused_utility` (died holding
    grenades) misses them. In the lobby, a player who full-bought and lived
    30+ s threw nothing in 23% of such rounds; for the target party it was
    37–42%. `no_utility` captures this.
  - A current-format matchmaking match was added to the anonymized fixtures.

Drops cannot be seen directly: a teammate with full gear might have been
dropped a weapon or might have kept one from the last round. So
`missed_drop` counts every round where you had drop money as a chance, and
flags those where a teammate stayed short anyway. Everyone who could have
dropped shares the finding.

## Known limits

- Distances are straight lines and ignore walls. 700u means "close", not
  "had line of sight".
- Equipment is snapshotted when the buy window closes, so a weapon picked up
  from the floor counts as bought.
