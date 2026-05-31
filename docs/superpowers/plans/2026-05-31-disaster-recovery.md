# Disaster Recovery Runbook Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Produce `docs/disaster-recovery.md` — a single authoritative recovery runbook with a failure-scenario catalog and an "app deleted" recovery inventory whose findings are validated by a real, non-destructive walkthrough against `petra-staging`.

**Architecture:** Documentation deliverable. The "test" for each recovery claim is that the read-only command actually works against live staging and its output is captured into the runbook — no placeholders. Work proceeds: capture real findings (Task 1) → write the validated Part B inventory (Task 2) → write the Part A scenario catalog (Task 3) → link from README (Task 4). Each task ends in a commit.

**Tech Stack:** Fly.io (`fly` CLI), Litestream (`/dist/litestream` on the machine), SQLite (`sqlite3`), Markdown. Spec: `docs/superpowers/specs/2026-05-31-disaster-recovery-design.md`.

---

## Important constraints for the executor

- **Target staging only.** Every `fly` command in Task 1 uses `--app petra-staging` (or `make ... FLY_APP=petra-staging`). Never run these against `petra` (prod).
- **Non-destructive.** The only write to the staging machine is a throwaway restore file under `/data/`, which is deleted in the same task. The real DB (`/data/petrapp.sqlite3`) is never touched.
- **Expect permission prompts and a cold-start wake.** Staging scales to zero; the first command wakes it (~5–15s).
- Reference patterns live in the `Makefile` (`fly-*` targets), `README.md` ("Recovering database"), and the `fly-ops` skill.
- The staging machine runs as user `petrapp`; the DB is at `/data/petrapp.sqlite3`; the Litestream binary is `/dist/litestream` with config `/etc/litestream.yml`.

---

## Task 1: Capture real findings from the non-destructive staging walkthrough

**Files:**
- Create (scratch, not committed): `/tmp/dr-walkthrough-findings.md` — paste raw command output here as you go.

The goal of this task is to gather **real output** that Task 2 turns into the runbook. Run each step, paste its output into the scratch file under a labeled heading.

- [ ] **Step 1: Wake staging**

Run:
```bash
make fly-wake FLY_APP=petra-staging
```
Expected: `awake.` If it fails, staging may need `fly apps list` to confirm the exact app name; do not substitute prod.

- [ ] **Step 2: Confirm the backup bucket is an independent resource**

Run:
```bash
fly storage list --app petra-staging
```
Capture: the bucket name and whether it is listed as an org/Tigris resource separate from the app. This answers "does the bucket outlive the app?" — note in findings whether deletion of the app would plausibly take the bucket with it (Tigris buckets are separate resources, but confirm what the output shows).

- [ ] **Step 3: Enumerate the secrets that are set**

Run:
```bash
fly secrets list --app petra-staging
```
Capture the list of secret **names** (values are not shown). Cross-check against the required set the app/Litestream need:
- `PETRAPP_VAPID_PUBLIC`, `PETRAPP_VAPID_PRIVATE`
- `AWS_ACCESS_KEY_ID`, `AWS_SECRET_ACCESS_KEY`, `BUCKET_NAME`, `AWS_ENDPOINT_URL_S3`, `AWS_REGION`, `LITESTREAM_REPLICA_PATH`

Note in findings which are present and, critically, that **none of these values are recoverable from the Litestream backup or git** — they exist only inside Fly.

- [ ] **Step 4: List the Litestream databases and generations on the machine**

Run:
```bash
fly ssh console --app petra-staging --user petrapp -C "/dist/litestream databases"
fly ssh console --app petra-staging --user petrapp -C "/dist/litestream snapshots /data/petrapp.sqlite3"
```
Capture: the replica path/bucket and the list of snapshots with timestamps. This proves the S3 replica is real and shows the effective recovery point (newest snapshot + WAL). If `snapshots` errors, try `/dist/litestream generations /data/petrapp.sqlite3` and capture that instead.

- [ ] **Step 5: Restore the latest backup to a throwaway path and verify integrity**

Run:
```bash
fly ssh console --app petra-staging --user petrapp -C "/dist/litestream restore -o /data/dr-drill-restore.sqlite3 /data/petrapp.sqlite3"
fly ssh console --app petra-staging --user petrapp -C "/usr/bin/sqlite3 -readonly /data/dr-drill-restore.sqlite3 'PRAGMA integrity_check; SELECT count(*) FROM users;'"
```
Capture: the restore log line(s), `integrity_check` result (expect `ok`), and the user count. This is the proof that an S3-only restore yields a usable DB.

- [ ] **Step 6: Clean up the throwaway restore file**

Run:
```bash
fly ssh console --app petra-staging --user petrapp -C "/bin/sh -c 'rm -f /data/dr-drill-restore.sqlite3 /data/dr-drill-restore.sqlite3-* ; ls -la /data'"
```
Expected: the `dr-drill-restore*` files are gone; `/data/petrapp.sqlite3` is untouched. Capture the `ls` output as proof staging is left clean.

- [ ] **Step 7: Note open questions for the gap list**

In the scratch file, write down anything the walkthrough could NOT confirm read-only, e.g.:
- Whether the Tigris bucket has deletion protection / a separate lifecycle from the app.
- Whether the VAPID keypair is stashed anywhere outside Fly.
- Whether the S3 access key can be regenerated from the Tigris dashboard if lost.

No commit this task (scratch file only). Proceed directly to Task 2.

---

## Task 2: Write `docs/disaster-recovery.md` with the validated Part B inventory

**Files:**
- Create: `docs/disaster-recovery.md`

- [ ] **Step 1: Create the runbook skeleton and Part B from real findings**

Create `docs/disaster-recovery.md` with this structure. Fill every bracketed `«…»` slot with the **actual captured output** from Task 1 — do not leave them bracketed.

```markdown
# Disaster Recovery Runbook

Last validated: 2026-05-31 against `petra-staging` (non-destructive walkthrough).

PetraApp is a single Fly Machine (scale-to-zero) with SQLite on a volume,
continuously replicated by Litestream to a Tigris/S3 bucket. CI
(`make migratetest`) restores the latest **production** backup on every push to
`main`, so DB restore-ability is exercised daily.

**The backup restores DATA ONLY.** Reconstituting the app after a catastrophic
loss also needs secrets and the bucket itself — see the inventory below.

## Recovery inventory — what it takes to rebuild from nothing

| Artifact | Source | Recoverable if app deleted? | Status (staging, 2026-05-31) |
|---|---|---|---|
| Data | Litestream S3 replica | Yes | «snapshot list + integrity_check=ok + user count from Task 1» |
| App/infra config | git (`fly.toml`, `Dockerfile`, `litestream.yml`) | Yes | In repo |
| VAPID keypair | `fly secrets` | Only if stashed outside Fly; regenerating breaks push subs | «present? from Step 3» |
| Tigris bucket + creds | `fly storage create` | Critical — gone = total data loss | «bucket name + independence note from Steps 2 & 4» |
| WebAuthn RPID | derived from `PETRAPP_FQDN` | Yes, while domain stays `petra.fly.dev` | n/a |

## Full rebuild procedure ("app deleted")

Prerequisite: `fly auth whoami` works; you have the VAPID keypair and Tigris
credentials from your offline stash (see Gap list).

1. Recreate the app:           `fly apps create petra`
2. Recreate/relink storage:    «exact command, from README "Creating new deployment" + Step 2 findings»
3. Restore the VAPID + S3 secrets: `fly secrets set PETRAPP_VAPID_PUBLIC=… PETRAPP_VAPID_PRIVATE=… --app petra` (plus AWS_* if not auto-set by storage)
4. Deploy:                      `fly deploy --app petra`
5. On first boot the entrypoint (`litestream replicate -exec ./petrapp`) restores from the replica. Verify with `make fly-sqlite3 FLY_APP=petra` → `PRAGMA integrity_check; SELECT count(*) FROM users;`

## Gap list (prioritized)

1. «highest-priority gap from findings — e.g. confirm Tigris bucket survives app deletion / has deletion protection»
2. «stash VAPID keypair + Tigris creds in a password manager, since they are unrecoverable from backup/git»
3. «record bucket name + endpoint outside Fly»
4. «any open question from Step 7»
```

- [ ] **Step 2: Verify no placeholder slots remain**

Run:
```bash
grep -n "«" docs/disaster-recovery.md || echo "no placeholders"
```
Expected: `no placeholders`. If any `«…»` remain, fill them from the Task 1 scratch file.

- [ ] **Step 3: Commit**

```bash
git add docs/disaster-recovery.md
git commit -m "docs(dr): add recovery runbook with validated app-deleted inventory"
```

---

## Task 3: Write Part A — the failure scenario catalog

**Files:**
- Modify: `docs/disaster-recovery.md`

- [ ] **Step 1: Append the scenario catalog**

Append this section to `docs/disaster-recovery.md`. Fill each row with concrete detection signals and commands; use the `make fly-*` targets where they exist (`fly-backup`, `fly-sql-write`, `fly-sql-readonly`, `fly-sqlite3`). Data-loss bounds come from Litestream config (`litestream.yml`): 5m replica interval (≈5 min RPO for the continuous replica), 24h snapshot interval, 168h retention.

```markdown
## Failure scenario catalog

| # | Scenario | Detection | Recovery procedure | Data loss / RPO | Drill status |
|---|---|---|---|---|---|
| 1 | App deleted / destroyed | App 404s; `fly apps list` missing it | Full rebuild procedure above | ≈5 min (last WAL push) if bucket intact; total if bucket gone | Desk-checked + non-destructive restore proven on staging 2026-05-31 |
| 2 | Volume lost/corrupted, app intact | Boot fails to open DB; disk errors in `make fly-logs` | `fly ssh` → `/dist/litestream restore -o /data/petrapp.sqlite3.new /data/petrapp.sqlite3`, swap in, restart | ≈5 min | Untested |
| 3 | Corrupted DB file | `PRAGMA integrity_check` ≠ ok; app errors | Restore from Litestream (as #2) or latest `/data/snapshots/*` from `make fly-backup` | ≈5 min (Litestream) / since last snapshot | Untested |
| 4 | Bad migration in prod | Smoke test fails / errors post-deploy; CI `make migratetest` ideally caught it first | Revert offending commit on `main` (redeploys prior code); if data mutated, restore DB from pre-deploy snapshot | depends; pre-deploy snapshot if taken | Desk-checked |
| 5 | Accidental data deletion (user/table) | User report; row counts drop | Restore backup to a temp path, extract the affected rows, re-insert via `make fly-sql-write` | since the deletion | Untested |
| 6 | Tigris bucket lost / creds rotated | Litestream replicate errors in logs; `litestream snapshots` fails | Recreate bucket/creds (`fly storage create`), set AWS_* secrets, redeploy; if bucket data gone, restore from newest `/data/snapshots/*` | total backup history if bucket data gone | Untested |
| 7 | VAPID secrets lost | Push sends fail; `fly secrets list` missing keys | Re-set from offline stash; if unavailable, regenerate (breaks existing push subscriptions — users must re-subscribe) | none to DB; push subs lost if regenerated | Untested |
| 8 | Region (`arn`) outage | Fly status / app unreachable region-wide | Wait out, or `fly deploy` to an alternate region after provisioning a volume there and restoring from Litestream | ≈5 min | Untested |

### Next drill to run

Destructive full rebuild: actually `fly apps destroy petra-staging`, then rebuild
from S3 only, to prove the bucket survives app deletion end-to-end. Deferred from
the 2026-05-31 pass (non-destructive only).
```

- [ ] **Step 2: Sanity-check the data-loss column against litestream.yml**

Run:
```bash
grep -A3 "levels:" litestream.yml
```
Expected: a 5m interval level — confirm the "≈5 min" RPO claims match. Adjust the table if the config differs.

- [ ] **Step 3: Commit**

```bash
git add docs/disaster-recovery.md
git commit -m "docs(dr): add failure scenario catalog"
```

---

## Task 4: Link the runbook from the README and finish

**Files:**
- Modify: `README.md` (the "## Operations" / "Recovering database" area)

- [ ] **Step 1: Add a pointer near the Operations section**

In `README.md`, under the `### Recovering database` heading, add a line at the top of that subsection:

```markdown
> **Full disaster-recovery runbook:** see [`docs/disaster-recovery.md`](docs/disaster-recovery.md)
> for the complete scenario catalog and the "rebuild from nothing" procedure. The notes below
> cover the common Litestream restore.
```

- [ ] **Step 2: Verify the link resolves**

Run:
```bash
test -f docs/disaster-recovery.md && grep -n "docs/disaster-recovery.md" README.md
```
Expected: the file exists and the README line is present.

- [ ] **Step 3: Commit**

```bash
git add README.md
git commit -m "docs(dr): link disaster-recovery runbook from README"
```

- [ ] **Step 4: Final review**

Read `docs/disaster-recovery.md` top to bottom. Confirm: inventory has real staging findings, all 8 scenarios present, gap list is concrete, no `«…»` slots, staging left clean (Task 1 Step 6). Done.

---

## Self-review notes (plan author)

- **Spec coverage:** Part A catalog (Task 3, all 8 scenarios) ✓; Part B validated inventory + walkthrough (Tasks 1–2) ✓; single `docs/disaster-recovery.md` + README link (Tasks 2,4) ✓; non-destructive, staging-only, no new automation, no formal RTO/RPO (constraints + out-of-scope) ✓; success criteria "real findings not placeholders" enforced by Task 2 Step 2 grep ✓.
- **Placeholder scan:** The `«…»` markers are intentional fill-from-real-output slots, guarded by an explicit grep gate (Task 2 Step 2) so they cannot survive into the committed doc.
- **Consistency:** App name `petra-staging` for the drill, `petra` for the rebuild example, used consistently; DB path `/data/petrapp.sqlite3`, user `petrapp`, litestream `/dist/litestream` match the Makefile and README throughout.
```
