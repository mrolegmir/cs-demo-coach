# CS Demo Coach — Product Scope

## Problem

CS2 analytics tools dump dozens of stats and generic tips. They are built for players who
grind every day. Someone who plays ~7 hours a week can't act on twenty suggestions — they
need to know the one or two things that matter most right now, and why.

## Who it's for

Casual-but-improving players who play a few sessions a week (typically on weekends), often
in a fixed party of 2–5 friends. Not pros, not aspiring esports players.

## Product principles

1. **Prioritize, don't overwhelm.** At most 1–3 focus areas per player per week, ranked.
2. **Evidence over vibes.** Every recommendation cites concrete occurrences (match, round,
   timing), how often it recurs, why it outranks other issues, and how to practice it.
   If there isn't enough data yet, the coach says so instead of guessing.
3. **Team-aware.** The party is analyzed as a unit, not only as individuals: trades, utility
   support, role split, a wider strategy pool ("you three can run X: one player does A,
   another B…").
4. **Longitudinal memory.** Each player's issues are tracked across all history. The coach
   recognizes improvement, demotes one-off bad games, re-prioritizes, and takes the
   player's own practice notes into account ("is this drill working?").
5. **Cheap to run.** LLM usage is the main cost, so it is budgeted and minimized by design.

## Core flows

- **Import matches.** Upload a `.dem`, watch the local replays directory, or link Steam
  match history for share-code import. Third-party stats enrichment remains future work.
- **Select what to analyze.** Request coaching per match, or opt in to automatic coaching
  for new matches. A saved friend list selects the owner's group from their team;
  individual matches allow manual overrides. Weekly briefs use the saved friends too.
- **Weekly brief.** Matches accumulate during the week; before the usual play days a brief
  is generated — focus areas per player plus a team section. On-demand analysis is also
  available within quota.
- **Practice log.** A player reports what they practiced; the coach factors it in next time.

## How coaching works (key design decision)

Finding problems and explaining them are separate steps:

1. **Parse** the demo into an event stream (`markus-wa/demoinfocs-golang`).
2. **Detect** — deterministic, rule-based detectors turn events into structured findings
   with evidence (e.g. "untraded death on B entry — rounds 4, 9, 17, 21, ~0:45 left";
   "flashes with zero enemy blind time"; "deaths while reloading"). Reproducible and
   unit-tested against real demos.
3. **Track** — findings feed a per-player issue history (first seen, frequency, trend,
   status: active / improving / resolved / not enough data).
4. **Coach** — an LLM receives compact findings + history (never raw demos), prioritizes,
   explains *why*, and suggests drills. Output is schema-validated, and every claim must
   reference finding IDs — no ungrounded advice.

This makes advice trustworthy (evidence is computed, not hallucinated), cheap (small
prompts), and testable.

## Users and limits

- Invite-only, a handful of users at first.
- Per-user quotas (analyses per week / token budget), enforced server-side.

## Non-functional requirements

- **Minimal running cost.** A small VPS hosts web/API/DB; heavy demo parsing runs on a
  worker that can live on a home machine and *pulls* jobs (no inbound exposure needed).
- **Replaceable dependencies.** Steam import, third-party stats, and the LLM provider sit
  behind interfaces. Using ready-made services early is fine as long as they can be swapped.
- **Pluggable LLM** — hosted API or a local model.
- **Async processing.** A match analysis may take minutes; the weekly cadence allows it.
- **Tests as the contract.** Detectors are fixture-tested on real demos; coach output is
  validated against a schema.

## Architecture

```
 browser ──► api (Go) ──────────► Postgres ◄──────── worker (Go)
             auth, uploads,        users, matches,     pulls jobs, downloads/parses
             quotas, reports       findings, issues,   demos, runs detectors
                │                  reports, job queue
                ▼
             coach (Go) ──► LLM provider (interface)
```

- **api** — HTTP, auth, uploads, match selection, quotas, report pages.
- **worker** — separate process because it has different resource and deployment needs
  (CPU-heavy parsing, can run on a home machine).
- **coach** — builds the prompt from findings + history, calls the LLM, validates output.
- **Postgres** — the only stateful dependency; also serves as the job queue
  (`SELECT … FOR UPDATE SKIP LOCKED`), so no separate broker is needed at this scale.
- After detection is committed, the API deletes source demos with durable retries;
  saved match data and findings remain.

## Phases

**v0.1 — core loop**
- Upload a demo → parse → 3–5 detectors → findings per player and for the party.
- LLM coaching for that match, every point linked to evidence.

**v0.2 — usable by a party**
- Calibrate detectors on real 2026 matchmaking demos (replace the weak buy-desync
  detector, add low-utility-usage).
- Steam sign-in (moved up from "Later"): an account is a SteamID, so a player's matches
  and history link up without manual setup. Invite-only, per-user quotas.
- Issue history across matches; weekly brief on Friday, before the usual weekend games.
- Deploy: API and Postgres on a small VPS; the worker runs on a home PC and pulls jobs.

**v0.3 — less manual work**
- Auto-upload from the CS2 replays folder (`csdc watch`, per-user upload tokens). Done.
- Practice log with a computed "is the drill working" verdict. Done.
- Steam match history polling: new share codes found through the Web API (ADR 11). Done.
- Demo download for those codes through a Game Coordinator bot account (ADR 12). Done.

**v0.4 — the coach comes to you**
- Weekly brief delivered to Telegram when it is ready (ADR 13). Done.
- Trend chart per mistake on the progress page: the rolling rate over three matches
  against the lobby, with the start of practice marked. Done.

**v0.5 — drills you can trust**
- A curated drill library (ADR 14): the coach must pick a drill from it, validated like
  evidence, and pick another when a logged drill is not working. Done.
- Drills in the practice log, one click from a drill card. Done.

**v0.6 — where on the map**
- Findings carry the game's place names (ADR 15), counted per mistake across matches. Done.
- Drills name the spot to start with; lineup drills use community utility guides in
  practice mode. Done.

**v0.7 — the party as a unit**
- Trade pairs (ADR 16): who trades whom, on the progress page and in team advice. Done.

**v0.8 — roles**
- Entry, support and AWP numbers per player (ADR 17), and team advice that names who
  flashes for the entry. Done.

**v1.0 — complete coaching loop**
- Explain each focus area's priority with computed excess occurrences against the lobby.
- Keep weekly history and evidence consistent at the brief's cutoff, including player name changes.
- Validate every worker coaching result, cap team items and reject repeated focus areas.
- Recognize resolved habits, avoid LLM calls when no focus is eligible, and provide a named
  three-player flash/entry/trade practice plan when role evidence supports it.
- Requirements coverage and release limits: [v1.0 walkthrough](docs/v1.0-walkthrough.md).

**Localization — implemented**
- Russian interface by default with an English switch, remembered in the browser.
- New match and manual weekly recommendations use the selected language;
  scheduled weekly briefs default to Russian. Saved reports retain their language.
- [Localization walkthrough](docs/localization-walkthrough.md), ADR 21; migration 10.

**v1.1 — expanded match review**
- Player-first pages, seven review categories, inline practice sessions and saved friends.
- Personal Shooting and Movement expose rifle episodes, sampled speed before
  and during firing, and capture-4 view/eye geometry before uniquely linked damage.
- Utility, flash follow-up and decision review provide additional grounded
  context. No new mechanics or decision-quality score is introduced.
- Release behaviour, data compatibility and rollout checks: [v1.1 walkthrough](docs/v1.1-walkthrough.md).

**Additional v1.1 review evidence**
- Teamplay review: [flash follow-up](docs/flash-follow-up-walkthrough.md)
  shows recorded blindness and conservative kill timing from saved matches.
  This is review evidence, not an official assist or grenade-effectiveness score.
- Decision review: [lost advantages and bomb context](docs/decisions-walkthrough.md)
  provide personal replay situations without a decision-quality score. Saved
  death snapshots support advantage review; bomb timelines need a fresh capture.
- Utility outcomes: [HE/fire damage review](docs/grenade-damage-walkthrough.md)
  adds recorded personal enemy/team/self damage and event evidence. Smoke quality
  and area denial remain unassessed; damage is not a grenade-effectiveness score.
- Mechanics analysis: the [shooting observations pilot](docs/mechanics-pilot-walkthrough.md)
  captures shots and reports coverage. Aim diagnoses remain unvalidated; see the
  [skill catalog](docs/skills-catalog.md) for research and promotion criteria.
  The [pre-shot movement extension](docs/pre-shot-movement-walkthrough.md)
  captures half a second before firing for episode review; mechanical diagnoses
  and verified contact/visibility remain future work.

**v1.1.1 — reliability**
- Atomic analysis/job completion, fenced worker attempts and durable API-owned cleanup.
- Stable player IDs throughout coaching validation, including duplicate nicknames.
- Transactional shared quotas and automatic-coaching parity for direct/HTTP workers.
- Migration 13 and coordinated protocol-v2 upgrade: [walkthrough](docs/v1.1.1-walkthrough.md).

**Later**
- Third-party stats enrichment and benchmarks for the player's rank.
- Validated mechanics assessment, visibility/contact reconstruction and smoke/area-denial review.

## Design questions

1. **Steam auto-import** — answered in ADR 9: share codes come from the Web API, demo URLs
   need a Game Coordinator client on a separate bot account.
2. **Where coaching knowledge comes from** — for drills, answered in ADR 14: a curated
   library the coach picks from. Benchmarks for the player's rank are still open.
3. **Third-party providers** — which ones expose usable APIs.
4. **Frontend** — answered in ADR 5: server-rendered `html/template`, with a small
   match-status polling script and a manual refresh fallback.
5. **First detectors** — answered in ADR 2; the current checks and assessment limits
   are listed in the [skill catalog](docs/skills-catalog.md).
