---
name: verifier-webui
description: |
  Evidence-capture protocol for verifying petra web-UI changes by driving the real app in a
  browser — page rendering, navigation, view-transition/CSS animations, or anything behind the
  passkey login. Use when verifying a UI change (the `verify` skill auto-discovers verifier-*
  skills), or directly when asked to confirm a front-end change works in the running app. Covers
  the headless Playwright + virtual-WebAuthn recipe, its limits (no cross-document view
  transitions headlessly), the WebKit technique for iOS-Safari bugs, and on-device verification.
argument-hint: "[what changed, e.g. 'set-card slide animation' or 'exercise page layout']"
allowed-tools:
  - Bash(go build*)
  - Bash(make dev*)
  - Bash(./bin/petrapp*)
  - Bash(npm*)
  - Bash(npx*)
  - Bash(node*)
  - Bash(tailscale*)
  - Read
---

# Verifier: petra web UI

Verification is runtime observation: build, run, drive the real app to where the changed code
executes, capture what you see. The login gate is **WebAuthn (passkeys)**, so the trick is a CDP
**virtual authenticator** — no mocks, the genuine flow.

## What this gets you — and what it can't

- ✅ Headless Chromium drives the full authenticated flow and proves **functional** correctness
  (state advances, DB writes) and **console cleanliness**.
- ❌ `chromium-headless-shell` (Playwright's default) does **not** run *cross-document* view
  transitions (no compositor) and **hangs screenshotting live petra pages** (continuous repaint).
  You cannot capture the animation frames headlessly. The same-document VT API *does* work
  (`document.startViewTransition`, `vt.types.add`, `:active-view-transition-type()`).
- 👉 For the actual motion, confirm **on-device** (`make dev-tailnet`). For iOS-Safari rendering
  bugs, reproduce in Playwright **WebKit** — but desktop WebKit ≠ iOS-Safari-mobile for some
  quirks (see below), so the device is the final word.

## 1. Setup (once per machine)

```bash
mkdir -p /tmp/pw && cd /tmp/pw && npm init -y && npm i playwright
npx playwright install chromium    # add `webkit` too for the iOS-engine repro
```

## 2. Run the app

```bash
cd <repo> && go build -o bin/petrapp ./cmd/petra
PETRAPP_ADDR=localhost:8081 ./bin/petrapp   # run in background
```

Templates and static assets **hot-reload from disk** — no rebuild while iterating on UI.

## 3. Drive it (Playwright + virtual passkey)

Register a virtual authenticator over CDP, then walk the real flow:

```js
const client = await context.newCDPSession(page);
await client.send('WebAuthn.enable');
await client.send('WebAuthn.addVirtualAuthenticator', { options: {
  protocol: 'ctap2', transport: 'internal',
  hasResidentKey: true, hasUserVerification: true,
  isUserVerified: true, automaticPresenceSimulation: true,
}});
```

Canonical flow (each step is a real navigation):

1. `/` → click `button.cta__primary` — registers the passkey.
2. `/preferences` → `selectOption('#<weekday>_minutes_select', '60')` → submit
   `form[action="/preferences/schedule"]`. Weekday is **today's**, lowercased (e.g.
   `#thursday_minutes_select`).
3. `/workouts/<YYYY-MM-DD>` → submit `form[action$="/start"]`.
4. Open `a.exercise` → the exercise page.
5. Warmup: submit `form[action*="warmup"]` (warmup → set 1 is itself a set-advance).
6. Complete a set: `button[name="signal"][value="on_target"]` (weighted/timed) or
   `button[type="submit"]` (bodyweight/deload).

**Gotchas that cost time:**

- The **stack-navigator** intercepts form submits and does a same-URL `location.replace`
  (cross-document) → use `click({ force: true })` and wait on a **selector**
  (`.exercise-set.active`), not `networkidle`. Entering from the overview / `?edit=` are
  forward/backward page slides instead.
- Post-register and post-submit **redirects race** your next `page.goto` (`ERR_ABORTED`). Wrap
  `goto` in a small retry and `sleep(~600ms)` after registering.
- `addInitScript` globals and `window.*` **reset on every document**, so they're empty after the
  replace nav. Persist cross-navigation observations in **`sessionStorage`**.
- Don't inject inline `<style>`/`style=` to tweak the page — the **CSP nonce blocks them** and
  pollutes `console` errors. Throttle animations via CDP `Animation.setPlaybackRate` instead.

## 4. Assert

- Functional: focus card advances (`.set-index` text `Set 1` → `Set 2`); query the DB if needed.
- `page.on('console')` / `page.on('pageerror')` stay empty.
- View-transition wiring: completing a set is a `replace` nav; the page-local handler adds the
  `set-advance` type and sets `body[data-vt="set-advance"]`. Headlessly you can confirm the API
  and the data-vt flag logic, but not the rendered slide.

## 5. iOS-Safari rendering bugs (Playwright WebKit)

Desktop WebKit is Safari's engine. To inspect the authed page without doing WebAuthn in WebKit:

1. In Chromium (already authed): `const html = await page.content()`.
2. Inject `<base href="http://localhost:8081/">` into `<head>` so `/static/main.css` resolves.
3. In webkit: `await page.setContent(html)`, then measure / screenshot.

Static-HTML renders screenshot fine with `{ animations: 'disabled' }` (the live-page hang doesn't
apply). **Caveat:** some bugs are iOS-Safari-*mobile*-only and won't reproduce in desktop WebKit —
e.g. the `display:flex` `<fieldset>` phantom-padding bug. When desktop+WebKit both look clean but
the user sees a bug, suspect a mobile-Safari quirk and verify on-device.

## 6. On-device (real iOS Safari)

```bash
make dev-tailnet   # → https://<tailnet-fqdn>:8443 — HTTPS, valid cert, WebAuthn works on iOS
```

Open the URL on the iPhone. The dev DB persists, so a passkey registered once stays registered.
This is the only way to confirm cross-document view-transition motion and mobile-Safari rendering.

## Report

Follow the `verify` skill's report shape. Be explicit about what you could and couldn't observe:
"functional flow + console verified headlessly; animation motion confirmed on-device" is an honest
PASS — claiming you saw a headless animation play is not.
