# 5. Server-rendered UI with html/template and no JavaScript

**Status:** accepted (v0.1). Resolves open question 4 in SCOPE.md.

## Context

The UI has three screens: upload, party picker and report. People read the
pages far more than they interact with them, and the weekly cadence means
nothing needs to update live.

## Decision

- `html/template` pages are embedded in the binary, with one stylesheet in the
  layout and light/dark mode via `prefers-color-scheme`.
- Pending pages refresh themselves with `<meta http-equiv="refresh">`.
- Evidence chips are anchor links to rows of the findings table.

## Alternatives

- **templ + htmx:** nicer components, but it adds a code generator to the build
  for no feature we need yet. It is easy to adopt later, because the handlers
  already build view models (`internal/web/view.go`).
- **SPA:** a second toolchain and an API contract to maintain, for three
  screens.
