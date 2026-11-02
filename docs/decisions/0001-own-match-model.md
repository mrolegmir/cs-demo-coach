# 1. Detectors work on our own match model, not on the parser

**Status:** accepted (v0.1)

## Context

`markus-wa/demoinfocs-golang` emits a stream of events with live pointers into
parser state (`*common.Player`, inventories, positions). That data is only valid
while the event fires, it changes between CS2 updates, and testing against it
takes a 50–300 MB demo.

## Decision

`internal/demo` is the only package that imports the parser. It turns a demo
into `match.Match`: plain data with rounds, kills (with a snapshot of who was
alive and where), flashes and who they blinded, grenades, and each player's buy.
Detectors (`internal/detect`) only see `match.Match`.

## Consequences

- Detector tests build matches by hand (a few lines per case) and run in
  milliseconds.
- Parsed matches are committed as small anonymized gzip fixtures
  (`testdata/matches`, about 30 KB each), so CI runs golden tests without demos.
  `go test ./internal/demo -update` rebuilds them when the demos are present
  locally.
- The model only holds what the detectors need. A new detector may need a new
  field and a fixture refresh, which shows up as a reviewable golden diff.
- Parser quirks are handled in one place. 2023 CS2 demos do not emit
  `RoundFreezetimeEnd`, so the parser also listens to the game-rules flag.
  `IsReloading` is never set in these demos, so a "died while reloading"
  detector was dropped.
