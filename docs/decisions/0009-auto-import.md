# 9. Auto-import: watch the replays folder first, a Steam bot later

**Status:** accepted (v0.3)

## Context

Uploading a 300 MB `.dem` by hand after every session is the step players
skip. SCOPE plans auto-import through match sharing codes. The research spike
found two halves to that:

1. The Steam Web API (`GetNextMatchSharingCode`) lists a player's new share
   codes, given their match-history auth code. That part is a plain HTTPS
   call.
2. Turning a share code into a demo URL needs the CS2 Game Coordinator, which
   only answers a logged-in Steam client that "plays" CS2. Services that do
   this (Leetify, cs-demo-downloader) run a separate bot account.

The bot is a second Steam account with Steam Guard, and it has to stay signed
in. That is acceptable, but it should not block auto-import.

## Decision

Ship it in two steps.

- **Now: `csdc watch`.** The player presses Download in CS2 (Watch → Your
  Matches), the game saves the demo to the replays folder, and a small
  command on the same PC uploads it. It finds the folder through Steam's
  `libraryfolders.vdf`, waits until a file stops growing (a download in
  progress must not be uploaded), and keeps a state file so restarts do not
  upload twice. On the first run it skips demos that were already there,
  unless `-all` is passed, so it does not burn a week's quota on old matches.
- **Upload tokens, not the worker token.** The watcher signs in with a
  per-user token made on the Auto-upload page, so uploads count against that
  user's quota and show up as theirs. Like sessions, only a SHA-256 of the
  token is stored; the page shows it once, and it can be revoked.
- **Server-side dedupe.** Matches store the demo's SHA-256, unique per
  uploader. Uploading the same file again, from the watcher or the browser,
  returns the first match instead of analyzing it twice. A match that failed
  gives up its hash, so a demo that hit a since-fixed parser bug can be
  uploaded again.
- **Match dates.** The watcher sends the file's time, which CS2 sets when it
  saves the demo right after the match. History, practice and the weekly
  brief use that date; browser uploads fall back to the upload time.
- **Later: share codes through a bot.** When the bot account exists, an
  importer polls share codes, asks the Game Coordinator for the demo URL and
  feeds the same upload path. The watcher stays as the zero-setup option.

## Consequences

- No new accounts, no Valve client emulation, nothing that can get banned.
  It works today on any player's PC.
- It still needs one click in the game per match, and the PC must be on.
  The bot removes both.
- A demo downloaded long after the match is dated by the download. Exact
  dates from the share code come with the bot step.
- The watcher pauses for an hour when the weekly quota is used up, and a
  30-minute limit per upload keeps a stalled connection from freezing it.
