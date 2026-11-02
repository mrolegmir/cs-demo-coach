# Skill categories walkthrough

Open an analyzed match while signed in as one of its participants. The result
and personal K/D/A precede a compact seven-category overview. Expand a category,
then a check, to see counts and episodes split by T/CT. Evidence links open
the collapsed findings table and reveal the selected row.

Amber identifies checks with episodes to review. Green is limited to checked
situations with no occurrences; gray distinguishes unavailable assessment.
Shooting and movement deliberately remain gray. Fresh utility captures add a
separate [HE/fire damage review](grenade-damage-walkthrough.md) inside Utility;
this does not change check statuses or enable new mistake detectors.
The seventh category, [Decisions](decisions-walkthrough.md), offers neutral
lost-advantage review and a captured bomb/post-plant timeline. It has no grade.
Teamplay also offers [flash follow-up review](flash-follow-up-walkthrough.md),
including both thrower and killer perspectives from already saved data.

The action section keeps the personal focus order from the saved coach report.
Its links reveal the corresponding check and available library exercises.
The previous narrative is retained under **Detailed coaching report**.

Choose an exercise and duration, then **Start practice**. The active session
survives reloads and appears in **My progress** as well as analyzed match pages.
Choose actual minutes and **Finish and log practice** to add completed work.
A start alone does not affect practice history calculations. Duplicate starts
do not duplicate sessions; duplicate finishes cannot duplicate completed work.

The scoreboard groups the two starting teams and orders kills descending,
placing the viewer's team first. Side changes have separators in round history.

Anonymized previews can be generated with `CSDC_PREVIEW_DIR` and
`go test ./internal/web -run TestPlayerFirstMatchPage -count=1`.
See ADR 23 for data and assessment limits.

## Validation

Coverage includes participant identity, missing versus measured-zero data,
per-side evidence, localized rendering, owned and idempotent practice lifecycle,
same-origin protection, and active practice on both match and progress pages.
Run the full race suite with `CSDC_TEST_DATABASE_URL` to include database tests.


## Clear check status and position references

Checks use a dedicated yellow palette separate from orange action buttons.
Each check also has a text status. Red outlined **Coach priority N** badges
identify saved personal focuses in their original order; they do not claim
critical error severity or introduce a new threshold.

**Where are these positions?** links to an allowlisted Total CS callout guide
and the pinned parser source for `LastPlaceName`. The game demo supplies the
labels; Total CS is a separate community reference whose version and names
may differ. Unknown/custom maps use the guide index, never a guessed map URL.
Cache's Quad and Forklift have short map-specific descriptions, paraphrased
from https://totalcsgo.com/callouts/cache. Those meanings are not assigned to
other maps. The exact technical source is
https://github.com/markus-wa/demoinfocs-golang/blob/v5.2.0/pkg/demoinfocs/common/player.go#L638.

Flash episodes explicitly label the thrower's area, not the burst or an enemy
position. There is no embedded radar or fabricated point: stored flash
coordinates describe the explosion, and no version-matched radar calibration
is currently provided. No parser, worker, schema or detector changes are
required for this clarification.
