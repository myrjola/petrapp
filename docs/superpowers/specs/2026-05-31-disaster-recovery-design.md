# Disaster Recovery Documentation & Procedures — Design

Date: 2026-05-31

## Problem

PetraApp's backup *infrastructure* is solid — single Fly Machine, SQLite on a
volume, Litestream continuous replication to a Tigris/S3 bucket, and CI
(`make migratetest`) that restores the latest **production** backup on every
push to `main`, so DB restore-ability is exercised daily.

The *recovery story*, however, has two gaps:

1. **No proven full-recovery drill.** We know the DB restores. We have never
   verified the full path of reconstituting the app from nothing — the case
   where someone deletes the Fly app and the volume plus on-machine snapshots
   are gone, leaving only the Litestream S3 replica.
2. **Recovery procedures are scattered and incomplete.** Steps live across the
   README ("Recovering database", "Creating new deployment"), Makefile
   comments, and the `fly-ops` skill. The README itself admits "The process
   could still use some improvements." Whole failure scenarios (bad migration,
   accidental data deletion, lost bucket, lost secrets, region outage) are
   undocumented.

The key realization: the Litestream backup restores **data only**. Everything
else needed to rebuild the app lives elsewhere or nowhere.

## Recovery surface (what the backup does NOT cover)

| Artifact | Source | Recoverable if the app is deleted? |
|---|---|---|
| Data (users, workouts, sessions) | Litestream S3 replica | ✅ Well-covered; CI proves restore daily |
| App / infra config (`fly.toml`, `Dockerfile`, `litestream.yml`) | git | ✅ Fine |
| **VAPID keypair** (`PETRAPP_VAPID_PUBLIC` / `PETRAPP_VAPID_PRIVATE`) | `fly secrets` | ⚠️ Required in prod (`cmd/web/main.go`). Regenerating works but **breaks every existing push subscription**. Unrecoverable unless stashed outside Fly. |
| **Tigris/S3 credentials & bucket** (`AWS_ACCESS_KEY_ID/SECRET`, `BUCKET_NAME`, `AWS_ENDPOINT_URL_S3`, `AWS_REGION`, `LITESTREAM_REPLICA_PATH`) | set by `fly storage create` | 🔴 **Critical.** If the bucket and its creds are gone, the backup itself is gone = total data loss. Must confirm the bucket survives app deletion and the creds are retrievable. |
| WebAuthn RPID | derived from `PETRAPP_FQDN` | ✅ Fine *while the domain stays `petra.fly.dev`*; a domain change breaks all passkeys. |

The DR question worth answering is therefore not "can we restore the DB" but
**"are the VAPID secrets and the Tigris bucket survivable, and does the bucket
outlive the app?"**

## Deliverable

A single durable runbook at **`docs/disaster-recovery.md`** (top-level in
`docs/`, alongside the other ops write-ups; linked from the README Operations
section). Two parts.

### Part A — Failure scenario catalog

A table covering the scenarios currently undocumented. Each row carries:

- **Detection** — how you notice it happened.
- **Recovery procedure** — copy-paste `make` / `fly` commands.
- **Data loss / RPO** — worst-case data lost given Litestream's 5m replica
  interval and snapshot retention (24h interval, 168h retention).
- **Drill status** — drilled / desk-checked / untested.

Scenarios:

1. App deleted or destroyed (flagship — full rebuild from S3).
2. Volume lost or corrupted, app otherwise intact.
3. Corrupted or broken DB file (bad `.backup`, disk full, WAL corruption).
4. Bad migration shipped to prod.
5. Accidental data deletion (one user / one table).
6. Tigris bucket lost, or credentials rotated / lost.
7. VAPID secrets lost.
8. Region (`arn`) outage.

### Part B — "App deleted" recovery inventory + non-destructive walkthrough

The deep-dive on the flagship scenario. The recoverability table above, but
**validated for real** by a read-only walkthrough against `petra-staging`:

- Confirm the Tigris bucket exists as an independent resource (`fly storage
  list`) and survives independently of the app.
- List the Litestream generations / snapshots actually present in staging's
  bucket (restore-to-temp-path dry run, or `litestream snapshots`).
- Enumerate which secrets are set (`fly secrets list` — shows names + digests,
  not values) and cross-check against the required set.
- Confirm a restore to a throwaway path on the running staging machine
  succeeds.

The walkthrough output becomes documented findings plus a **prioritized gap
list** — e.g. "stash the VAPID keypair in a password manager", "confirm Tigris
bucket deletion protection / independent lifecycle", "record bucket name +
endpoint somewhere outside Fly".

The full rebuild path (app create → storage/secrets → deploy → restore →
verify) is documented as a procedure, with the **destructive rebuild drill
recorded as the next drill to run** — not executed in this pass.

## Out of scope (YAGNI)

- Destructive drill (actually destroying + rebuilding staging or prod).
- New automation / `make` targets for DR.
- Formal RTO/RPO targets and freshness/restorability monitoring or alerting.

These surfaced during brainstorming and were deliberately deprioritized. The
catalog will state data-loss bounds per scenario, but we are not committing to
formal objectives or building alerting now.

## Success criteria

- `docs/disaster-recovery.md` exists, linked from the README, and is the single
  authoritative recovery reference.
- Part A catalogs all eight scenarios with detection, procedure, data-loss
  bound, and drill status.
- Part B contains **real findings** from the staging walkthrough (not
  placeholders) and a prioritized gap list with the most important being the
  survivability of the VAPID secrets and the Tigris bucket.
- No destructive action taken; staging remains healthy.
