# CS Demo Coach

[![CI](https://github.com/mrolegmir/cs-demo-coach/actions/workflows/ci.yml/badge.svg)](https://github.com/mrolegmir/cs-demo-coach/actions/workflows/ci.yml)
![Go 1.26](https://img.shields.io/badge/go-1.26-00ADD8?logo=go)
[![License: MIT](https://img.shields.io/badge/license-MIT-blue.svg)](LICENSE)

CS2 demo analysis with evidence-linked coaching, personal match review and progress across matches.

For players who play a few sessions a week, the useful question is what to work on next.
CS Demo Coach finds recurring mistakes, compares their frequency with the lobby, and
turns the strongest signals into a short practice plan. Each recommendation links back
to recorded rounds and events. You can review your own play and a saved group of friends.

**Current source snapshot: v1.1.1.** This public repository is a source snapshot;
ongoing development takes place privately. Publishing source is separate from deploying
a hosted instance. See the [v1.1 walkthrough](docs/v1.1-walkthrough.md) and
[v1.1.1 reliability notes](docs/v1.1.1-walkthrough.md).

## What you can do

- **Review a match from your perspective.** Result, round history and a team-grouped
  scoreboard lead into seven categories: Utility, Teamplay, Positioning, Economy,
  Shooting, Movement and Decisions. Checks distinguish episodes to review,
  no detected occurrences and unavailable assessment.
- **Get a grounded practice plan.** Deterministic detectors cover isolated deaths,
  missed trades, wasted/team flashes, unused utility, low utility usage, mismatched
  buys, missed drops and unsupported opening deaths. Saved priorities retain their
  evidence and order. Exercises come from a validated drill library.
- **Inspect additional observations.** Rifle sequences show sampled movement and,
  where captured, view-to-eye geometry before damage. HE/fire damage, lost advantages,
  bomb timelines and flash follow-up have separate event evidence. These observations
  are review aids, not validated aim, counter-strafe or decision-quality diagnoses.
- **Analyze with friends.** Save friends by SteamID, optionally coach new matches
  automatically, and override the selected group for individual matches. Trade pairs
  and observed entry/support/AWP activity inform team advice; Steam premade membership
  is not inferred.
- **Track habits and practice.** Progress pages compare player and lobby rates across
  matches. Start an exercise, then log completed minutes. Friday briefs summarize
  individual priorities and team practice; optional Telegram delivery brings them to you.
- **Import with less manual work.** Upload a demo, watch the local CS2 replays folder,
  or link Steam match history. Share-code downloads use an optional Game Coordinator bot.
- **Use Russian or English.** The interface defaults to Russian, with an English switch.
  New manual coaching follows the selected language; saved advice keeps its language.

![Personal match review with anonymized players](docs/img/v1.1-match.png)

The example uses an anonymized bundled match fixture. See the
[documentation index](docs/README.md) for category details and earlier coaching reports.

## Design worth reading

**The LLM explains computed findings.** Detectors and statistical eligibility decide
what can become a focus. `coach.Input.Validate` checks player IDs, finding references,
eligible kinds and library drills. Rules produce advice without a model; optional LLM
output passes the same grounding contract. Comparison with the lobby is a heuristic
for prioritization, not a calibrated skill rating.

**PostgreSQL is also the queue.** Workers claim jobs using `FOR UPDATE SKIP LOCKED`.
Retries, stale-attempt fencing and atomic result/job completion keep late workers from
overwriting newer attempts. The API retries demo cleanup after committed detection.
Weekly quotas are checked transactionally when work is admitted.

**Heavy work runs elsewhere.** A pull worker can run on a home PC over HTTPS, without
inbound ports or direct database access. The server hosts the API and PostgreSQL.
Only `internal/demo` imports the demo parser; the remaining pipeline uses
`internal/match`. Sorted parser output, anonymized fixtures, golden reports and
database integration tests make changes reviewable.

Stack: Go 1.26, standard-library HTTP and `html/template`, pgx, goose, PostgreSQL,
Docker Compose; optional Node.js Steam bot. Pages are server-rendered, with a small
progress-polling script and a manual refresh fallback.

## Quick start

Install Docker with Compose, then:

```bash
git clone https://github.com/mrolegmir/cs-demo-coach.git
cd cs-demo-coach
docker compose up --build
```

Open **http://localhost:8080**, upload a CS2 `.dem`, select players and request coaching.
The stack starts PostgreSQL, the API and one worker using the offline rule-based coach.
Local mode has no sign-in. Ports 8080 and 55432 must be available.

For an optional LLM, copy [.env.example](.env.example) to `.env` and configure the
provider before starting Compose. No model or API key is required for the default.
See [setup and configuration](docs/setup.md) for LLM endpoints, Steam sign-in,
remote workers, automatic imports and Telegram.

With Go 1.26, command-line analysis needs no database:

```bash
go run ./cmd/csdc analyze match.dem
go run ./cmd/csdc analyze -party "a,b,c" match.dem
go run ./cmd/csdc detect match.dem
# Anonymized mechanics observations from a bundled fixture:
go run ./cmd/csdc mechanics testdata/matches/mm_2026_dust2.json.gz
```

## Architecture

```text
browser / replay watcher ──► API ──► PostgreSQL
                            │       matches, findings, friends, practice,
                            │       briefs and transactional job queue
                            │
                       HTTPS + token
                            │
                         worker
                  parse → detect → coach
                    (rules or optional LLM)

Optional: Steam Web API → share codes → Node.js GC bot → demo download
          Telegram Bot API ← weekly brief delivery
```

| Code | Responsibility |
|---|---|
| `internal/match`, `internal/demo` | Parser-independent model, observations, anonymization; parser adapter |
| `internal/detect`, `internal/coach`, `internal/drills` | Findings, prioritization, grounding and exercises |
| `internal/store`, `internal/jobwire`, `internal/worker` | Migrations, quotas, durable queue, worker protocol and execution |
| `internal/history`, `internal/brief` | Trends, practice comparisons, weekly snapshots and scheduling |
| `internal/web`, `internal/auth` | Personal pages, RU/EN, uploads, Steam sign-in, friends and worker endpoints |
| `internal/watch`, `internal/sharecode`, `internal/importer`, `bot/` | Replay watching and Steam imports |
| `internal/telegram` | Chat linking and brief delivery |

The [30 architecture decisions](docs/decisions) explain the trade-offs.
[SCOPE.md](SCOPE.md) describes the product and remaining work.

## Development and validation

```bash
go build ./...
go vet ./...
golangci-lint run
go test -race ./...

# Include PostgreSQL integration tests (Bash environment syntax):
docker compose up -d db
CSDC_TEST_DATABASE_URL=postgres://csdc:csdc@localhost:55432/csdc go test -race -p 1 ./...

cd bot
npm ci --ignore-scripts
npm test
```

Go tests skip database integration without `CSDC_TEST_DATABASE_URL`. Parser tests
requiring raw demos skip when those files are absent; compact anonymized fixtures
and detector goldens are included. CI runs vet, lint, race tests with PostgreSQL 17,
and the bot tests. For PowerShell, set `$env:CSDC_TEST_DATABASE_URL` before running Go.

To update expected detector reports, run
`go test ./internal/detect -run TestGolden -update` and review the diff.
Raw demos stay local in the gitignored `demos/` directory; fixture regeneration uses
`go test ./internal/demo -update`. Screenshots and shared examples must use anonymized data.

## Limits

Historical captures cannot gain missing telemetry without parsing the demo again.
Source demos are removed after successful detection; saved results remain.
No rank benchmarks, official rating, verified visibility reconstruction or validated
aim/spray/counter-strafe diagnosis is provided. Practice comparisons are observational,
not proof that a drill caused improvement. See the [skill catalog](docs/skills-catalog.md)
for assessment boundaries.

When upgrading an existing installation to v1.1.1, stop/drain the old workers and
update API and workers together: protocol v2 rejects legacy writes. Migration 13
is additive. Follow the [rollout notes](docs/v1.1.1-walkthrough.md#rollout-requirements).

## License

[MIT](LICENSE)
