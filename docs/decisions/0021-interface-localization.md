# 21. Russian by default with an English interface option

**Status:** accepted.

## Decision

The server-rendered interface defaults to Russian regardless of the browser's
Accept-Language header. A Russian/English selector appears on every page,
including Steam sign-in. POST /language stores the selection in a host-only,
HttpOnly, SameSite=Lax cookie for one year and redirects to a validated local
path. Production HTTPS uses a Secure cookie. Authentication is not required;
the existing origin check applies when authentication is configured.

English source strings are translation keys and the fallback. Russian messages
live in an embedded JSON catalog under internal/web/locales. Templates call tr
for visible text and use format arguments for complete variable sentences.
Each render clones the parsed template and binds language functions to that
request, so concurrent users cannot change each other's language. Responses
set Content-Language, Vary: Cookie and the HTML lang attribute.

Localized content includes navigation, forms, notices, statuses, mistake and
role labels, the curated drill library, and practice summaries. Dates in the
interface use a numeric day/month/year format in both languages. IDs, form
values and finding anchors retain their original contracts. Advice JSON now
includes the computed language metadata for new reports.

## Language of new recommendations

New match coaching and manually requested weekly briefs use the interface
language at submission time. Migration 10 adds a constrained language column
to jobs; single-match workers read this snapshot rather than a browser cookie.
Brief inputs carry language in their existing JSON snapshot, and the planner
explicitly requests Russian for automatic weekly briefs. Retries preserve the
original language. No account language preference is stored.

The rules coach uses a separate embedded Russian message catalog, including
focus titles, reasons, drill application, named team plans, priority
explanations, practice verdicts and resolved-habit notes. The LLM receives the
output language in both its data and system instruction, on the first attempt
and the correction attempt. Generation still uses one call with at most one
validation retry. Empty-focus reports still avoid LLM calls.

The application stamps Advice.Language after generation, overriding any model
value. Telegram brief headings and links use that stored metadata. New CLI
advice defaults to Russian; analyze -lang en selects English.

## Content boundary and rollout

Saved reports are not backfilled or translated. Changing the interface language
changes subsequent requests, not saved prose. Detector evidence descriptions,
user notes, map callouts and external service error details retain their
original text. Identifiers, selection, counts, ranking and evidence validation
are shared across languages; localization does not rerun detection.

Rollout needs migration 10 and both a rebuilt API and a rebuilt home worker.
Existing database rows and historical reports are retained. Deployment remains
an explicit owner decision; the owner approved this rollout after local
verification. The owner also approved including the experimental shooting-observations pilot
in this rollout; its limits remain as documented in ADRs 19 and 20.
