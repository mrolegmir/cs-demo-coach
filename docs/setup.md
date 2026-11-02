# Setup and configuration

## Local stack

From the repository root, run `docker compose up --build` and open
http://localhost:8080. The default coach uses rules and needs no external services.
PostgreSQL is available locally on port 55432. Data and uploaded demos use named
volumes. Local mode has no authentication; use the production configuration for hosting.

Copy `.env.example` to `.env` for optional worker settings. Compose only passes
the variables explicitly listed in `compose.yaml`; adding unrelated API settings
to `.env` does not enable them in the local API container.

## Optional model

Set `CSDC_LLM=ollama` or `openai` and `CSDC_LLM_MODEL`. Set
`CSDC_LLM_BASE_URL` explicitly for the selected provider: local Compose defaults
to `http://host.docker.internal:11434`, including when selecting `openai`.
For an OpenAI-compatible endpoint, include its API prefix, typically `/v1`, and
set `CSDC_LLM_API_KEY` as required by that service.

For Ollama running on the host, Docker Desktop provides `host.docker.internal`.
Linux installations may need a host-gateway mapping or another reachable address.
The model must already be installed and reachable from the worker.

The host worker also supports `CSDC_LLM=codex`; its adapter requires an installed,
authenticated CLI. The supplied worker image contains only the Go binary. Provider
selection and defaults are defined in [`internal/coach/env.go`](../internal/coach/env.go).

## Hosted API and remote worker

[`deploy/compose.prod.yaml`](../deploy/compose.prod.yaml) runs only API and PostgreSQL.
Create an untracked `deploy/.env` with your own values:

```dotenv
POSTGRES_PASSWORD=replace-with-a-long-random-url-safe-password
CSDC_WORKER_TOKEN=replace-with-a-long-random-shared-secret
CSDC_PORT=8090
CSDC_PUBLIC_URL=https://coach.example.com
CSDC_ADMIN_STEAM_IDS=replace-with-your-steamid64
CSDC_TZ=UTC
```

Use a URL-safe database password because it is interpolated into a connection URL.
Configure an HTTPS reverse proxy to the loopback API port, then run:

```bash
docker compose -f deploy/compose.prod.yaml --env-file deploy/.env up -d --build
```

`CSDC_PUBLIC_URL` enables Steam OpenID sign-in and invite-only access. Admin IDs
come from `CSDC_ADMIN_STEAM_IDS`; other users need invites. The production file
also exposes quotas, brief scheduling, Steam import and Telegram settings.

Build a worker on another machine with `go build -o bin/worker ./cmd/worker`.
Set `CSDC_API_URL` to your HTTPS API URL and `CSDC_WORKER_TOKEN` to the same shared
secret, then run the binary. Set optional coach variables on that machine.
Leave `DATABASE_URL` unset for an HTTP worker. It downloads demos and submits
results through authenticated API endpoints; no inbound worker ports are needed.

An optional direct-database worker uses `DATABASE_URL` for the queue, but still
needs API access for demo files. Configure its `CSDC_QUOTA_UPLOADS` and
`CSDC_QUOTA_COACHING` to match the API (defaults 15, zero means unlimited).

For an existing installation, use the coordinated API/worker upgrade procedure
in [v1.1.1 rollout requirements](v1.1.1-walkthrough.md#rollout-requirements).

## Imports and delivery

- **Replay watcher:** generate an upload token on the site's Auto-upload page.
  Set `CSDC_API_URL` and `CSDC_UPLOAD_TOKEN`, then run `go run ./cmd/csdc watch`
  on the machine with the CS2 replays folder. See [ADR 9](decisions/0009-auto-import.md).
- **Steam history:** configure `CSDC_STEAM_API_KEY` on the authenticated API,
  then link an auth code and newest match share code in the UI. New codes are
  polled every 30 minutes. See [ADR 11](decisions/0011-share-code-polling.md).
- **Demo URL resolution:** install Node.js 24, run `npm ci --ignore-scripts` in
  `bot/`, then `node bot.js login` for a dedicated Steam account. Run
  `node bot.js run` with `CSDC_API_URL` and `CSDC_WORKER_TOKEN` set. Session files
  stay local. See [ADR 12](decisions/0012-share-code-bot.md).
- **Telegram:** set `CSDC_TELEGRAM_TOKEN` on the API, then use Connect Telegram
  in the UI. Ready weekly briefs are delivered to linked chats.
  See [ADR 13](decisions/0013-telegram-briefs.md).

Scheduled briefs run on Fridays at `CSDC_BRIEF_HOUR` (default 15) in `CSDC_TZ`
(default UTC). Scheduled prose defaults to Russian; manual requests use the
selected interface language. Saved reports are not translated retroactively.

The PowerShell launchers in `scripts/` are optional host helpers; they read
untracked `.env.worker`, `.env.watch` and `.env.bot` files respectively. They do
not create scheduled tasks themselves. Never commit those files or bot sessions.
