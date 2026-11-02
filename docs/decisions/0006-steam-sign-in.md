# 6. Steam sign-in, invite-only, with weekly quotas

**Status:** accepted (v0.2). Moves "Steam sign-in" up from SCOPE's "Later".

## Context

v0.2 makes the service usable by a party of friends on a public URL, so it
needs accounts, access control and quotas (LLM cost). Players are identified
in demos by their SteamID64.

## Decision

- **Steam OpenID 2.0** (`internal/auth`). A login yields the SteamID64 that
  is already in every demo, so an account links to its matches and findings
  without a mapping step. No Steam Web API key and no stored secrets are
  needed: the signature is checked by asking Steam (`check_authentication`).
  We also check `return_to`, the provider endpoint, and the claimed-ID format.
  Every `openid.*` parameter must appear exactly once, so the ID we check
  cannot differ from the one Steam verifies. Each login carries a one-time
  state, both in `return_to` and in a short-lived cookie, and the assertion's
  nonce must be under 5 minutes old. Together these mean a callback URL from
  a log or screenshot cannot be replayed, and nobody can be signed into
  someone else's account.
- **Invite-only.** Admins come from `CSDC_ADMIN_STEAM_IDS`, and every user can
  create one-time invite links (7 days). An invite is redeemed during the
  Steam round trip through a short-lived cookie.
- **Sessions** are random 256-bit tokens in an HttpOnly, Secure, SameSite=Lax
  cookie. Only their SHA-256 is stored, so a database leak does not leak
  sessions. State-changing requests must also come from the site's own
  origin.
- **Visibility:** a user sees matches they uploaded or played in; admins see
  all. Other match IDs return 404, so they reveal nothing. Only the uploader
  (or an admin) may coach a match, so an opponent who appears in it cannot
  replace the uploader's party and advice.
- **Quotas:** uploads and coaching requests per user per rolling 7 days,
  counted from `matches.uploaded_by` and `jobs.requested_by`, so there are no
  counters to drift. Admins are exempt.
- Without `CSDC_PUBLIC_URL` the server runs open, for local development and
  `docker compose up`.

## Consequences

- Friends who play together see each other's matches automatically, which
  sets up the v0.2 party view and issue history.
- The display name comes from the latest analyzed match the player appears
  in. A new user shows as their SteamID until then; this avoids depending on
  the Steam Web API.
- Integration tests run the whole flow against a real database and a fake
  Steam: invite, reuse, logout, cross-site POST, visibility and quota.
