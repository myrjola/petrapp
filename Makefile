
.DEFAULT_GOAL := info
.PHONY: info ci gomod init build test dev lint build-docker fly-sqlite3 clean sec \
        migratetest repomix repomix-clipboard setup-git-hooks lint-fix \
        fly-wake fly-sql-readonly fly-sql-write fly-backup fly-logs fly-pprof-cpu fly-pprof-goroutine

export GOTOOLCHAIN := auto
GOLANGCI_LINT_VERSION := v2.11.4

# Default Fly app for ops targets. Override with FLY_APP=petra-staging for staging.
FLY_APP ?= petra
# Path of the production database on the Fly machine.
FLY_DB_PATH ?= /data/petrapp.sqlite3

# Suppress linker warnings on macOS
ifeq ($(shell uname -s),Darwin)
	export CGO_LDFLAGS := -w
endif

info:
	@echo "Run 'make clean && make ci' for a fresh build"

init: gomod bin/golangci-lint
	@echo "Dependencies installed"

gomod:
	@echo "Installing Go dependencies..."
	go version
	go mod download
	go mod verify

bin/golangci-lint:
	@echo "Installing golangci-lint..."
	curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/HEAD/install.sh | sh -s -- $(GOLANGCI_LINT_VERSION)

sec:
	go tool govulncheck ./...

ci: init build lint-fix test sec

build:
	@echo "Building..."
	go build -o bin/petrapp github.com/myrjola/petrapp/cmd/web
	go build -o bin/smoketest github.com/myrjola/petrapp/cmd/smoketest
	go build -o bin/migratetest github.com/myrjola/petrapp/cmd/migratetest
	go build -o bin/stresstest github.com/myrjola/petrapp/cmd/stresstest

test:
	@echo "Running tests..."
	go test --race --shuffle=on ./...

lint: bin/golangci-lint
	@echo "Running linter..."
	./bin/golangci-lint run

dev:
	@echo "Running dev server with debug build..."
	go build -gcflags="all=-N -l" -o bin/petrapp github.com/myrjola/petrapp/cmd/web
	bash scripts/dev.sh

build-docker:
	@echo "Building Docker image..."
	docker build --tag petrapp .

fly-sqlite3: fly-wake
	@echo "Connecting to sqlite3 database on $(FLY_APP)"
	fly ssh console --app $(FLY_APP) --pty --user petrapp \
		-C "/usr/bin/sqlite3 -cmd \"PRAGMA foreign_keys = ON;\" $(FLY_DB_PATH)"

# fly-wake issues an HTTP request to wake the deployed instance, since it scales to zero when idle.
# Every other fly-* target depends on this so commands don't time out against a cold machine.
fly-wake:
	@echo "→ waking $(FLY_APP)..."
	@curl -fsS --max-time 60 --retry 3 https://$(FLY_APP).fly.dev/api/healthy >/dev/null
	@echo "  awake."

# fly-sql-readonly runs a SQL script against the deployed database in read-only mode.
# Pass SCRIPT=path/to/query.sql.
fly-sql-readonly: fly-wake
ifndef SCRIPT
	$(error SCRIPT is required, e.g. make fly-sql-readonly SCRIPT=/tmp/q.sql)
endif
	@cat "$(SCRIPT)" | fly ssh console --app $(FLY_APP) --user petrapp \
		-C "/usr/bin/sqlite3 -readonly $(FLY_DB_PATH)"

# fly-backup snapshots the live database on the Fly machine via sqlite3 .backup, which produces
# a single consistent file (unlike a raw cp that needs DB + WAL). Stored under /data/snapshots/.
fly-backup: fly-wake
	@TS=$$(date -u +%Y%m%dT%H%M%SZ) ; \
	  REMOTE=/data/snapshots/petrapp-$(FLY_APP)-$$TS.sqlite3 ; \
	  echo "→ snapshotting $(FLY_DB_PATH) → $$REMOTE on $(FLY_APP)" ; \
	  fly ssh console --app $(FLY_APP) --user petrapp \
	    -C "/bin/sh -c 'mkdir -p /data/snapshots && /usr/bin/sqlite3 $(FLY_DB_PATH) \".backup $$REMOTE\"'" ; \
	  echo "  snapshot at $$REMOTE"

# fly-sql-write runs a SQL script that may mutate the database. Always takes a backup first.
# Pass SCRIPT=path/to/migration.sql.
fly-sql-write: fly-wake fly-backup
ifndef SCRIPT
	$(error SCRIPT is required, e.g. make fly-sql-write SCRIPT=/tmp/migration.sql)
endif
	@TS=$$(date -u +%Y%m%dT%H%M%SZ) ; \
	  REMOTE=/tmp/petrapp-write-$$TS.sql ; \
	  echo "→ uploading $(SCRIPT) → $$REMOTE on $(FLY_APP)" ; \
	  printf 'put %s %s\n' "$(SCRIPT)" "$$REMOTE" | fly ssh sftp shell --app $(FLY_APP) ; \
	  echo "→ executing on $(FLY_APP)..." ; \
	  fly ssh console --app $(FLY_APP) --user petrapp \
	    -C "/bin/sh -c '/usr/bin/sqlite3 $(FLY_DB_PATH) < $$REMOTE && rm -f $$REMOTE'"

# fly-logs prints recent logs (snapshot, no streaming). Combine with grep for filtering.
fly-logs: fly-wake
	@fly logs --app $(FLY_APP) --no-tail

# fly-pprof-cpu captures a 30s CPU profile from the running instance. Spawns the proxy as a
# background process and tears it down when the capture finishes.
fly-pprof-cpu: fly-wake
	@mkdir -p pprof
	@OUT=pprof/cpu-$(FLY_APP)-$$(date -u +%Y%m%dT%H%M%SZ).pb.gz ; \
	  echo "→ proxying 6060 → $(FLY_APP)..." ; \
	  fly proxy --app $(FLY_APP) 6060:6060 >/dev/null 2>&1 & PROXY_PID=$$! ; \
	  trap "kill $$PROXY_PID 2>/dev/null" EXIT ; \
	  sleep 2 ; \
	  echo "→ capturing 30s CPU profile..." ; \
	  curl -fsS -o $$OUT "http://localhost:6060/debug/pprof/profile?seconds=30" ; \
	  echo "  saved $$OUT (open with: go tool pprof --http=: $$OUT)"

# fly-pprof-goroutine captures a goroutine snapshot.
fly-pprof-goroutine: fly-wake
	@mkdir -p pprof
	@OUT=pprof/goroutine-$(FLY_APP)-$$(date -u +%Y%m%dT%H%M%SZ).pb.gz ; \
	  fly proxy --app $(FLY_APP) 6060:6060 >/dev/null 2>&1 & PROXY_PID=$$! ; \
	  trap "kill $$PROXY_PID 2>/dev/null" EXIT ; \
	  sleep 2 ; \
	  curl -fsS -o $$OUT "http://localhost:6060/debug/pprof/goroutine" ; \
	  echo "  saved $$OUT (open with: go tool pprof -top $$OUT)"

clean:
	@echo "Cleaning up..."
	rm -rf bin

migratetest: build
	@echo "Deleting previous restored backup..."
	rm -rf restored.sqlite3* .restored.sqlite3-litestream/
	@echo "Restoring database from backup..."
	litestream restore --config litestream.yml restored.sqlite3
	@echo "Running migration test..."
	bin/migratetest

repomix:
	npx repomix --include "**/*.go,**/*.gohtml,**/*.js,**/*.css,**/schema.sql" --output repomix-output.txt

repomix-clipboard: repomix
	cat repomix-output.txt | pbcopy

setup-git-hooks:
	./scripts/setup-git-hooks.sh

lint-fix: bin/golangci-lint
	./bin/golangci-lint run --fix
