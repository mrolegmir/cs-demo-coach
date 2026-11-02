# 4. Postgres as the only state, pull-based workers

**Status:** accepted (v0.1), worker transport changed in v0.2

## Context

Running costs must stay minimal. Parsing is CPU-heavy and may run on a home
machine that cannot accept inbound connections.

## Decision

- Postgres holds everything, including the job queue. Workers claim jobs with
  `UPDATE … WHERE id = (SELECT … FOR UPDATE SKIP LOCKED LIMIT 1)`, so no message
  broker is needed.
- Workers *pull*. Since v0.2 a worker talks only to the API, over HTTPS with a
  shared token. `/internal/jobs/claim` and its siblings wrap the same store
  calls. The worker downloads the demo from `/internal/demos/{id}` and deletes
  it after parsing. Postgres is never exposed to the internet: a worker on a
  home PC needs nothing but an outbound HTTPS connection. A worker on the same
  host can still use `DATABASE_URL` and talk to Postgres directly; both paths
  implement one `worker.Queue` interface.
- Failed jobs are retried with quadratic backoff, up to 3 attempts. Errors that
  retrying cannot fix fail the match immediately with a readable reason: a
  corrupt demo, or advice that keeps failing validation. Jobs left `running`
  past a timeout (a crashed worker) are taken over by another worker.
- Findings are also stored as rows: a `findings` table indexed by player and
  kind. That makes the v0.2 issue history a query rather than a migration of
  JSON blobs.
- Uploads stream to disk without buffering, are checked for the CS2 demo magic
  bytes, and are capped at 1 GB.

## Consequences

- One `docker compose up` runs everything. The compose worker uses the API
  path, so the setup a home worker uses is the one exercised locally.
- An integration test runs the worker against a real API server and a real
  Postgres: detect, coach, a corrupt demo, and a wrong token.
- Tests run against a real Postgres: a throwaway database per test, set by
  `CSDC_TEST_DATABASE_URL`, and a Postgres service in CI. That includes a test
  that five concurrent workers never claim the same job.
- The API keeps uploaded demos on local disk until a worker has parsed them, so
  for now there is a single API instance. That is fine for an invite-only
  product.
