# 18. Release contract: explain priorities and keep weekly evidence consistent

**Status:** accepted (v1.0)

## Context

The core flows in SCOPE are implemented, but a numbered list alone does not
explain why one focus outranks another. The weekly input also included history
after its cutoff, and historical evidence used old player names while the party
used current ones. A changed nickname could make valid evidence fail grounding.

## Decision

- Both coaches use the same final ordering of their selected focus areas:
  active habits before improving habits, then descending excess occurrences
  (`count - lobby rate × chances`), with kind as the stable tie-breaker.
  Each focus carries an application-generated explanation with its rank,
  excess and chances. Excess is a prioritization heuristic, not rounds lost
  or proof of a causal effect on winning. The LLM still chooses up to three
  eligible issues and explains them; it cannot override the displayed ranking.
- The rules coach sorts candidates before selecting them, including historical
  input, and acknowledges at most one resolved issue per player. Weekly briefs
  do not inherit single-match warnings such as “only 0 rounds”.
- If no individual issue stands out, use deterministic advice without an LLM
  call. Pair advice and history acknowledgements still work. Existing request
  quotas and the one-correction limit remain in place.
- Validation rejects null advice, repeated focus kinds, more than two team
  items, whitespace-only required text, and a focus without evidence of that
  player's own mistake of that kind. A related player is not the primary player.
  Workers validate every coach's output before saving it, even when a coach
  reports success.
- Weekly input excludes matches and practice at or after its exclusive cutoff,
  including from long-term history. Party members' evidence uses their latest
  name before that cutoff, consistently across personal and pair evidence.
  The stored match and its original names are unchanged.
- When role advice identifies an entry and a suitable flasher, a third party
  member is assigned to follow and trade. This is a proposed practice plan,
  not a claim about that member's observed role or a map-specific execute.

## Consequences

No schema migration or new external service is needed. Older advice without
`priority` still renders. Saved advice and queued inputs retain their snapshot;
request a new brief or coaching report to see the new behavior.

Validation checks structure, eligibility, drills and citations. It cannot
verify every sentence written by a model; the evidence remains visible beside
the advice. Roles still aggregate both sides, and map-specific tactics and
rank benchmarks remain outside this release.
