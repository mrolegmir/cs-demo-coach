# 25. Retain a short movement history before firing

**Status:** accepted for v1.1.

## Decision

Mechanics capture version 3 retains up to 500 ms of frame samples before each
firearm shot, plus a shot-time sample. Each sample records its tick, airborne
state and optional horizontal displacement speed with the measured interval.
The parser keeps a rolling half-second buffer per living human participant;
shots receive independent copies. Buffers reset at round start and on death.
Duplicate frame ticks do not replace the previous distinct position.

Existing speed rules remain: non-positive intervals, gaps above 100 ms and
speeds above 400 units/s are unavailable. Missing speed stays distinct from
measured zero. Sample timestamps may be irregular; neither interpolation nor
subtick velocity is inferred. The interval ending at the first retained sample
can start before the half-second window.

Rifle sequences expose the history belonging to their first shot. The match
page adds a collapsed RU/EN movement section with round/time/weapon/place and
sample times relative to that shot. It uses the existing selected-party filter
and match access checks, and does not require administrator diagnostics mode.
The CLI mechanics export includes the same first-shot history.

Capture versions 0–2 retain their existing behavior and explicitly lack movement
history. Version 3 retains version 2's conservative shot/damage attribution.
No database migration or parser dependency update is needed. Previously parsed
matches require a fresh parse; coaching cannot recover the history.

## Limits and next step

These are firing sequences, not verified duels. No target, visibility, smoke,
cover, stop threshold, key presses or counter-strafe technique is inferred.
No mechanical findings, priorities, practice prescriptions or LLM claims are
enabled. Aim preparation still needs contact and visibility evidence, followed
by episode review and calibration on separate matches.

Retaining history at every shot costs more parsed JSON storage. In the four
anonymized fixtures gzip sizes grow from roughly 106–150 KiB to 217–577 KiB.
Full-match frame streams are not retained and demo deletion is unchanged.
