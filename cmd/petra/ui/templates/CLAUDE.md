# Templates & CSS — agent notes

Reference: [README.md](README.md). **Before any visual or CSS work, read
[`../design-system.md`](../design-system.md)** — design language, tokens,
colour/motion conventions.

- Never use inline `style="..."` attributes — the CSP nonce makes the browser
  silently drop them. Every `<style>`/`<script>` tag carries `{{ $.Nonce }}`
  (`{{ .Nonce }}` in components).
- No web fonts, no `@font-face`; no `:hover`/`cursor: pointer` — mobile-only,
  style `:active`/`:focus-visible`.
- Trusted Types is enforced: build DOM with nodes/`textContent`, never HTML
  strings (see README "JavaScript & CSP").
- Copy targets the average gym user — domain terms never appear verbatim; use
  the "UI register" label map in [CONTEXT.md](../../../../CONTEXT.md).
