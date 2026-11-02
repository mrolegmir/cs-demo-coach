# Aim direction and movement review

Available with v1.1. Mechanics capture 4; no migration or parser update.

Open a match as a signed-in participant. Shooting and Movement now show neutral
**Observations available** when recorded rifle sequences exist. This describes
available evidence, not a skill grade. Coaching priorities remain unchanged.

## Shooting

Expand a sequence by round, side, time and weapon. When a uniquely linked
positive enemy damage event identifies one recipient for the first damaging
shot, the card names that player, the shot ordinal and event-time distance in
game units. Ambiguous first-hit events do not select a target. No recorded
damage is not a miss.

Fresh capture adds half a second of samples before damage. The table pairs
speed with geometric view-to-eye angle for the same frame. A measured zero
angle is separate from missing coordinates. Samples stop before the damage
tick; there is no guessed instantaneous or subtick observation.

Read the interpretation help: the later damaged player may not have been the
intended or visible target. Eye-point geometry is not a head hitbox or an aim
error score. Recoil, smoke and walls are not reconstructed; no accuracy, reaction
time, spray-control or crosshair-placement diagnosis is claimed.

## Movement

Expand a sequence to inspect speed at each shot and airborne state. The first
shot's preparation contains the preceding half-second where available. Per-shot
rows also show elapsed time, measurement interval and linked health damage.
The table scrolls horizontally; speed and airborne state appear first.

Speed is a frame average. Missing is unknown, not standing still. No recorded
damage does not establish a miss or a movement mistake. The review does not
infer key presses, exact stop time or whether a counter-strafe was correct.

## Saved reports and previews

Older captures retain the information they already contain. Versions 1/2 have
no preparation history, version 3 can show speed preparation, and version 4 adds
view/eye contact samples. A new coaching request cannot recover absent fields;
the new trajectory needs a fresh parse. Demos remain deleted after processing.

Personal review follows the viewer rather than the selected coaching party.
Existing selected-party diagnostics remain available separately. New CLI
mechanics exports contain the same episode evidence, anonymized as before.

```powershell
$env:CSDC_AIM_PREVIEW_DIR = Join-Path (Get-Location) 'demos/out/aim-preview'
go test ./internal/web -run TestAimMovementRealDemoPreview -count=1
go run ./cmd/csdc mechanics testdata/matches/mm_2026_dust2.json.gz
```

The local preview has RU/EN current and legacy pages. Browser checks cover both
categories at 1440/390 px. Four anonymized fixtures are refreshed; detector
goldens remain unchanged. Build, vet, lint, full race/database tests and a real
worker detection payload regression validate the release. See [ADR 29](decisions/0029-aim-and-movement-contact-review.md).
