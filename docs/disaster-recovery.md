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
4. **Investigate the oversized WAL.** Staging showed a 244 MB
   `petrapp.sqlite3-wal` against a 5.3 MB DB — Litestream isn't checkpointing
   (likely a scale-to-zero interaction). On the 1 GB→5 GB auto-extend volume
   this is a latent disk-pressure risk; confirm prod isn't similarly bloated.
