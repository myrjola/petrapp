# Petra

Personal trainer app

## Quickstart

### Install dependencies, configure linting, and optinally set up git hooks

```
make
make setup-git-hooks # if you want to develop straight to main branch
```

### Start go server

```
make dev
```

This will start the server on a free local port and open the browser. You
can [attach a debugger](https://www.jetbrains.com/help/go/attach-to-running-go-processes-with-debugger.html) to it.

## Operations

The deployment is a single-node Fly Machine that **scales to zero when idle**, with the SQLite volume continuously
replicated by Litestream. The two environments are:

| Environment | Fly app          | URL                              |
|-------------|------------------|----------------------------------|
| Production  | `petra`          | `https://petra.fly.dev`          |
| Staging     | `petra-staging`  | `https://petra-staging.fly.dev`  |

### Select which Fly app is targeted

The `make fly-*` ops targets default to `FLY_APP=petra`. Override per-invocation for staging:

```sh
make fly-logs FLY_APP=petra-staging
```

If you invoke `fly` directly and get:

```
Error: the config for your app is missing an app name, add an app field to the fly.toml file or specify with the -a flag
```

…export `FLY_APP` or pass `--app`:

```sh
export FLY_APP=petra
```

### Waking a cold instance

Because the machine scales to zero, the first request after idle has to spin it up before any `fly ssh` /
`fly proxy` command will work. The `make fly-wake` target sends a `GET /api/healthy` and waits for a 200; every
other `make fly-*` target depends on it, so you don't normally invoke it manually.

```sh
make fly-wake                  # production
make fly-wake FLY_APP=petra-staging
```

### Deploying

This project uses [Fly.io](https://fly.io/) for infrastructure and [Litestream](https://litestream.io/)
for [SQLite](https://www.sqlite.org/) database backups. It's a single instance Dockerized application with a persistent
volume. Try `fly launch` to configure your own. You might also need to add some secrets to with `fly secrets`.

### Database access

The container image contains the `sqlite3` binary so you can manipulate the live database. There are three flavours:

```sh
# Interactive REPL (humans only).
make fly-sqlite3

# Non-interactive read-only — pass a SQL file.
echo "SELECT COUNT(*) FROM users;" > /tmp/q.sql
make fly-sql-readonly SCRIPT=/tmp/q.sql

# Mutating SQL — automatically takes a Litestream-style on-machine snapshot first.
make fly-sql-write SCRIPT=/tmp/migration.sql
```

`fly-sql-write` always invokes `fly-backup` before running the script. `fly-backup` itself can be run on its own:

```sh
make fly-backup                # snapshots prod's /data/petrapp.sqlite3 → /data/snapshots/<timestamp>.sqlite3
```

The snapshot is created with sqlite3's `.backup` command, which produces a single consistent file (no separate WAL).

### Recovering database

One way to recover a lost or broken database is to restore it with Litestream. The process could still use some
improvements but at least it works. Notably, you need to have a working machine running so that you can run commands on
it. Another alternative is to clone the machine with an empty volume and populate it yourself using the `fly sftp shell`
command.

```
# list databases
fly ssh console --app $FLY_APP --user petrapp -C "/dist/litestream databases"
# restore latest backup to /data/petrapp4.sqlite
fly ssh console --app $FLY_APP --user petrapp -C "/dist/litestream restore -o /data/petrapp4.sqlite /data/petrapp.sqlite3"

# Edit fly.toml env PETRAPP_SQLITE_URL = "/data/petrapp.sqlite3" before deploying to take new database into use
vim fly.toml

# Deploy the new configuration
fly deploy
```

### Performance investigation

#### pprof

Use [pprof](https://pkg.go.dev/net/http/pprof) for performance investigation. The `make` targets handle waking the
machine, spawning the proxy as a background process, and tearing it down when the capture finishes:

```sh
make fly-pprof-cpu             # 30-second CPU profile → pprof/cpu-<app>-<timestamp>.pb.gz
make fly-pprof-goroutine       # goroutine snapshot → pprof/goroutine-<app>-<timestamp>.pb.gz
```

Inspect the resulting files:

```sh
go tool pprof --http=: pprof/cpu-petra-*.pb.gz
go tool pprof -top   pprof/goroutine-petra-*.pb.gz
```

If you'd rather drive the proxy yourself (e.g., to capture an unusual profile type):

```sh
make fly-wake                  # ensure the machine is running
fly proxy --app $FLY_APP 6060:6060 &
go tool pprof --http=: "http://localhost:6060/debug/pprof/profile?seconds=30"
```

#### Flight Controller for automatic trace capture

When a request times out, the app writes a [trace](https://pkg.go.dev/runtime/trace) to a file and logs something like
the following line:

```json
{
  "time": "2025-09-13T10:02:11.604995985+03:00",
  "level": "WARN",
  "msg": "captured timeout trace",
  "service_name": "pr-29-myrjola-petrapp",
  "file": "/data/traces/timeout-20250913-070211.trace",
  "bytes": 709652,
  "trace_id": "HBGYTREFLURSGLEQGR2OX4XEBK",
  "proto": "HTTP/1.1",
  "method": "GET",
  "uri": "/api/test/timeout?sleep_ms=3000"
}
```

This file can be downloaded with the following replacing FLY_APP and file name with service_name and file from the log
line:

```
FLY_APP=pr-29-myrjola-petrapp fly sftp get /data/traces/timeout-20250913-070211.trace
```

Once you have the file, you can analyze it with:

```
go tool trace timeout-20250913-070211.trace
```

### CI/CD and preview environments

This project deploys continuously via [GitHub Actions](https://docs.github.com/en/actions). **You
should not run `fly deploy` by hand for routine changes** — the workflows below own that.

#### Pushing to `main` deploys to staging then prod

`.github/workflows/main.yml` runs on every push to `main`:

1. **Test** — runs `make ci` (build, lint, test, govulncheck) plus `make migratetest`. The
   migration test restores the latest **production** Litestream backup from S3 and runs the app's
   `NewDatabase` (pre-migrations + declarative migrate) against it. Risky schema changes are
   validated against real prod data here, before they ever reach a live machine.
2. **Build & push** — Docker image tagged with the commit SHA, pushed to the Fly registry.
3. **Staging deploy** — promotes the new image to `petra-staging`.
4. **Prod deploy** — only runs after staging succeeds, then promotes to `petra`.

If staging or its smoke test fails, prod is not touched. If you need to abort mid-pipeline, revert
the offending commit on `main` — the next push will redeploy the previous code.

#### Opening a PR creates a review app

`.github/workflows/fly-review.yml` provisions a per-PR Fly app on PR open / sync, and tears it
down on PR close. The pattern is:

- App name: `pr-<PR-number>-myrjola-petrapp` (note: derived from the GitHub repo slug
  `myrjola/petrapp`, not from the prod app name `petra`).
- URL: `https://pr-<PR-number>-myrjola-petrapp.fly.dev`.

Review apps use a **local Litestream replica** (`LITESTREAM_REPLICA_PATH=/data/backup`) — no S3
push — so they don't pollute the prod backup bucket. They also wake-from-zero like prod, so the
fly-ops `make` targets work against them with `FLY_APP=pr-<N>-myrjola-petrapp`.

#### Day-to-day flow

```sh
# Routine change:
git checkout -b my-change
# ... commit, push, open PR. Wait for CI green and review app to come up.
# Manually exercise https://pr-<N>-myrjola-petrapp.fly.dev. Merge to main when happy.
# CI auto-deploys to staging then prod.

# Hotfix straight to prod (rare):
# Same as above — there is no manual override path.
```

### Creating new deployment

Prerequisite: Ensure you have [Fly](https://fly.io/docs/) set up correctly with `fly auth whoami`.

Create a new app with a globally unique name.

```sh
fly apps create petrapp-staging
```

Create a bucket for the database backups. This should configure the secrets automatically matching the configuration in
litestream.yml.

```sh
fly storage create --app petrapp-staging --name petrapp-staging-backup
```

Now we are ready to deploy the app.

```sh
fly deploy --app petrapp-staging
```

## Attribution

Petra logo made by Martin Yrjölä using [Inkscape](https://inkscape.org/).
