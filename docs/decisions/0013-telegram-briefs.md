# 13. Briefs in Telegram: long polling from the api, one-time link codes

**Status:** accepted (v0.4)

## Context

The weekly brief (ADR 8) is only a web page. Players have to remember to open
it on Friday, and most do not. The party already talks in Telegram, so a
message there when the brief is ready is the shortest path from "the coach
has something to say" to "the player reads it". Email was the other option;
it needs a sending domain and lands in spam, for a handful of users who all
have Telegram.

## Decision

- **One Telegram bot, run by the api.** The token comes from `@BotFather`
  (`CSDC_TELEGRAM_TOKEN`). Without it nothing changes.
- **Long polling, not a webhook.** The api calls `getUpdates` itself, so the
  server needs no new public route, no TLS setup for Telegram and no secret
  path to guard. At a few messages a day, one open request is free.
- **Linking with a one-time code.** "Connect Telegram" on the home page makes
  a random code, stores only its SHA-256 (like upload tokens), and opens
  `t.me/<bot>?start=<code>`. Pressing Start sends `/start <code>` to the bot,
  which binds that chat to the account and spends the code. Codes work for
  15 minutes. The chat ID never has to be typed, and a code seen by someone
  else cannot be reused. A chat belongs to one account; linking it again
  moves it. `/stop` in the chat, or the button on the site, unlinks it.
- **Private chats only.** A brief describes each player's habits; posting it
  into a group chat is a sharing decision the bot should not make for them.
- **What is sent:** the period, each player's focus titles and the team's,
  and a link to the brief. The why, the evidence and the drills stay on the
  site, behind sign-in. Plain text, no formatting, so LLM output cannot
  break the message or inject markup.
- **Delivery is a sweep.** Every minute the api sends ready briefs that were
  not sent yet. A brief is marked sent after Telegram accepts it, so a
  passing failure (network, flood control) is retried on the next sweep; a
  refusal that cannot change (another 4xx) is logged and marked, so it is
  not retried every minute. Ages count from when the coach finished the
  brief (`ready_at`), not when it was queued, because the home worker may be
  off for a weekend. Briefs that became ready more than two days ago are not
  sent, so an api that was down does not deliver last week's brief, and
  briefs that became ready before the chat was linked are skipped, so
  linking does not replay history. A 403, or "chat not found", means the
  chat is gone; it is unlinked.
- **The site does not wait for Telegram.** The bot asks `getMe` for its
  username (the deep link needs it), retrying with backoff up to ten
  minutes apart; the Connect button appears once it has answered. A
  Telegram outage at deploy time delays delivery instead of switching it
  off until the next restart.

## Consequences

- Briefs reach players without anyone opening the site, and the planner
  (ADR 8) is unchanged: delivery only reads ready briefs.
- Only the brief's owner gets the message. Teammates without an account are
  in the brief but get nothing; inviting them is the way in.
- At-least-once: if the api dies between Telegram accepting a message and
  the mark, the brief is sent again on restart. A duplicate notification is
  cheaper than a lost one.
- A second api instance would poll too and Telegram would reject one of
  them (409). There is one instance; a lock would be needed before a second.
