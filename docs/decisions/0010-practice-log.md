# 10. Practice log: judge a drill by the numbers, not by feel

**Status:** accepted (v0.3)

## Context

SCOPE's longitudinal memory includes "the player's own practice notes":
the coach should know what the player trained and answer "is this drill
working?". The risk is the same as with coaching in general: an LLM handed
a note like "did 30 minutes of flash lineups" will happily say it helped.

## Decision

- A practice entry is a date, an optional mistake kind, minutes and a short
  note. Players add them on their own progress page; only they see the notes.
- The **effect** is computed, not judged by the LLM: `history.Issue.Since`
  splits the issue's matches at the first session of that kind and compares
  the player's rate with the lobby's before and after.
- The **verdict** is deterministic. `too_early` until three matches were
  played since; `no_baseline` if no match was played before the practice
  started; `working` when the gap to the lobby shrank by at least a third
  (or, for a player who was not above the lobby, did not grow); otherwise
  `not_yet`. Comparing gaps rather than raw rates
  controls for lobbies that happen to be easier or harder.
- The coach gets the numbers and the verdict for the issue, and the owner's
  latest notes. The prompt asks it to say whether the drill pays off with
  those numbers, and to suggest a different drill on `not_yet`. Teammates'
  practice reaches a brief as numbers only, since their notes are theirs.

## Consequences

- The coach cannot claim a drill worked when the numbers say otherwise; the
  rules coach writes the same sentence without an LLM.
- Three matches is a small sample, so `working` means "pointing the right
  way", not proof. The page shows the rates and the number of matches next to the verdict for that
  reason.
- Practice not tied to a mistake is kept in the log but does not affect any
  issue.
