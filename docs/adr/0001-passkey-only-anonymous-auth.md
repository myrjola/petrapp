# Passkey-only anonymous authentication

Petra authenticates exclusively with WebAuthn passkeys — no email, no password,
no usernames, no recovery flow. Registration creates an anonymous account bound
to a credential; the app never learns who the user is. This was chosen for the
privacy-focused product stance (no PII to leak, account deletion is genuinely
complete) and because passkeys remove the entire password-management surface
(storage, reset, breach handling) from a solo-maintained app.

## Consequences

- Losing every passkey means losing the account. There is deliberately no
  recovery channel — acceptable for workout data, by design.
- The WebAuthn RP ID derives from `PETRAPP_FQDN`; changing the domain
  invalidates all existing passkeys (see `docs/disaster-recovery.md`).
- Shared auth lives in `internal/platform/auth` so other apps in the module
  inherit the same model.
