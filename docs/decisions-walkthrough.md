# Decisions and objective review

Implemented for v1.1. Objective capture version 1; no migration.

Open a match while signed in as one of its participants and expand **Decisions**
(**Решения**). The category stays neutral: it contains replay review material,
not a score of your decision-making. It is personal and ignores the selected
coaching party.

## Lost rounds after a numerical advantage

Expand a round to see the time and recorded player counts, such as 5v3, while
you were still alive. View the living players on both teams, then subsequent
deaths with time, killer and the victim's game-supplied area. Review what
happened next, including spacing, angle clearing and the objective.

One greatest observed difference in player counts is selected per lost round;
ties keep the first snapshot. Side switches are respected. Unknown/duplicate
players, same-tick multi-kills, missing victims and snapshots after your death
do not produce a candidate. At least one opponent must remain alive. Missing
or unlisted situations are not an endorsement of all your decisions.

This part works from saved death snapshots. No new coaching request or parse
is required, and saved narrative is not rewritten.

## Bomb and post-plant timeline

With a fresh capture, expand a round to see plant and defuse actions, deaths
after the plant, and the round outcome. A complete plant snapshot shows T/CT
alive counts and whether you were in that snapshot. Defuse starts show the
recorded kit state; unavailable kit information is separate from "No".

Times are relative to freeze end. The +time column is elapsed time after
planting; it is not a countdown. A/B come only from recorded event sites.
Unknown actors and incomplete snapshots stay unavailable. Duplicate plants do
not supply a guessed reference time. A death at plant time prevents definite
plant counts. Events sharing a tick do not establish an exact order.

Older matches explain why a new parse is required for this timeline. No bomb
timer, retake deadline or intent behind an interrupted defuse is inferred.
Both won and lost rounds are included.

## Validation and local demo

Four real, anonymized parser fixtures were refreshed:

| Fixture | Bomb events | Completed plants with full snapshots |
|---|---:|---:|
| Ancient | 35 | 10 |
| Anubis | 21 | 8 |
| Inferno | 33 | 12 |
| Current Dust2 | 29 | 10 |

Existing detector goldens stay unchanged. Model and HTTP tests cover personal
selection, side switches, neutral status, missing/duplicate data, kit absence,
anonymization and preserved nickname rendering. Build, vet, lint and race tests
run with local PostgreSQL. RU/EN desktop/mobile pages are reviewed in a browser.

Generate local pages from the committed anonymized Dust2 fixture:

```powershell
$env:CSDC_DECISIONS_PREVIEW_DIR = Join-Path (Get-Location) 'demos/out/decisions-preview'
go test ./internal/web -run TestDecisionsRealDemoPreview -count=1
```

The preview includes RU/EN current and legacy-capture pages. Expand Decisions.
See [ADR 27](decisions/0027-decision-review-and-bomb-context.md) for selection rules.
