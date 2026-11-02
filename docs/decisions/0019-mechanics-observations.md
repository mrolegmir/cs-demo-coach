# 19. Separate mechanical observations from coaching findings

**Status:** accepted for local pilot; not deployed

## Context

The original detectors cover selected utility, economy and teamplay patterns.
The coach cannot assess skills absent from its input, and missing aim feedback
must not imply good aim. Expanding a fixed list of diagnoses alone does not solve
the lack of underlying observations.

## Decision

Extend the parser-independent match model with versioned firearm shots. Capture
event-time state as plain values, including view angles and position. Version 1
estimates recent horizontal speed from the preceding FrameDone position and the
in-game tick difference. The pinned parser has no Player.Velocity method. Zero or
negative tick differences, gaps above 100 ms and estimated speeds above 400 units/s
produce unknown speed. The estimate is not instantaneous or subtick velocity.

Derive rifle shooting sequences within each round and player. Same-weapon shots
with gaps at most 250 ms form a sequence. Group sizes 1, 2–5, 6+ and speeds above
80 units/s are descriptive bins, not validated mistake thresholds. The supported
rifles are AK-47, M4A4/M4A1, FAMAS, Galil AR, AUG and SG 553.

Show observations and coverage on the match page, filtered by the selected party.
Do not add these observations to detector findings, LLM input, priorities or issue
history. Existing reports remain readable and say shot capture is unavailable.
The data fits existing match JSON storage; no database migration is needed.

Maintain a skill catalog and explicit promotion criteria. Exploratory pattern
discovery is future work. New observations do not prove that a shot was directed
at a visible target, missed, or lost because of movement/recoil.

## Consequences

- Deterministic, anonymizable data can support multiple later analyses.
- New analyses use more JSON storage; full frame histories are not retained.
- Old demos cannot be reanalyzed after deletion unless uploaded again.
- No automatic aim grade or counter-strafe prescription is justified yet.
- Golden detector reports must stay unchanged when refreshing match fixtures.
