# 30. Atomic worker completion, admission and stable player identity

**Status:** accepted (v1.1.1)

## Context

Review reproduced five failures: detection could be saved and then hidden after
its deleted demo was retried; an old job attempt could overwrite a later one;
equal nicknames confused coaching validation; concurrent requests exceeded the
weekly limit; direct workers skipped the HTTP handler's automatic coaching.

## Decision

- Worker protocol v2 identifies a claim by job ID and attempt number. The store
  locks the job, checks that generation and its running state, and derives the
  target from the database. Workers cannot independently save a result, delete
  the source and acknowledge the job. `FinishDetection` and `FinishAdvice` save
  their result and mark the job done in one transaction. A duplicate completion
  for the same finished attempt succeeds without writing anything, including
  after a subsequent manual coaching request. Stale failures/retries are rejected.
- Detection completion also invokes automatic coaching in that transaction.
  Both worker adapters call this operation. Worker wire types live in `jobwire`;
  the worker no longer imports the web package.
- The API retries demo cleanup after a detect job is durably done. Migration 13
  adds a durable `jobs.demo_deleted` marker. Removing an already absent file is
  successful; a crash between removal and acknowledgement is harmless. Cleanup
  starts with the API, independently of Steam imports or authentication.
- Legacy worker writes and DELETE-demo requests return 409 with an explicit
  upgrade message. Silently accepting unfenced writes would invalidate v2.
  Read endpoints and the share-code bot protocol remain compatible. Upgrade
  requires stopping/draining old workers before switching the API and restarting
  a v2 worker. Mixed-version processing is intentionally rejected.
- Store admission locks the requesting user before the match, checks usage and
  enqueues in the same transaction. Upload deduplication uses the same owner
  lock. Manual coach, manual brief and automatic coach share that lock. Scheduled
  briefs remain uncharged; admins are exempt, including automatic coaching.
  Limits are immutable constructor arguments. API and direct workers use the
  same environment configuration/defaults; a direct worker must be configured
  with the API's quota values. UI quota displays are advisory.
- New coaching inputs preserve stable IDs in `player_ids`, `Evidence.PlayerID`
  and `RelatedIDs`. Human-readable model references are disambiguated against
  the entire roster (including names that resemble generated suffixes).
  Validation resolves these references to IDs. The application stamps
  `PlayerAdvice.PlayerID` after validation, for every coach implementation.
  Names are still available for prose; raw IDs need not be sent to the model.
- Saved reports are not rewritten. New match advice is attributed by ID.
  Legacy advice is attributed only if its name identifies one participant in the
  match and that participant belongs to the selected party. Ambiguous legacy
  input is rejected before coaching, without a panic or an LLM call.

## Consequences

Postgres remains the only queue and durable state. There is no new broker or
generic service framework. Existing standalone store save methods remain for
fixtures and maintenance; worker adapters expose only fenced completion.

Tests cover lost completion responses, stale generations, transaction rollback,
duplicate completions after a new request, concurrent uploads and mixed
coach/brief admission, equal nicknames, adapter parity, legacy write rejection
and cleanup retries. The parser itself still has no context cancellation; the
generation check prevents an overdue parser from committing over its successor.
