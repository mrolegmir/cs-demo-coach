# 14. A curated drill library the coach must pick from

**Status:** accepted (v0.5)

## Context

ADR 3 grounds the *why* of every focus area: it must cite findings, and
`Validate` rejects advice that does not. The *drill* was not grounded. The
LLM wrote it freely, so the same habit got a different drill every week,
some drills were vague ("practice your utility"), some named workshop maps
or commands that do not exist, and the practice log could not tell which
drill a player had tried. SCOPE's open question 2 asks where coaching
knowledge comes from; for drills, the answer can be a small library we
write and review once.

## Decision

- **`internal/drills` is the library:** two drills per kind of mistake for
  now, each with steps, where it is done (offline server, your matches,
  with your party), how long it takes, and a check that says when it is
  done right. Offline drills share one practice-server setup with real CS2
  console commands. The library is Go data with a test that every kind has
  at least two drills, so there is always an alternative.
- **The coach picks, the page explains.** A focus area carries a
  `drill_id`. The LLM gets the library entries for the kinds that stand out
  and must choose one; its `drill` text now says how that drill applies to
  this player's rounds instead of inventing one. The page renders the steps
  and the check from the library, so they read the same everywhere.
- **`Validate` checks drills like evidence:** the ID must exist and belong
  to the focus area's kind. When a player logged a drill for an issue and
  the practice verdict is "not working yet" (ADR 10), choosing that drill
  again is rejected while another one exists. The rule-based coach follows
  the same rule.
- **The practice log knows drills.** Each drill card links to the practice
  form with the drill selected; an entry with a drill takes its kind, and
  its title and time when the note or minutes are left empty. The brief
  passes the drills a player logged to the coach (owner only, like notes).
- **Old advice keeps working.** Advice stored before the library has no
  `drill_id`; pages show its free-text drill as before.

## Consequences

- On a real match with ChatGPT through the Codex provider, the model picked
  a valid drill for every focus area (seven across five players) and tied
  each to the player's rounds in one sentence.
- The library is the place to improve drills, and a change reaches every
  report. It is also a limit: a habit gets one of two drills, however
  specific the situation. Adding drills is cheap; adding map-specific ones
  (lineups per map) is the natural next step.
- Practice verdicts are still per mistake, not per drill: with a handful of
  matches a week, splitting them further would leave nothing to compare.
