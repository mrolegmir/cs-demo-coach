# 8. Weekly brief: the party's habits before the weekend

**Status:** accepted (v0.2)

## Context

The target party plays Friday to Sunday. SCOPE asks for a brief before the
usual play days: focus areas per player plus a team section, built from the
week's matches, within the same LLM budget and grounding rules as
single-match coaching.

## Decision

- **Who:** the owner's usual party for the week. That is the owner plus
  every *teammate* (same starting side) who played at least half of the
  owner's matches that week (and at least two), up to five players.
  Opponents never count, even a group met twice. Nobody has to configure a
  party, and teammates do not need accounts.
- **Privacy:** teammates are judged only on matches they played with the
  owner. The owner cannot open a teammate's other matches, so the brief does
  not quote them.
- **What the coach sees:** for each member, issues come from the whole
  history (ADR 7), not just the week, so a quiet week does not hide a habit.
  Only active and improving issues may become focus areas; resolved ones and
  bad games are passed along as context for a short mention. Examples are the
  member's latest findings, labelled with map and date. Finding IDs are
  prefixed with the match (`m12-f40`) because IDs repeat across matches.
- **Same coach, same contract:** the brief is a `coach.Input` like a single
  match, and it goes through the same `Validate`. The API builds the input
  when the brief is requested and stores it, so the remote worker only runs
  the coach on it.
- **When:** a planner in the API checks every 10 minutes. After Friday 15:00
  (`CSDC_BRIEF_HOUR`, `CSDC_TZ`), it queues a brief for every user who played
  in an analyzed match that week and has no brief covering it yet. A manual
  "make my brief now" counts as one. Only manual briefs use the coaching
  quota; scheduled ones are free.

## Consequences

- On the owner's 9 real matches, with ChatGPT through the Codex provider, the
  brief found a shared party habit (no utility thrown on full buys, all three
  above the lobby rate) and one player's dry entries. It mentioned a resolved
  issue in the notes.
- Delivery is the web page only; a Telegram or email notification is a
  natural next step.
