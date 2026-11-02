# 12. The share-code bot: a small Node.js process, downloads stay in Go

**Status:** accepted (v0.3)

## Context

ADR 11 collects a player's new share codes. To turn a code into a demo, a
Steam client has to be signed in, "play" CS2 and ask the Game Coordinator
for the match (`MatchListRequestFullGameInfo`); the answer carries the URL of
the demo on Valve's replay servers. That means speaking Steam's client
protocol: an encrypted connection to a connection manager, the token-based
sign-in Valve introduced in 2023, and CS2's protobuf messages.

Go has no maintained library for this: the known ones predate the new
sign-in. Node.js has `steam-user` and `globaloffensive` (both by the same
maintainer, updated in 2025–2026), which are what most share-code services
build on.

## Decision

- **A separate Node.js bot, as small as possible.** `bot/` only resolves
  codes: it claims a waiting code from the api, calls `requestGame`, and
  reports the demo URL and match time, or that the demo expired. Everything
  else (download, dedupe, quotas, parsing, coaching) stays in Go. The bot is
  one dependency-light process with two pinned packages, installed with
  `--ignore-scripts`, and its logic is tested against a fake Game
  Coordinator.
- **A dedicated Steam account.** The bot signs in as an account that owns
  nothing and is linked to nothing else: a leaked session or a Valve
  restriction costs nothing. `node bot.js login` asks for the password and
  the Steam Guard code on the terminal and stores only a refresh token
  (renewed on every start, so it does not run out after ~200 days).
- **The bot talks to the api like a worker,** over HTTPS with the worker
  token, through `/internal/sharecodes/*`, and runs on any machine; it needs
  no database and no open port.
- **The api downloads the demo,** not the bot: the file goes straight to the
  server that queues it, instead of down to the bot's PC and back up. The URL
  comes from the bot, so the api only fetches from `*.valve.net` on the
  default port; anything else is refused, which keeps the worker token from
  becoming a way to make the server fetch arbitrary addresses. Every
  redirect hop is checked the same way. `.bz2` demos
  are unpacked while streaming and go through the upload path: magic check,
  SHA-256 dedupe (a demo the watcher already uploaded is linked, not
  analyzed twice), size limit and the weekly quota.
- **One queue in the `share_codes` table.** `waiting → resolving` (bot) and
  `download → downloading` (api) are claimed with `FOR UPDATE SKIP LOCKED`,
  like jobs. A claim older than ten minutes (thirty for downloads, which
  time out after twenty) is taken over, so a bot or api that died mid-way
  does not strand a code; after five attempts a code fails, keeping its last
  reason. Final states are `imported`, `expired`, `over_quota` and `failed`,
  each with a reason the Auto-upload page shows.
- **Answers are matched by match ID.** The bot decodes the match ID from the
  share code and ignores Game Coordinator answers about other matches, so a
  late answer to a timed-out request is never filed under the next code. Bot
  reports only apply to a code the bot still holds (`resolving`), so a late
  report cannot undo a code that was taken over and imported meanwhile.
- **Pacing.** One request at a time, five seconds apart, a minute between
  polls when idle: the Game Coordinator throttles fast clients, and a party
  plays a few matches a day.

## Consequences

- Fully automatic import: play, and the report appears without opening CS2,
  as long as the bot's machine is on.
- A second runtime in the repository. It is contained in `bot/`, has its own
  CI job, and can be replaced by a Go client later without changing the api:
  the contract is three HTTP endpoints.
- Over quota, a match is not downloaded at all (`over_quota`) rather than
  waiting, so a busy week cannot pile up a backlog that eats next week's
  quota.
- Valve keeps demos for about a month; codes older than that end as
  `expired`.
