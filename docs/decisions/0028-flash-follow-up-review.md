# 28. Review flash follow-up with conservative temporal attribution

**Status:** accepted for v1.1.

## Decision

The personal Teamplay category adds flash follow-up review from stored flash,
blinding and death events. It provides both perspectives: teammate kills during
the viewer's flash and the viewer's kills during a teammate's flash. Own kills
after one's own flash remain visible but do not count as teammate support.

A kill's candidate sources are every recorded flash in that round whose
positive blind interval for the victim could cover the kill. Elapsed time is
computed from the stored flash tick and tick rate, and must be strictly less
than the recorded blind duration. The interval endpoint is excluded. Same-tick
flashes remain possible candidates but cannot receive individual attribution.
Enemy flashes and unknown throwers/sides are also candidates: they must not be
silently dropped to make an allied source appear unique.

Individual matching requires exactly one source, a positive tick difference,
a known human thrower other than the victim, an allied flash side, and known
opposite killer/victim sides in the death snapshot. Duplicate/missing killer
or victim states prevent attribution. The thrower need not still be alive.
World kills, suicides, teamkills and deaths outside the live round are excluded.
Repeated victim records within one flash count as one possible source. Repeated
flash records remain separate candidates. Ambiguous kills are counted once
per personal report, even if several of the viewer's flashes were candidates.

This is temporal association, not proof of causality, an official flash-assist
statistic, complete visibility, or a grenade-effectiveness score. Partial blinds
are included without introducing a minimum-duration threshold. No recorded
kill does not imply a wasted flash; no positive duration does not prove complete
capture of all effects. Stored parser linkage and duration limitations remain.

## Presentation and compatibility

Summaries separate outgoing support, incoming support and ambiguous matches.
Per-flash review lists recorded recipients, relations and durations; enemy
targets are distinct positive-duration recipients per flash. Unknown or
contradictory team records remain unclassified. Blind durations are never summed
across overlapping flashes. The area is the throw position, as captured before.
Kill cards show round/time, killer/victim, possible sources, attribution state
and elapsed time only for eligible unique matches. Unknown timing leaves
matching totals unavailable while preserving raw flash review.

The viewer defines the report, independently of the selected coaching party.
Non-participants do not get substituted data. Names remain literal escaped
identities, never translation keys. Existing findings, colors, coach priorities,
drills, history and LLM input are unchanged.

No reparse, capture-version change, migration, dependency update or worker
replacement is required for this feature alone. The API can compute it from
existing parsed matches. Earlier pending capture extensions still need their
own API/worker rollout; this decision does not authorize deployment.

## Evidence

The four committed anonymized demos contain 67/53/49/48 flash bursts and
1/0/0/4 uniquely matched teammate kills respectively (Ancient/Anubis/Inferno/
current Dust2). Incoming and outgoing counts agree across participants. These
are capture checks, not player benchmarks or calibrated coaching thresholds.
