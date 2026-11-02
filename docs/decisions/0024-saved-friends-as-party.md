# 24. Use an owner's saved friends to select the coaching group

**Status:** accepted.

## Decision

The owner explicitly defines who counts as their group: themselves plus saved
friends on the same starting team in a match. This is a user preference, not
an inference of Steam premade membership. Real Game Coordinator responses did
not provide usable party IDs in the validation sample.

`party_friends` stores owner-scoped SteamIDs, without requiring friends to
have application accounts. The settings page offers teammates from the user's
last twenty matches and numeric Steam profile IDs. Nicknames are presentation,
never identity. Unknown profiles can be saved before their first shared match.
There is no Steam friends import or new Steam API dependency in this version.

Automatic coaching is explicitly enabled per user and defaults off. On a
successful API detection submission, a transaction locks the match and owner,
checks the rolling coaching quota, snapshots the group and queues one Russian
coaching job. The existing remote worker protocol is unchanged. If the owner
is not a participant, no group or request is fabricated. Quota exhaustion
leaves detection available for manual coaching. Replayed detection does not
overwrite coaching or completed reports; automatic requests do not overwrite
an existing selected group.

Weekly briefs select only saved friends who shared the owner's team during
the period, including friends seen once. They use at most four friends, most
frequent first with SteamID tie breaking. A friend's history is restricted to
matches on the owner's team. A new explicit manual exclusion from a match
also excludes that match from the friend's weekly history. Earlier snapshots
are not retroactively declared manual exclusions.

## Consequences

Migration 12 adds friend settings and a manual-selection flag. Existing advice
is retained; changing friends does not requeue old reports. An explicit match
action can request a new report using the current friends. Manual selections
remain supported. Only the API needs replacing in the deployed remote-worker
setup; no bot or home worker restart is needed.
