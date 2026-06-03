
.DEFAULT_GOAL := info

export GOTOOLCHAIN := auto
GOLANGCI_LINT_VERSION := v2.12.2

# Per-worktree golangci-lint cache. The cache stores diagnostics keyed by file
# content hash and records absolute paths in the cached results; sharing one
# cache across worktrees causes stale-path WARNs after a worktree is removed.
export GOLANGCI_LINT_CACHE := $(CURDIR)/.cache/golangci-lint

# Default Fly app for ops targets. Override with FLY_APP=petra-staging for staging.
FLY_APP ?= petra
# Path of the production database on the Fly machine.
FLY_DB_PATH ?= /data/petrapp.sqlite3

# Secrets backup. App secrets (VAPID keypair, OPENAI_API_KEY, Tigris creds) are
# write-only on Fly and live nowhere else, so we keep an age-encrypted copy in
# secrets/<app>.env.age (committed) and push it back during recovery. The age
# PRIVATE key lives outside the repo (default below) and in your password manager.
AGE_KEY ?= $(HOME)/.config/petrapp/secrets-age.key
SECRETS_DIR := secrets
SECRETS_RECIPIENTS := $(SECRETS_DIR)/age-recipients.txt

# Suppress linker warnings on macOS.
ifeq ($(shell uname -s),Darwin)
	export CGO_LDFLAGS := -w
endif

# ── Local development ────────────────────────────────────────────────

.PHONY: info
info:
	@echo "Run 'make clean && make ci' for a fresh build"

.PHONY: init
init: gomod bin/golangci-lint
	@echo "Dependencies installed"

.PHONY: gomod
gomod:
	@echo "Installing Go dependencies..."
	@go version
	@go mod download
	@go mod verify

bin/golangci-lint:
	@echo "Installing golangci-lint..."
	@curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/HEAD/install.sh | sh -s -- $(GOLANGCI_LINT_VERSION)

.PHONY: build
build:
	@echo "Building..."
	@go build -o bin/petrapp github.com/myrjola/petrapp/cmd/web
	@go build -o bin/smoketest github.com/myrjola/petrapp/cmd/smoketest
	@go build -o bin/migratetest github.com/myrjola/petrapp/cmd/migratetest
	@go build -o bin/stresstest github.com/myrjola/petrapp/cmd/stresstest

.PHONY: test
test:
	@echo "Running tests..."
	@go test --race --shuffle=on ./...

.PHONY: lint
lint: bin/golangci-lint
	@echo "Running linter..."
	@./bin/golangci-lint run

# fuzz runs the seed corpus for every Fuzz* target as ordinary tests (no fuzzing
# engine), so it is fast and fits into `make test`-style runs. To actually fuzz,
# pass FUZZTIME and a single target, e.g.
#   make fuzz FUZZ=FuzzConvertWeight FUZZTIME=30s
FUZZ     ?= Fuzz
FUZZTIME ?=
.PHONY: fuzz
fuzz:
ifeq ($(strip $(FUZZTIME)),)
	@echo "Running fuzz seed corpora..."
	@go test --race -run '$(FUZZ)' ./...
else
	@echo "Fuzzing $(FUZZ) for $(FUZZTIME)..."
	@go test -run '^$$' -fuzz '$(FUZZ)' -fuzztime '$(FUZZTIME)' ./internal/domain/
endif

.PHONY: lint-fix
lint-fix: bin/golangci-lint
	@echo "Running linter with auto-fix..."
	@./bin/golangci-lint run --fix

.PHONY: sec
sec:
	@go tool govulncheck ./...

.PHONY: ci
ci: init build lint-fix test sec

.PHONY: clean
clean:
	@echo "Cleaning up..."
	@rm -rf bin

.PHONY: dev
dev:
	@echo "Running dev server with debug build..."
	@go build -gcflags="all=-N -l" -o bin/petrapp github.com/myrjola/petrapp/cmd/web
	@bash scripts/dev.sh

.PHONY: dev-tailnet
dev-tailnet: build  ## Build and run with Tailscale HTTPS (for iOS WebAuthn).
	@bash scripts/dev-tailscale-https.sh

.PHONY: build-docker
build-docker:
	@echo "Building Docker image..."
	@docker build --tag petrapp .

.PHONY: migratetest
migratetest: build
	@echo "Deleting previous restored backup..."
	@rm -rf restored.sqlite3* .restored.sqlite3-litestream/
	@echo "Restoring database from backup..."
	@litestream restore --config litestream.yml restored.sqlite3
	@echo "Running migration test..."
	@bin/migratetest

.PHONY: repomix
repomix:
	@npx repomix --include "**/*.go,**/*.gohtml,**/*.js,**/*.css,**/schema.sql" --output repomix-output.txt

.PHONY: repomix-clipboard
repomix-clipboard: repomix
	@cat repomix-output.txt | pbcopy

.PHONY: setup-git-hooks
setup-git-hooks:
	@./scripts/setup-git-hooks.sh

# ── Fly.io ops ───────────────────────────────────────────────────────

# fly-wake issues an HTTP request to wake the deployed instance, since it scales to zero when idle.
# Every other fly-* target depends on this so commands don't time out against a cold machine.
.PHONY: fly-wake
fly-wake:
	@echo "-> waking $(FLY_APP)..."
	@curl -fsS --max-time 60 --retry 3 https://$(FLY_APP).fly.dev/api/healthy >/dev/null
	@echo "  awake."

.PHONY: fly-sqlite3
fly-sqlite3: fly-wake
	@echo "Connecting to sqlite3 database on $(FLY_APP)"
	@fly ssh console --app $(FLY_APP) --pty --user petrapp \
		-C "/usr/bin/sqlite3 -cmd \"PRAGMA foreign_keys = ON;\" $(FLY_DB_PATH)"

# fly-sql-readonly runs a SQL script against the deployed database in read-only mode.
# Pass SCRIPT=path/to/query.sql.
.PHONY: fly-sql-readonly
fly-sql-readonly: fly-wake
ifndef SCRIPT
	$(error SCRIPT is required, e.g. make fly-sql-readonly SCRIPT=/tmp/q.sql)
endif
	@cat "$(SCRIPT)" | fly ssh console --app $(FLY_APP) --user petrapp \
		-C "/usr/bin/sqlite3 -readonly $(FLY_DB_PATH)"

# fly-backup snapshots the live database on the Fly machine via sqlite3 .backup, which produces
# a single consistent file (unlike a raw cp that needs DB + WAL). Stored under /data/snapshots/.
.PHONY: fly-backup
fly-backup: fly-wake
	@TS=$$(date -u +%Y%m%dT%H%M%SZ) ; \
	  REMOTE=/data/snapshots/petrapp-$(FLY_APP)-$$TS.sqlite3 ; \
	  echo "-> snapshotting $(FLY_DB_PATH) -> $$REMOTE on $(FLY_APP)" ; \
	  fly ssh console --app $(FLY_APP) --user petrapp \
	    -C "/bin/sh -c 'mkdir -p /data/snapshots && /usr/bin/sqlite3 $(FLY_DB_PATH) \".backup $$REMOTE\"'" ; \
	  echo "  snapshot at $$REMOTE"

# fly-sql-write runs a SQL script that may mutate the database. Always takes a backup first.
# The script is piped via SSH stdin (same pattern as fly-sql-readonly), so nothing is written
# to disk on the remote and there's no cleanup to fail. Pass SCRIPT=path/to/migration.sql.
.PHONY: fly-sql-write
fly-sql-write: fly-wake fly-backup
ifndef SCRIPT
	$(error SCRIPT is required, e.g. make fly-sql-write SCRIPT=/tmp/migration.sql)
endif
	@echo "-> executing $(SCRIPT) on $(FLY_APP)..."
	@cat "$(SCRIPT)" | fly ssh console --app $(FLY_APP) --user petrapp \
	    -C "/usr/bin/sqlite3 $(FLY_DB_PATH)"

.PHONY: fly-logs
fly-logs: fly-wake
	@fly logs --app $(FLY_APP) --no-tail

# fly-pprof-cpu captures a 30s CPU profile from the running instance. Spawns the proxy as a
# background process and tears it down when the capture finishes.
.PHONY: fly-pprof-cpu
fly-pprof-cpu: fly-wake
	@mkdir -p pprof
	@OUT=pprof/cpu-$(FLY_APP)-$$(date -u +%Y%m%dT%H%M%SZ).pb.gz ; \
	  echo "-> proxying 6060 -> $(FLY_APP)..." ; \
	  fly proxy --app $(FLY_APP) 6060:6060 >/dev/null 2>&1 & PROXY_PID=$$! ; \
	  trap "kill $$PROXY_PID 2>/dev/null" EXIT ; \
	  sleep 2 ; \
	  echo "-> capturing 30s CPU profile..." ; \
	  curl -fsS -o $$OUT "http://localhost:6060/debug/pprof/profile?seconds=30" ; \
	  echo "  saved $$OUT (open with: go tool pprof --http=: $$OUT)"

.PHONY: fly-pprof-goroutine
fly-pprof-goroutine: fly-wake
	@mkdir -p pprof
	@OUT=pprof/goroutine-$(FLY_APP)-$$(date -u +%Y%m%dT%H%M%SZ).pb.gz ; \
	  fly proxy --app $(FLY_APP) 6060:6060 >/dev/null 2>&1 & PROXY_PID=$$! ; \
	  trap "kill $$PROXY_PID 2>/dev/null" EXIT ; \
	  sleep 2 ; \
	  curl -fsS -o $$OUT "http://localhost:6060/debug/pprof/goroutine" ; \
	  echo "  saved $$OUT (open with: go tool pprof -top $$OUT)"

# Load-test shape for fly-stresstest. Override on the command line, e.g.
# make fly-stresstest FLY_APP=petra-staging STRESS_USERS=50 STRESS_DURATION=5m STRESS_THINK=0.
STRESS_USERS    ?= 30
STRESS_DURATION ?= 2m
STRESS_THINK    ?= 1s

# fly-stresstest drives sustained load against the selected app while capturing CPU + heap
# profiles spanning the load window, then writes a JSON latency report — the way to profile a
# bottleneck under load rather than on an idle machine. Spawns the pprof proxy, runs the load,
# and tears the proxy down on exit. Captures land in pprof/ (cpu-<ts>.pb.gz, heap-<ts>.pb.gz,
# report-<ts>.json).
#
# Refuses prod (petra) unless STRESS_FORCE=1: a run registers synthetic users and writes
# workout data, so it must never hit prod by accident. Point it at petra-staging instead.
# The guard is a prerequisite ordered before fly-wake so a misfire never even wakes prod.
.PHONY: stress-guard
stress-guard:
	@if [ "$(FLY_APP)" = "petra" ] && [ -z "$(STRESS_FORCE)" ]; then \
	  echo "refusing to stresstest prod (petra): it registers synthetic users and writes workout data." >&2 ; \
	  echo "use FLY_APP=petra-staging, or pass STRESS_FORCE=1 to override." >&2 ; \
	  exit 1 ; \
	fi

.PHONY: fly-stresstest
fly-stresstest: stress-guard fly-wake build
	@mkdir -p pprof
	@echo "-> proxying 6060 -> $(FLY_APP)..." ; \
	  fly proxy --app $(FLY_APP) 6060:6060 >/dev/null 2>&1 & PROXY_PID=$$! ; \
	  trap "kill $$PROXY_PID 2>/dev/null" EXIT ; \
	  sleep 2 ; \
	  echo "-> stresstest ($(STRESS_USERS) users, $(STRESS_DURATION), think $(STRESS_THINK)) -> $(FLY_APP)..." ; \
	  ./bin/stresstest --users $(STRESS_USERS) --duration $(STRESS_DURATION) --think $(STRESS_THINK) \
	    --pprof-url http://localhost:6060 --out pprof $(FLY_APP).fly.dev ; \
	  echo "  captures in pprof/ (open with: go tool pprof --http=: pprof/cpu-<timestamp>.pb.gz)"

# ── Secrets backup (age-encrypted env files) ─────────────────────────
# One-time setup:
#   1. make secrets-keygen                  # creates the age key + prints the public key
#   2. add the printed public key to secrets/age-recipients.txt
#   3. cp secrets/petra.env.example secrets/petra.env  # fill in real values from `fly secrets`
#   4. make secrets-encrypt FLY_APP=petra && rm secrets/petra.env
#   5. commit secrets/petra.env.age
# Recovery: make fly-secrets-push FLY_APP=petra  (decrypts and imports into Fly).
# See secrets/README.md for the full workflow.

.PHONY: secrets-keygen
secrets-keygen:
	@command -v age-keygen >/dev/null 2>&1 || { echo "age-keygen not found; install age: https://github.com/FiloSottile/age" >&2 ; exit 1 ; }
	@if [ -f "$(AGE_KEY)" ]; then echo "age key already exists at $(AGE_KEY); refusing to overwrite" >&2 ; exit 1 ; fi
	@mkdir -p "$(dir $(AGE_KEY))"
	@age-keygen -o "$(AGE_KEY)"
	@echo
	@echo "Public key (add it to $(SECRETS_RECIPIENTS)):"
	@age-keygen -y "$(AGE_KEY)"
	@echo
	@echo "BACK UP the PRIVATE key at $(AGE_KEY) in your password manager — without it the .age files cannot be decrypted."

.PHONY: secrets-encrypt
secrets-encrypt:
	@command -v age >/dev/null 2>&1 || { echo "age not found; install age: https://github.com/FiloSottile/age" >&2 ; exit 1 ; }
	@test -s "$(SECRETS_RECIPIENTS)" || { echo "no recipients in $(SECRETS_RECIPIENTS); run 'make secrets-keygen' and add the public key" >&2 ; exit 1 ; }
	@test -f "$(SECRETS_DIR)/$(FLY_APP).env" || { echo "missing plaintext $(SECRETS_DIR)/$(FLY_APP).env (copy from $(SECRETS_DIR)/petra.env.example)" >&2 ; exit 1 ; }
	@age -R "$(SECRETS_RECIPIENTS)" -o "$(SECRETS_DIR)/$(FLY_APP).env.age" "$(SECRETS_DIR)/$(FLY_APP).env"
	@echo "encrypted -> $(SECRETS_DIR)/$(FLY_APP).env.age"
	@echo "now delete the plaintext: rm $(SECRETS_DIR)/$(FLY_APP).env"

.PHONY: secrets-decrypt
secrets-decrypt:
	@command -v age >/dev/null 2>&1 || { echo "age not found; install age: https://github.com/FiloSottile/age" >&2 ; exit 1 ; }
	@test -f "$(AGE_KEY)" || { echo "no age key at $(AGE_KEY); restore it from your password manager" >&2 ; exit 1 ; }
	@test -f "$(SECRETS_DIR)/$(FLY_APP).env.age" || { echo "missing $(SECRETS_DIR)/$(FLY_APP).env.age" >&2 ; exit 1 ; }
	@age -d -i "$(AGE_KEY)" -o "$(SECRETS_DIR)/$(FLY_APP).env" "$(SECRETS_DIR)/$(FLY_APP).env.age"
	@echo "decrypted -> $(SECRETS_DIR)/$(FLY_APP).env (gitignored; delete when done editing, then re-run secrets-encrypt)"

# fly-secrets-push decrypts the committed ciphertext straight into `fly secrets import`
# over a pipe, so the plaintext never touches disk. This triggers a Fly release.
.PHONY: fly-secrets-push
fly-secrets-push:
	@command -v age >/dev/null 2>&1 || { echo "age not found; install age: https://github.com/FiloSottile/age" >&2 ; exit 1 ; }
	@test -f "$(AGE_KEY)" || { echo "no age key at $(AGE_KEY); restore it from your password manager" >&2 ; exit 1 ; }
	@test -f "$(SECRETS_DIR)/$(FLY_APP).env.age" || { echo "missing $(SECRETS_DIR)/$(FLY_APP).env.age" >&2 ; exit 1 ; }
	@echo "-> importing secrets into $(FLY_APP) from $(SECRETS_DIR)/$(FLY_APP).env.age"
	@age -d -i "$(AGE_KEY)" "$(SECRETS_DIR)/$(FLY_APP).env.age" | fly secrets import --app "$(FLY_APP)"
