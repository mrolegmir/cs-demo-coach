# Movement before firing

Implemented for v1.1. Capture version 3; no database migration.

1. Parse a fresh demo and open its match page. Expand **Movement before firing**
   (Russian: **Движение перед стрельбой**). Administrator access is not required.
2. Expand a rifle sequence using its player, round/time and weapon/place.
   The table shows up to 500 ms of movement before the first shot, followed by
   the shot-time sample at 0 ms. Speed is in game units per second; the interval
   column explains how much time each average covers. Airborne state is separate.
3. Compare measured zero with **not recorded**. A missing sample must not be read
   as standing still. A long gap is not interpolated.
4. Open a saved version 0–2 match. It explains that a new parse is required.
   Requesting coaching again does not add the missing history.
5. Select a party: movement sequences follow the existing party filter.

The section supports reviewing movement around firing. It does not diagnose
aim, establish a duel or measure exact stop timing/counter-strafe inputs.
Coaching priorities and the unassessed shooting/movement categories remain as
before. Visibility and contact reconstruction are the next research step.

## Real-demo capture audit

All fixtures are anonymized; no original names or Steam IDs are published.

| Fixture | Firearm shots | Shots with pre-shot history | Retained samples | Samples with speed |
|---|---:|---:|---:|---:|
| Ancient | 2758 | 2758 | 30327 | 30325 |
| Anubis | 1986 | 1986 | 21846 | 21846 |
| Inferno | 2124 | 2123 | 23344 | 23341 |
| Current-format Dust2 | 2443 | 2443 | 80619 | 80619 |

History means at least one sample before the shot, in addition to its terminal
sample. Capture availability does not validate a movement diagnosis.

Reproduce the anonymized export:

```powershell
go run ./cmd/csdc mechanics testdata/matches/mm_2026_dust2.json.gz
```

See [ADR 25](decisions/0025-pre-shot-movement-history.md) for retention semantics.

## Validation and local demo

- Build, vet and golangci-lint.
- Full race suite; database integration also runs against a temporary local
  PostgreSQL cluster for store, worker, web and Telegram packages.
- All four real-demo parser fixtures regenerated and checked for monotonically
  ordered history, bounded windows and valid speed intervals. Existing detector
  golden reports are unchanged.
- Buffer bounds, duplicate ticks, gaps, teleports, independent history copies,
  first-shot selection, legacy versions and damage compatibility are tested.
- RU/EN HTTP rendering verifies measured zero, missing data and party filtering.
- Browser checks at 1440 and 390 px: nested sections open, tables remain readable,
  no page overflow. Anonymized HTML, screenshots and the Dust2 mechanics export
  are saved locally under `demos/out/movement-preview/` (gitignored).
