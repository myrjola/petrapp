# Server-rendered MPA with a stack-navigator shim, not an SPA

Petra is a server-rendered Go MPA. App-like navigation (native-feeling back
button, single-roundtrip form submits, view transitions) comes from a small JS
shim built on the Navigation API that intercepts form POSTs and speaks a custom
wire protocol with the server (`X-Requested-With: stacknav` request header;
`200` + `X-Location`, optional `X-Replace-Url`, instead of a 303). We chose
this over an SPA or htmx-style partial swaps because the app must work without
JavaScript (progressive enhancement: plain POST-Redirect-GET 303s remain the
fallback), and because the CSP (`require-trusted-types-for 'script'`) forbids
rendering fetched HTML in place — which rules out partial-swap architectures
and forces the flash + redirect-to-form pattern for validation errors.

## Consequences

- History strategy is pop-or-push: traverse back to an existing entry on a URL
  match, otherwise push; same-URL submits and server-flagged disposable form
  pages (`X-Replace-Url`) replace. The protocol reference lives in
  `cmd/petra/README.md`.
- Every redirect goes through the `redirect`/`redirectReplace` helpers, which
  negotiate the protocol; handlers never hand-roll it.
- The shim is concurrency-subtle; its invariants are model-checked in
  `tlaplus/` and exercised by Playwright tests.
