# Gameplay skill coverage

This catalog is a research backlog, not a list of mistakes attributed to a player.
The player does not need to supply terminology. New candidates come from coaching
sources and recurring match episodes, then pass the validation process below.

| Area | Candidate observations | Current coverage / prerequisites | Practice direction after validation |
|---|---|---|---|
| First-shot preparation | Recent movement at firing, delay after stopping | Pilot: recent horizontal displacement at shots; no stop timing or exact velocity | Stop-and-shoot practice |
| Firing discipline | Single, short and long firing sequences by weapon | Pilot: rifle sequences with gaps ≤250 ms, uniquely linked enemy damage and first damage ordinal; intended targets remain unknown | Deliberate short bursts |
| Recoil control | Hit distribution through sustained fire, aim compensation | Damage/shot evidence captured; diagnosis still needs recoil, intended target and visibility context | Weapon-specific recoil practice |
| Crosshair placement / pre-aim | Angular correction before first visible contact, head-height alignment | Capture 4 samples view-to-damaged-eye geometry; visibility, head hitboxes and diagnosis remain unassessed | Route clearing and pre-aim |
| Flicks / micro-corrections | Correction time, overshoot, undershoot | Not assessed; needs contact windows and view-angle sampling | Target acquisition |
| Target switching | Acquisition between distinct targets | Not assessed; needs target identity and visibility | Multi-target practice |
| Movement / counter-strafe | Deceleration before firing, ground/air state | Personal pre-shot/per-shot speed and airborne review; no input technique or exact stop-time diagnosis | Stop-and-shoot practice |
| Peeking / exposure | Number of exposed angles, duration exposed, repeated peek | Not assessed; needs geometry, visibility and duel episodes | Angle isolation and cover |
| Positioning / survival | Isolation, escape options, cover, objective timing | Partial: isolation and dry opening deaths; distance ignores walls | Supported entries and spacing |
| Trading / coordination | Trade opportunities, timing, partner patterns | Selected rules and temporal flash follow-up review; no causal/official-assist claim; geometry limits remain | Entry/support/trade rehearsal |
| Utility | Blind effects, unused utility, usage opportunities | Selected rules implemented; HE/fire health damage review captured separately; no smoke/area-denial value model | Utility timing and support |
| Economy | Buy mismatch and missed drops | Selected rules implemented | Shared buy calls |
| Information / decisions | Rotations, advantage management, objective decisions | Lost-advantage replay context and bomb/post-plant timeline; no quality score; visibility, player information and comms unknown | Scenario review |

## Promoting a candidate into coaching

1. State an observable hypothesis, eligible opportunities and exceptions. A named
   skill is not itself a measurable detector.
2. Verify field availability on supported demo formats. Missing is distinct from
   measured zero. Keep evidence at match/round/tick granularity and version capture.
3. Inspect positive and negative episodes, including prefires, smoke spam, airborne
   shots, weapon differences and non-combat shooting. Use anonymized fixtures.
4. Calibrate thresholds on held-out matches and comparable situations. A lobby
   comparison alone cannot validate a mechanic or establish causation.
5. Add a reproducible detector, meaningful regression tests, calibrated confidence,
   history semantics and a matching curated drill before enabling a coaching claim.

An exploratory model may propose patterns from captured observations. Such output
is a hypothesis for review, never an automatic new production diagnosis. No pattern
discovery job or rank benchmark is implemented in this pilot.

## Data retention

New parsed matches retain firearm shots: player, tick, weapon, position, view angles,
airborne state, place, and optional recent horizontal speed with its sample interval.
This supports sequence and movement observations, not general replay reconstruction.
Version 2 also retains firearm damage events and conservative shot links, including
victim, event-time distance and hitgroup. `csdc mechanics <demo.dem|match.json.gz>`
exports anonymized review evidence and exploratory buckets. Unmatched damage is
retained and damage availability is explicit. See [ADR 20](decisions/0020-shot-damage-evidence.md).
Existing parsed matches have no capture version and remain explicitly unavailable.
Re-coaching cannot recover missing fields. Demos are still deleted after parsing;
expanding into visibility or pre-contact aim analysis needs a new capture design
and newly parsed demos. Match access rules also protect the stored shot observations.

Capture version 3 adds a rolling half-second of frame-averaged speed and airborne
state before each shot. Rifle sequences expose their first-shot history in the
match page and CLI export. This supports movement review, without a stop timing,
counter-strafe or aim diagnosis. See the [movement walkthrough](pre-shot-movement-walkthrough.md)
and [ADR 25](decisions/0025-pre-shot-movement-history.md).

Capture 4 adds synchronized pre-damage view/eye samples and retains full pre-shot
preparation only for the first shot of each rifle sequence. Personal Shooting
and Movement cards expose the recorded evidence; see [the walkthrough](aim-movement-walkthrough.md).

Independent utility capture version 1 adds HE/fire health-damage events, personal
enemy/team/self summaries and round/time evidence. It does not link damage to
throws or score grenade effectiveness. See the [grenade damage walkthrough](grenade-damage-walkthrough.md).
