---
name: fly-ops
description: |
  Database, log, and performance ops against the deployed Petra Fly Machines. Use this skill when
  the user asks to run a query, read logs, capture a profile, snapshot the database, or otherwise
  inspect/mutate a deployed environment. Handles waking the scale-to-zero instance, defaults to
  production with explicit safety steps for writes.
argument-hint: "[task description, e.g. 'how many users on staging' or 'profile prod cpu']"
allowed-tools:
  - Bash(make fly-wake*)
  - Bash(make fly-sql-readonly*)
  - Bash(make fly-sql-write*)
  - Bash(make fly-backup*)
  - Bash(make fly-logs*)
  - Bash(make fly-pprof-cpu*)
  - Bash(make fly-pprof-goroutine*)
  - Bash(make fly-sqlite3*)
  - Bash(curl https://petra.fly.dev*)
  - Bash(curl https://petra-staging.fly.dev*)
  - Bash(fly status*)
  - Bash(fly logs*)
  - Bash(fly proxy*)
  - Bash(go tool pprof*)
---

# Fly ops for Petra

You're operating against deployed Fly Machines. The deployment is a single-node SQLite app that
**scales to zero**, with Litestream replicating to S3.

## Environments

| Target  | `FLY_APP`        | URL                              |
|---------|------------------|----------------------------------|
| Prod    | `petra`          | `https://petra.fly.dev`          |
| Staging | `petra-staging`  | `https://petra-staging.fly.dev`  |

**Default target is production (`petra`).** All `make fly-*` targets accept `FLY_APP=petra-staging`
to retarget. Be explicit in every command you run — pass `FLY_APP=...` even when targeting prod, so
the target is visible in the transcript.

## Safety protocol

Match action to risk:

- **Reads** (SELECT-only, logs, profiles): run without confirmation.
- **Writes against staging**: confirm the SQL with the user before executing.
- **Writes against prod**: always confirm. Always show the user (a) the exact command you'll run,
  (b) the SQL contents, and (c) confirmation that the pre-write backup completed. Never bundle
  multiple destructive statements in one script unless the user explicitly asked for that.
- **Schema migrations**: prefer running them from the codebase's declarative migration path
  (a deploy will trigger `migrateTo`) rather than ad-hoc `fly-sql-write`. If you must use
  `fly-sql-write` for a one-shot, snapshot first (the target does this automatically) and tell the
  user where the snapshot lives.

If you're unsure whether a query mutates state, treat it as a write.

## Workflow patterns

### 1. Run SQL against the deployed database

For any query, write it to a file first (this avoids shell-escaping issues) and route through the
make target. The target handles waking and read-only enforcement.

```bash
# Read-only:
cat > /tmp/q.sql <<'SQL'
SELECT COUNT(*) AS users, MAX(created) AS most_recent FROM users;
SQL
make fly-sql-readonly SCRIPT=/tmp/q.sql FLY_APP=petra
```

For writes, the `fly-sql-write` target snapshots the DB on the machine first, uploads the script
via `fly sftp`, runs it, and removes the script:

```bash
cat > /tmp/migration.sql <<'SQL'
UPDATE feature_flags SET enabled = 1 WHERE name = 'maintenance_mode';
SQL
# Confirm with the user before running this:
make fly-sql-write SCRIPT=/tmp/migration.sql FLY_APP=petra
```

The user's SQL files live in `/tmp/` by convention — they're transient. Do not commit them.

### 2. Troubleshoot issues by reading logs

```bash
make fly-logs FLY_APP=petra
make fly-logs FLY_APP=petra-staging | grep -i error
```

`fly logs --no-tail` returns a bounded snapshot. To narrow the window or filter by route, pipe to
`grep`. When you see a stack trace or error message, look up the relevant code:

- HTTP handlers: `cmd/web/handler-*.go`
- Service layer: `internal/workout/service.go`
- DB layer: `internal/sqlite/`, `internal/workout/repository-*.go`

For request-timeout traces, the README explains how to fetch them via `fly sftp get` and analyze
with `go tool trace`.

### 3. Investigate performance issues

The `fly-pprof-*` targets manage the proxy lifecycle automatically — they spawn `fly proxy`
in the background, wait for it, capture, then tear it down on exit:

```bash
make fly-pprof-cpu FLY_APP=petra          # 30s CPU profile
make fly-pprof-goroutine FLY_APP=petra    # goroutine dump
```

Profiles land in `pprof/` (gitignored). Open them with:

```bash
go tool pprof -top pprof/cpu-petra-<timestamp>.pb.gz
# or interactive:
go tool pprof --http=: pprof/cpu-petra-<timestamp>.pb.gz
```

### 4. Database backups before dangerous operations

`make fly-sql-write` always invokes `make fly-backup` first, so a normal write flow is already
covered. Run `fly-backup` on its own when you want a snapshot before a sequence of operations
that don't all go through `fly-sql-write` (e.g., a deploy that includes a migration, or before
manual investigation that might lead to a write):

```bash
make fly-backup FLY_APP=petra
# → /data/snapshots/petrapp-petra-<timestamp>.sqlite3 on the Fly machine
```

The snapshot is taken with sqlite3 `.backup`, which is a single consistent file (no separate
WAL). It lives on the same volume as the live database. Litestream's continuous replication is
the second line of defense and runs independently.

If a write goes wrong and you need to roll back, the snapshot path will be in the output of the
preceding `fly-sql-write` invocation. Restoring it requires shelling in and copying:

```bash
fly ssh console --app petra --user petrapp \
  -C "/bin/sh -c 'cp /data/snapshots/<snapshot>.sqlite3 /data/petrapp.sqlite3'"
```

Pause and confirm with the user before any restore — it overwrites the live DB.

## Don't

- Don't invoke `fly` commands without `--app $FLY_APP` (they fail with a confusing error and may
  silently target the wrong app if `FLY_APP` happens to be exported).
- Don't use `make fly-sqlite3` from this skill — it's an interactive REPL meant for humans. Use
  `fly-sql-readonly` or `fly-sql-write` instead.
- Don't re-implement the make targets with raw `fly ssh console` invocations. The targets handle
  waking, backup, and cleanup; reproducing those by hand is error-prone.
- Don't paste the user's database contents into chat without confirmation — query results may
  contain PII.
