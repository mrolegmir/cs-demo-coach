# Friends and automatic team coaching

Open **People I play with** in the signed-in navigation. Select friends among
recent teammates, or add a SteamID64 / numeric Steam profile link. Names may
change; the saved SteamID identifies the friend. Opponents are not suggested
as teammates. Friends do not need an account in this application.

Enable **Automatically analyze new matches** and save. A newly processed
match queues coaching for the owner and saved friends on the same starting
team. Friends on the opposing team and other teammates are excluded. The
weekly coaching quota still applies. Without the option enabled, friends
are preselected for manual analysis and used in the weekly brief.

The match page shows the selected group. Saved reports retain their group;
under **Change party**, **Analyze with friends from my list** explicitly
requests a fresh report using the current list. The usual manual checkboxes
can override a match's group. New manual exclusions are respected when building
the excluded friend's weekly history.

Weekly briefs consider friends even if they played together only once during
the period. They never include a random teammate just because of repeated
matches, or a friend's matches played against the owner.

This implements the owner's group-selection rule. It does not claim to recover
actual Steam party membership. Steam friends import is not implemented.

## Validation

Database and authenticated HTTP tests cover owned settings, same-origin forms,
self-ID rejection, automatic detection submission, same-team membership,
quota exhaustion, duplicate submissions, disabled automation, manual selection
precedence and weekly exclusions. Anonymized settings-page previews can be
generated with `CSDC_PREVIEW_DIR` and `TestFriendsPreview`.
