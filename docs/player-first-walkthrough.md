# Player-first pages

Player-facing match pages introduced on `feat/player-first-match-pages`.

## Demo

Anonymized, handler-rendered examples are in the gitignored directory
`demos/out/player-first-preview/`: `index.html` and `match.html`.
Regenerate them with `CSDC_PREVIEW_DIR` set to that absolute directory and
`go test ./internal/web -run TestPlayerFirst`.

1. Home: recent matches come first, with a score and an accessible result
   symbol; filenames and database IDs are absent from the visible table.
2. Weekly brief: Friday generation is automatic. The collapsed early-request
   action explains that it uses the manual coaching allowance.
3. Telegram and manual demo upload are collapsed below the weekly section.
4. Open a match as a participant: see the score, rounds won on T/CT and
   recommendations. Your recommendations precede collapsed teammate advice.
   Existing recommendation prose retains its saved language and detail.
5. A new analysis defaults to the signed-in player. The optional party editor
   retains the saved selection on existing analyses. Steam premade membership
   is not captured, so automatic premade selection is not implemented.
6. Round history shows win/loss, side and your kills each round. The scoreboard
   shows recorded kills, deaths, assists, headshot kills and thrown flashes.
7. Request recommendations: the page checks status in the background, reloading
   only on a status transition. Without JavaScript, use the manual check link.

## Limits

Scores need a known participant and their side in every recorded round;
unknown or incomplete perspectives display no result. Scores describe the
recorded rounds, not a separately verified official result for truncated demos.
Full ADR, HLTV rating and Steam premade membership require additional data or
implementation. No aim, spray or counter-strafe diagnosis has been added.
Administrators can explicitly open `?diagnostics=1` for the existing telemetry;
ordinary users cannot enable it with that query.

## Checks

Score tests cover side switches, draws, missing participants and incomplete
perspectives. Scoreboard tests cover suicides and team kills. Handler checks
cover page order, hidden identifiers, telemetry visibility, localization and
the authorized non-cacheable status endpoint. PostgreSQL integration verifies
that the match list retains round outcomes without fetching event telemetry.

The local PostgreSQL suites run with `-p 1` because the existing temporary
database naming helper can collide between packages. No database migration
or saved-advice rewrite is needed.

`go build ./...`, `go vet ./...`, `golangci-lint run` and the full
`go test -race -p 1 ./...` suite with PostgreSQL pass. The polling
script was exercised in Node for unchanged status, completion, network failure
and unauthorized responses. Anonymized HTML previews were rendered in headless
Edge and visually inspected at 1100 × 1000.
