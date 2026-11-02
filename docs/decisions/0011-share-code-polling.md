# 11. Share codes: poll Steam now, fetch demos when the bot exists

**Status:** accepted (v0.3)

## Context

ADR 9 split Steam auto-import in two: listing a player's new matches is a
plain Web API call, while turning a share code into a demo URL needs the
Game Coordinator and a bot account that does not exist yet. The first half
needs nothing but a Web API key, so it ships on its own.

## Decision

- **Users link their history on the Auto-upload page** with the match
  history authentication code (Steam help → CS2 → "Access to Your Match
  History") and the share code of their newest match. Both are checked for
  form; Steam checks the rest on the next round.
- **The api polls**, not the home worker: it already holds the database and
  runs the weekly-brief loop, and the Web API key is a server secret. Each
  link is checked about every 30 minutes; one check reads at most 20 codes, so
  a link set to an old code catches up over a few rounds.
- **Only the user's own mistakes disable a link.** Steam's 412 (the code is
  not theirs) and 403 (the auth code was revoked) store a message the page
  shows, and the link is skipped until the user saves it again. Steam answers
  a bad API key with the same 403, so before blaming the user the importer
  checks the key with a call that needs nothing else; a bad key stops the
  round and is logged, and nobody's link is touched. Any other failure
  (network, 5xx, a code Steam sends in a form we don't parse) still records
  the check with the codes found before it, so the link waits the usual 30
  minutes instead of hammering Steam during an outage.
- **The auth code is stored as is.** It has to be sent to Steam, so it can't
  be hashed like tokens. It only grants reading the match list, the page
  shows just its last group, and Steam errors are stripped of the request URL
  before they are logged, because the URL carries both the code and the key.
- **A stale check is dropped.** Recording a check updates the link only if
  it still has the auth code and newest code the check started from, so
  saving a new link mid-check can't be overwritten by the old walk.
- **The starting code counts as a found match.** The importer only finds
  codes after it, and it is the match the player just played.
- **Found codes wait** in `share_codes` with status `waiting`. The bot step
  will resolve them through the Game Coordinator and feed the upload path,
  setting `match_id`; until then the page tells the user to download the
  match in CS2 and let the watcher upload it.

## Consequences

- Turning it on is one env var (`CSDC_STEAM_API_KEY`); without it the form
  is hidden and nothing polls.
- Players see their new matches listed without opening CS2, but reports
  still need the demo: the value lands fully only with the bot.
- The share code also carries the match ID, which will give exact match
  dates once the Game Coordinator returns the match info.
