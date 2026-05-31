# Disaster Recovery Runbook

Last validated: **2026-05-31** against `petra-staging` (non-destructive walkthrough).

PetraApp is a single Fly Machine (scale-to-zero) with SQLite on a volume,
continuously replicated by Litestream to a Tigris/S3 bucket. CI
(`make migratetest`) restores the latest **production** backup on every push to
`main`, so DB restore-ability is exercised daily.

> **The backup restores DATA ONLY.** Reconstituting the app after a
> catastrophic loss also needs the app secrets and the bucket itself — neither
> of which lives in the Litestream replica or in git. See the inventory below.

## Recovery inventory — what it takes to rebuild from nothing

Validated against `petra-staging` on 2026-05-31 (production mirrors this setup;
its bucket is `petra-backup`).

| Artifact | Source | Recoverable if app deleted? | Status (staging, 2026-05-31) |
|---|---|---|---|
| Data (users, workouts, sessions) | Litestream S3 replica | Yes | Restore from S3 alone → `PRAGMA integrity_check` = `ok`, 1972 users. Replica `status: ok`, newest LTX `2026-05-31T17:39Z`. |
| App / infra config | git (`fly.toml`, `Dockerfile`, `litestream.yml`) | Yes | In repo. |
| **VAPID keypair** (`PETRAPP_VAPID_PUBLIC` / `PETRAPP_VAPID_PRIVATE` / `PETRAPP_VAPID_SUBJECT`) | `fly secrets` | ⚠️ Only if stashed outside Fly. Regenerating works but **breaks every existing push subscription** (clients must re-subscribe). Required in prod (`cmd/web/main.go`). | All three present as secrets. **Not stashed anywhere outside Fly.** |
| **`OPENAI_API_KEY`** | `fly secrets` | ⚠️ Unrecoverable from backup/git; re-mint from the OpenAI dashboard. | Present as a secret. **Not stashed outside Fly.** |
| **Tigris bucket + credentials** (`AWS_ACCESS_KEY_ID/SECRET`, `BUCKET_NAME`, `AWS_ENDPOINT_URL_S3`, `AWS_REGION`) | `fly storage create` | 🔴 **Critical.** If the bucket and its creds are both gone, the backup itself is gone = total data loss. | Bucket `petra-staging-backup` exists as an **org-level** Tigris resource (`fly storage list`), independent of the app — so it survives app deletion. All five creds present as secrets. **Open question:** are the access keys tied to the app's storage extension (and thus lost on app delete) or independent of it? See gap list. |
| WebAuthn RPID | derived from `PETRAPP_FQDN` | Yes, while the domain stays `petra.fly.dev` | A domain change invalidates all existing passkeys. |

The DR question worth answering is therefore not "can we restore the DB" (CI
proves that daily) but **"are the VAPID/OpenAI secrets and the Tigris bucket
survivable, and does the bucket outlive the app?"** The bucket does (org-level
resource); the secrets do **not** unless stashed offline.

## Full rebuild procedure ("app deleted")

Prerequisite: `fly auth whoami` works, and you have the **VAPID keypair**,
**`OPENAI_API_KEY`**, and **Tigris credentials** from your offline stash (see
Gap list — today these exist only inside Fly).

```sh
# 1. Recreate the app.
fly apps create petra

# 2. Re-attach the existing backup bucket. The bucket (petra-backup) is an
#    org-level resource and survives app deletion, but the app needs fresh
#    AWS_* credentials for it. Either re-mint keys for the existing bucket from
#    the Tigris dashboard, or `fly storage create` a new extension and point it
#    at the existing bucket. Confirm BUCKET_NAME / AWS_ENDPOINT_URL_S3 match.
fly storage list                       # find the surviving bucket
# ... obtain AWS_ACCESS_KEY_ID / AWS_SECRET_ACCESS_KEY for it ...

# 3. Restore all non-data secrets from the offline stash.
fly secrets set --app petra \
  PETRAPP_VAPID_PUBLIC=…  PETRAPP_VAPID_PRIVATE=…  PETRAPP_VAPID_SUBJECT=… \
  OPENAI_API_KEY=… \
  AWS_ACCESS_KEY_ID=…  AWS_SECRET_ACCESS_KEY=…  BUCKET_NAME=…  \
  AWS_ENDPOINT_URL_S3=…  AWS_REGION=…

# 4. Deploy. The entrypoint is `litestream replicate -exec ./petrapp`, which
#    restores /data/petrapp.sqlite3 from the replica on first boot if absent.
fly deploy --app petra

# 5. Verify.
make fly-sqlite3 FLY_APP=petra
#   sqlite> PRAGMA integrity_check;        -- expect: ok
#   sqlite> SELECT count(*) FROM users;    -- expect: a sane, non-zero count
```

If the volume already contains a DB but you need to force a restore from the
replica, restore to a side path and swap it in:

```sh
fly ssh console --app petra --user petrapp \
  -C "/dist/litestream restore -o /data/petrapp.sqlite3.new /data/petrapp.sqlite3"
# stop the app, mv the file into place, restart.
```

## Gap list (prioritized)

1. **🔴 Confirm the Tigris access keys survive app deletion.** The bucket is an
   org-level resource and survives, but `AWS_ACCESS_KEY_ID/SECRET` were minted
   by the app's storage extension. Verify in the Tigris dashboard that you can
   mint **new** keys for an existing bucket (so a deleted app doesn't lock you
   out of an intact backup). This is the single most important unknown — losing
   the keys while keeping the bucket is still a lockout.
2. **⚠️ Stash the unrecoverable secrets offline.** Put the VAPID keypair
   (`PETRAPP_VAPID_PUBLIC/PRIVATE/SUBJECT`), `OPENAI_API_KEY`, and the Tigris
   creds in a password manager. They exist only inside Fly today; if the app is
   deleted they are gone. Regenerating VAPID breaks all push subscriptions.
3. **Record bucket name + endpoint outside Fly.** `BUCKET_NAME` and
   `AWS_ENDPOINT_URL_S3` are needed to rebuild and aren't in git.
4. **(Informational — not a fault) Large WAL is expected.** Staging showed a
   244 MB `petrapp.sqlite3-wal` against a 5.3 MB DB. This is by design: the app
   disables SQLite autocheckpoint (`PRAGMA wal_autocheckpoint = 0`,
   `internal/sqlite/sqlite.go`) and delegates checkpointing to Litestream, whose
   0.5.x strategy only runs a WAL-shrinking **TRUNCATE** checkpoint at
   `truncate-page-n` (default ≈121k pages ≈ **500 MB**); the cheaper PASSIVE
   checkpoints replicate data but don't shrink the WAL *file*. So the WAL grows
   to ≈500 MB before resetting — still well under the volume's ~800 MB
   auto-extend trigger. Optional tuning: lower `truncate-page-n` / add a
   `checkpoint-interval` in `litestream.yml` to keep cold-start WAL replay
   smaller. Watch: on Litestream `v0.5.10` we are in the version range of
   [issue #1083](https://github.com/benbjohnson/litestream/issues/1083) (silent
   replication stall on WAL space reuse) — periodically confirm `litestream
   status` txid tracks the live DB.

## Failure scenario catalog

Data-loss bounds come from `litestream.yml`: the finest replica level syncs every
**5 minutes** (retention 1h), then 1h (24h), then 24h (168h). So the continuous
recovery point is ≈5 min; older points coarsen with age. `make fly-backup` also
takes on-machine `.backup` snapshots under `/data/snapshots/`.

| # | Scenario | Detection | Recovery procedure | Data loss / RPO | Drill status |
|---|---|---|---|---|---|
| 1 | App deleted / destroyed | App 404s; `fly apps list` missing it | Full rebuild procedure above | ≈5 min (last replica sync) if bucket intact; **total** if bucket + creds both gone | Desk-checked; S3-only restore proven on staging 2026-05-31 |
| 2 | Volume lost / corrupted, app intact | Boot fails to open DB; disk errors in `make fly-logs` | `fly ssh … -C "/dist/litestream restore -o /data/petrapp.sqlite3.new /data/petrapp.sqlite3"`, stop app, swap file in, restart | ≈5 min | Untested |
| 3 | Corrupted DB file | `PRAGMA integrity_check` ≠ `ok`; app errors | Restore from Litestream (as #2), or copy the latest `/data/snapshots/*` from `make fly-backup` into place | ≈5 min (Litestream) / since last snapshot | Untested |
| 4 | Bad migration shipped to prod | Post-deploy smoke test / errors; ideally CI `make migratetest` caught it against prod data first | Revert the offending commit on `main` (next push redeploys prior code). If data was mutated, restore the DB from the pre-deploy snapshot (`fly-sql-write` auto-snapshots before mutating) | Code: none. Data: back to pre-deploy snapshot if taken | Desk-checked |
| 5 | Accidental data deletion (one user / table) | User report; row counts drop | Restore the backup to a temp path (as #2), extract the affected rows with `sqlite3`, re-insert into the live DB via `make fly-sql-write SCRIPT=…` | Since the deletion | Untested |
| 6 | Tigris bucket lost / creds rotated or lost | Litestream replicate errors in `make fly-logs`; `litestream status` / `ltx` fail | Re-mint creds or recreate the bucket (`fly storage create`), set the `AWS_*` secrets, redeploy. If the bucket *data* is gone, restore from the newest `/data/snapshots/*` instead | None if creds-only; **total backup history** if bucket data is gone (last `.backup` snapshot is the floor) | Untested |
| 7 | VAPID secrets lost | Push sends fail; `fly secrets list` missing the keys | Re-set from the offline stash. If unavailable, regenerate — this **breaks existing push subscriptions**, users must re-subscribe | None to DB; push subscriptions lost if regenerated | Untested |
| 8 | Region (`arn`) outage | Fly status / app unreachable region-wide | Wait it out, or provision a volume in an alternate region and `fly deploy` there, restoring from the Litestream replica on boot | ≈5 min | Untested |

### Next drill to run

**Destructive full rebuild.** Actually `fly apps destroy petra-staging`, then
rebuild from S3 only, to prove end-to-end that the bucket survives app deletion
and that fresh Tigris credentials can be minted for it (gap-list item 1).
Deferred from the 2026-05-31 pass, which was non-destructive by design.
