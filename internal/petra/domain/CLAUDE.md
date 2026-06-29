# Domain — agent notes

Package reference: [README.md](README.md). Vocabulary: [CONTEXT.md](../../../CONTEXT.md).

- Standard library only — no SQL, HTTP, logging, or third-party imports here.
- A value derived from multiple domain fields is a method on the owning domain
  type, never handler logic.
- Multi-field validation returns a `FieldErrors` (collect every failing rule via
  `Add`, `return fe.OrNil()`); keys MUST equal the HTML form field `name`s so
  the web layer attaches each message to its input. A single page-level message
  stays a `ValidationError`.
