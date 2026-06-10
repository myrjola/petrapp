# Web layer — agent notes

Architecture, error handling, stack-navigator protocol, and testing patterns:
[README.md](README.md). UI work: [ui/templates/README.md](ui/templates/README.md)
and [ui/design-system.md](ui/design-system.md).

- Every redirect goes through `redirect()`/`redirectReplace()` — never
  `http.Redirect` directly; they negotiate the JS-shim wire protocol.
- `userError`'s `safeURL` must point at a GET handler that pops + renders the
  flash banner — never `r.Referer()`, never a POST-only action endpoint
  (redirect loop). See "Error Handling" in the README before touching a POST
  handler.
- User-facing strings use the UI register, never domain terms — the label map
  lives in [CONTEXT.md](../../CONTEXT.md) under "UI register".
