# HE and fire damage review

Implemented for v1.1. Utility capture version 1, no migration.

1. Parse a fresh demo, sign in as a participant and open the match's **Utility**
   category (**Гранаты** in Russian).
2. Expand **HE and fire damage** (**Урон от HE и огня**). Compare recorded throw
   counts and health damage to enemies, teammates and yourself for HE and Fire.
   Molotov and incendiary damage share the Fire family.
3. Expand damage events to see round/time, your side at the event, the damaged
   player, their relation to you, damage and the damaged player's area. Fire can
   create many events from a single grenade. Place names are not throw positions.
4. Check unknown-amount and unknown-team counters. Unknown amounts do not become
   zero or get replaced by overkill damage. Unknown-team events do not enter
   enemy/teammate totals. These two exclusion counters can overlap.
5. Older analyses explain that a fresh parse is needed. If a freshly parsed demo
   contains no usable utility damage events, damage totals show dashes and an
   availability notice; throw counts still appear.
6. Changing the coached party does not change your personal utility results.
   A viewer absent from the demo does not receive another player's category.

This review does not score grenade effectiveness. Damage is not linked to
individual throws, armor damage is excluded, and fire may deny space without
damaging anyone. No new mistake detector, severity grade or coach priority is
introduced. Full smoke/area-denial analysis remains future work.

## Anonymized real-demo audit

| Fixture | HE damage events | Fire damage events | Enemy HE HP | Enemy fire HP | Team HP | Self HP |
|---|---:|---:|---:|---:|---:|---:|
| Inferno | 48 | 60 | 490 | 225 | 43 | 37 |
| Current Dust2 | 38 | 63 | 647 | 231 | 22 | 0 |

All 209 events have non-overkill health amounts. Ancient and Anubis fixtures
produce no utility damage events; their totals are unavailable. Counts are
whole-match aggregates for capture review, not player benchmarks.
Both supported fixtures retain 13 events after the thrower's death.

## Validation

- Four real-demo parser fixtures refreshed; existing detector goldens unchanged.
- Tests cover overkill exclusion, unknown versus measured zero, event-time sides,
  separate enemy/team/self totals, fire-family grouping, anonymization, personal
  filtering, non-participants and legacy/unsupported capture.
- RU/EN rendering and browser review at desktop/mobile widths.
- Build, vet, lint and full race tests with local PostgreSQL.

Create an anonymized local page using the real Dust2 fixture:

```powershell
$env:CSDC_UTILITY_PREVIEW_DIR = Join-Path (Get-Location) 'demos/out/utility-preview'
go test ./internal/web -run TestUtilityRealDemoPreview -count=1
```

The directory is gitignored. Expand the Utility category and its damage review.
See [ADR 26](decisions/0026-grenade-damage-observations.md) for measurement limits.
