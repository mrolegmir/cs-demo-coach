# Interface localization walkthrough

The interface now opens in Russian. Use **Русский · English** in the header to
switch languages; the choice is remembered in this browser for one year. The
selector works before signing in and returns to the same page and query.

## Try it locally

1. Open the home page in a fresh browser profile: headings and upload controls
   are Russian, even if the browser prefers English.
2. Choose English. Open a match and a player's progress: the interface stays
   English. Choose Русский to switch back.
3. Check Steam sign-in, invitations, auto-upload, weekly briefs and the practice
   log. Drill titles, instructions and success criteria follow the interface.
4. With Русский selected, request new match coaching or a new weekly brief.
   Titles, explanations, priorities and practice verdicts should be Russian.
   Telegram delivery uses the generated report language. Select English before
   the next request to generate English advice instead. Automatic weekly briefs
   default to Russian.
5. Inspect an existing coaching report: its saved prose and evidence retain
   their source language. Switching the interface does not regenerate it.

For example, a newly generated priority explanation now starts with
«Приоритет 1 из 2: примерно на … эпизода больше уровня лобби …» rather than
“Priority 1 of 2 …”. This is computed from the same counts and ranking.

CLI: csdc analyze -lang ru demo.dem (default) or -lang en for English advice.

## Adding text

Wrap visible interface text in a template tr call and add its Russian
translation to internal/web/locales/ru.json. Use complete messages with format
arguments where sentence structure depends on dynamic values. Preserve
protocol values and user content. Catalog coverage tests also check every
curated drill title, location, step and success criterion. Rules-coach prose
lives in internal/coach/locales/ru.json; keep format verbs in the same order.

See [ADR 21](decisions/0021-interface-localization.md) for persistence,
concurrency and the boundary between interface and generated report content.

## Verification

- Build, go vet, golangci-lint and go test -race ./....
- Tests cover Russian defaults, invalid cookies, explicit English, concurrent
  rendering, all page templates, cookie persistence, allowed languages,
  cross-site POST rejection and local return destinations.
- Existing interface regressions request English explicitly.
- Database integration tests use CSDC_TEST_DATABASE_URL, including migration 10,
  a remote HTTP worker, retry language persistence, manual brief input snapshots
  and the scheduled Russian default.
- Coach regressions check matching drill/evidence selection across languages,
  Russian named team plans, practice/quiet reports, catalog formatting, and
  language instructions on both LLM attempts. Model tests use a scripted
  provider; no live LLM call or Telegram message is required for verification.

Deployment requires migration 10 and rebuilt API and worker binaries. Old
reports remain unchanged. The deployment preserves the existing database and
retains a pre-release database backup and API/worker binaries for rollback.
