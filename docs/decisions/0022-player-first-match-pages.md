# 22. Put the player's match and recommendations first

**Status:** accepted.

## Decision

The home page starts with recent matches and their score from the signed-in
participant's perspective, followed by weekly briefs. Manual briefs are an
optional early request; Friday generation needs no click. Telegram controls
and manual uploads are collapsed at the bottom.

Match pages show the result and recommendations before round history and the
scoreboard. Demo filenames and internal IDs are absent from page content.
The party defaults to the saved selection or the signed-in participant;
editing it is optional. The model does not capture Steam premade membership,
so teammates are not silently treated as friends.

Scores follow the participant's side on every round, including overtime.
Unknown participants or missing round outcomes do not get a fabricated score.
The list projects only round winners and player snapshots in one existing
database query, without fetching full analyses per match or a schema migration.

The scoreboard uses recorded kills, deaths, assists, headshot kills and thrown
flashes. Full ADR and HLTV rating are not available: firearm-only damage is
insufficient for ADR, and no rating formula is implemented. No mistake-count
severity badge is introduced because it would not measure coaching priority.

Raw shooting observations are available only with an explicit administrator
diagnostics query. They do not become unvalidated mechanical advice.

This supersedes ADR 5's no-JavaScript decision only for pending match pages:
a small inline script polls an authorized, non-cacheable status endpoint and
reloads on a status transition. It never submits coaching requests. Without
JavaScript there is a manual check link. Other pages remain server rendered.
Advice text and grounding contracts are unchanged.
