
.DEFAULT_GOAL := ci
.PHONY: ci gomod init build test dev lint build-docker fly-sqlite3 clean sec \
        migratetest repomix repomix-clipboard setup-git-hooks deploy-alloy deploy-logshipper

export GOTOOLCHAIN := auto
GOLANGCI_LINT_VERSION := v1.64.6

# Suppress linker warnings on macOS
ifeq ($(shell uname -s),Darwin)
	export CGO_LDFLAGS := -w
endif

init: gomod custom-gcl setup-git-hooks
	@echo "Dependencies installed"

gomod:
	@echo "Installing Go dependencies..."
	go version
	go mod download
	go mod verify

custom-gcl:
	@echo "Installing golangci-lint and building custom version for nilaway plugin to ./custom-gcl"
	curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh | sh -s $(GOLANGCI_LINT_VERSION)
	bin/golangci-lint custom

sec:
	go tool govulncheck ./...

ci: init build lint test sec

build:
	@echo "Building..."
	go build -o bin/petrapp github.com/myrjola/petrapp/cmd/web
	go build -o bin/smoketest github.com/myrjola/petrapp/cmd/smoketest
	go build -o bin/migratetest github.com/myrjola/petrapp/cmd/migratetest
	go build -o bin/stresstest github.com/myrjola/petrapp/cmd/stresstest

test:
	@echo "Running tests..."
	go test --race --shuffle=on ./...

lint:
	@echo "Running linter..."
	./custom-gcl run

dev:
	@echo "Running dev server with debug build..."
	go build -gcflags="all=-N -l" -o bin/petrapp github.com/myrjola/petrapp/cmd/web
	./bin/petrapp

build-docker:
	@echo "Building Docker image..."
	docker build --tag petrapp .

fly-sqlite3:
	@echo "Connecting to sqlite3 database on deployed Fly machine"
	fly ssh console --pty --user petrapp -C "/usr/bin/sqlite3 -cmd \"PRAGMA foreign_keys = ON;\" /data/petrapp.sqlite3"

clean:
	@echo "Cleaning up..."
	rm -rf bin
	rm -rf custom-gcl

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
