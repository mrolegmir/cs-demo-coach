# Flash follow-up review

Implemented for v1.1. Available from existing parsed matches.

Sign in as a match participant, expand **Teamplay** (**Командная игра**), then
**Flash follow-up** (**Игра после флешек**).

The summary separates your recorded flash bursts, teammate kills during your
flash, your kills during a teammate's flash and kills with multiple possible
sources. Individual totals use only one eligible allied source with known
teams; they are not official flash-assist counts or a skill grade.

## Inspect the evidence

- **Your flash bursts** shows round/time and the throw area. Expand a flash for
  recorded recipients, team relations and blind durations. Enemy targets are
  distinct players with positive recorded duration for that flash; team and
  self-blinds remain visible. Unclassified records are counted separately.
- **Kills during recorded blindness** shows the killer and victim, source
  throwers with burst times, and whether the association was unique, ambiguous
  or ineligible. Only unique eligible matches have a displayed elapsed interval.

Both partial and longer blinds can enter review. There is no new duration or
effectiveness threshold. A kill precisely at the recorded blind interval's end
is excluded; an action in the burst's tick receives no individual attribution.
Enemy and unknown-source flashes can block an otherwise unique match. Repeated
records for the victim within one flash do not create extra sources.

No kill does not make a flash useless, and a temporal overlap does not establish
that the flash caused a kill. No visibility or communication is reconstructed.
Own-flash/own-kill situations remain in evidence but do not count as teamwork.
Unknown tick timing leaves matching totals unavailable, not measured zero.

The personal report ignores the selected coaching party and respects round-time
sides. Existing check statuses, recommended drills, coach priorities and saved
narrative remain unchanged. This feature alone needs only an API release; it
does not require reparsing, migrations or changes to the home worker.

## Real-demo audit and validation

| Anonymized fixture | Recorded flash bursts | Uniquely matched teammate kills |
|---|---:|---:|
| Ancient | 67 | 1 |
| Anubis | 53 | 0 |
| Inferno | 49 | 0 |
| Current Dust2 | 48 | 4 |

These whole-match counts check availability and matching consistency, not
rank expectations or player performance. Tests verify incoming/outgoing count
agreement, round locality, side switches, short blinds, interval boundaries,
same-tick events, enemy/unknown sources, duplicate data, missing timing/sides,
posthumous throwers, anonymization and both personal perspectives.

RU/EN HTTP rendering verifies literal escaped player names, personal selection
and neutral priorities. Browser checks cover summaries, flash details and kill
cards at 1440 and 390 px. Build, vet, lint and race tests run with local PostgreSQL;
existing detector goldens remain unchanged.

Generate the local preview from the committed anonymized Dust2 fixture:

```powershell
$env:CSDC_FLASH_PREVIEW_DIR = Join-Path (Get-Location) 'demos/out/flash-preview'
go test ./internal/web -run TestFlashSupportRealDemoPreview -count=1
```

Expand Teamplay, then Flash follow-up. See [ADR 28](decisions/0028-flash-follow-up-review.md).
