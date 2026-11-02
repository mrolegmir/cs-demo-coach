# Shooting observations pilot

The owner approved rolling out this pilot together with interface localization.
The API and worker use the same release; old analyses need a new parse for
shooting telemetry. No mechanical diagnosis is enabled.
An anonymized local example is saved at `demos/out/mechanics-preview/match.html`.

1. Parse a fresh demo, open its match page and expand **Analysis coverage and
   shooting observations**. Select a party to restrict the displayed players.
2. Compare rifle shot counts, available recent-speed samples and sequence sizes.
   The explanatory text states the grouping and measurement limits. A larger
   number of long groups is not a verdict about spray control.
3. Open an older saved match: it says a new parse is required for shot capture.
   Re-coaching alone does not create missing data.
4. Existing focus areas still come only from validated detectors. The empty-focus
   message is limited to the patterns checked, rather than endorsing all skills.

See [the skill catalog](skills-catalog.md) for coverage and development criteria,
and [ADR 19](decisions/0019-mechanics-observations.md) for measurement semantics.

## Real-demo capture check

Four local demos, including the current-format matchmaking fixture, were parsed
and anonymized. Only aggregate counts are shown here.

| Fixture | Firearm shots | Recent speed samples | Samples above 80 units/s | Rifle sequences |
|---|---:|---:|---:|---:|
| Ancient | 2758 | 2758 | 148 | 345 |
| Anubis | 1986 | 1986 | 152 | 264 |
| Inferno | 2124 | 2123 | 63 | 305 |
| Current-format Dust2 | 2443 | 2443 | 74 | 357 |

These checks establish capture availability, not the accuracy of a mechanical
diagnosis. Near-complete speed availability does not establish subtick precision.
The committed fixtures retain anonymized shot evidence; no real nicknames or
Steam IDs are introduced.

## Verification

- `go build ./...`, `go vet ./...`, `golangci-lint run` (zero issues).
- `go test -race ./...` without database integration.
- `go test -race -p 1 ./...` with local test PostgreSQL. Packages run
  sequentially because the existing temporary-database helper produced a name
  collision during the parallel integration run.
- Four refreshed anonymized parser fixtures; existing detector golden reports
  remain unchanged.
- HTTP rendering checks for both capture versions, selected-party filtering,
  missing-speed handling, sequence boundaries and shot anonymization.

The saved HTML example was rendered through the application handler. Browser
visual inspection and validation of aim diagnoses were not performed.

## Damage evidence extension (capture version 2)

The local HTML example now uses anonymized current-format Dust2 data. Expand
**Review shooting sequences** to inspect every rifle sequence: round/time,
weapon/place, shots, speed-sample coverage, airborne shots, shots with uniquely
linked enemy damage, health damage and first-damage shot ordinal. The table is
filtered to the selected party. Outcomes are unavailable for versions 0/1 and
for demos without usable firearm damage, rather than presented as misses.

Reproduce the exploration exports:

```powershell
go run ./cmd/csdc mechanics testdata/matches/mm_2026_dust2.json.gz
go run ./cmd/csdc mechanics testdata/matches/arsham_infernocs2_1stSept23.json.gz
```

Local anonymized artifacts: `demos/out/mechanics-preview/dust2-evidence.json`
and `inferno-evidence.json`. These include damage targets, distances, unmatched
events and descriptive buckets; no nicknames or original Steam IDs are exported.

| Fixture | Firearm damage events | Positive enemy events | Uniquely linked enemy events |
|---|---:|---:|---:|
| Ancient | 0 | 0 | 0 |
| Anubis | 0 | 0 | 0 |
| Inferno | 343 | 339 | 336 |
| Current-format Dust2 | 503 | 497 | 495 |

Zero events in the first two fixtures means unavailable outcome evidence.
Counts exclude health damage overkill; friendly and zero-health-damage events
are retained in audit evidence but excluded from positive enemy counts.

This is an initial data audit, not a completed calibration. Dust2 has only one
grounded AK-47 sequence beginning above the descriptive speed cutoff. Sequence
length buckets are not matched on range/visibility/intent. No mechanical
diagnosis passed promotion into coaching; see [ADR 20](decisions/0020-shot-damage-evidence.md).
