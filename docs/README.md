# Documentation

Start with the [project README](../README.md) and [setup guide](setup.md).
[Product scope](../SCOPE.md) explains the audience, priorities and remaining work.

## Current source

- [v1.1.1: reliability](v1.1.1-walkthrough.md) — atomic completion, attempt fencing,
  stable player identity, transactional quotas and API-owned demo cleanup. Includes upgrade requirements.
- [v1.1: expanded match review](v1.1-walkthrough.md) — release overview and capture compatibility.
- [Personal pages](player-first-walkthrough.md), [categories and practice](skill-categories-walkthrough.md),
  [saved friends](friends-walkthrough.md), [localization](localization-walkthrough.md).
- [Aim direction and movement](aim-movement-walkthrough.md),
  [pre-shot movement](pre-shot-movement-walkthrough.md),
  [HE/fire damage](grenade-damage-walkthrough.md),
  [decisions and bomb context](decisions-walkthrough.md),
  [flash follow-up](flash-follow-up-walkthrough.md).
- [Skill catalog](skills-catalog.md) — what is assessed, observed or still unvalidated.

## Design and earlier checkpoints

[Architecture decisions](decisions/) cover the parser model, grounded coaching,
PostgreSQL queue, authentication, imports, practice and later review extensions.
Walkthroughs describe their checkpoint; later documents supersede earlier limitations.
Local preview paths in walkthroughs are regeneration instructions, not bundled artifacts.

| Checkpoint | Contents |
|---|---|
| [v1.0](v1.0-walkthrough.md) | Coaching priorities, grounding and weekly snapshot contract |
| [v0.8](v0.8-walkthrough.md) | Observed roles |
| [v0.7](v0.7-walkthrough.md) | Trade pairs |
| [v0.6](v0.6-walkthrough.md) | Game-supplied place names |
| [v0.5](v0.5-walkthrough.md) | Curated drills |
| [v0.4](v0.4-walkthrough.md) | Telegram briefs and trends |
| [v0.3](v0.3-walkthrough.md) | Replay watcher, practice and Steam imports |
| [v0.2](v0.2-walkthrough.md) | Accounts, issue history, weekly briefs and remote workers |
| [v0.1](v0.1-walkthrough.md) | Initial parse/detect/coach pipeline |
