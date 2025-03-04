
.DEFAULT_GOAL := ci
.PHONY: ci gomod init build test dev lint build-docker fly-sqlite3 clean sec \
        cross-compile migratetest repomix repomix-clipboard setup-git-hooks

export GOTOOLCHAIN := auto
GOLANGCI_LINT_VERSION := v1.64.6

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
	go install golang.org/x/vuln/cmd/govulncheck@latest
	govulncheck -show verbose ./...

ci: init build lint test sec

build:
	@echo "Building..."
	go build -o bin/petrapp github.com/myrjola/petrapp/cmd/web
	go build -o bin/smoketest github.com/myrjola/petrapp/cmd/smoketest
	go build -o bin/migratetest github.com/myrjola/petrapp/cmd/migratetest

test:
	@echo "Running tests..."
	go test --race ./...

lint:
	@echo "Running linter..."
	./custom-gcl run

dev:
	@echo "Running dev server with debug build..."
	go build -gcflags="all=-N -l" -o bin/petrapp github.com/myrjola/petrapp/cmd/web
	./bin/petrapp

cross-compile:
	@echo "Cross-compiling..."
	docker build --tag petrapp-bin --file cross-compile.Dockerfile .
	docker create --name petrapp-bin-extract petrapp-bin
	docker cp petrapp-bin-extract:/dist/petrapp.linux_amd64 ./bin/petrapp.linux_amd64
	docker rm petrapp-bin-extract

build-docker:
	@echo "Building Docker image..."
	docker build --tag petrapp .

fly-sqlite3:
	@echo "Connecting to sqlite3 database on deployed Fly machine"
	fly ssh console --pty --user petrapp -C "/usr/bin/sqlite3 /data/petrapp.sqlite3"

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
	npx repomix --include "**/*.go,**/*.gohtml,**/*.js,**/*.css,**/*.sql" --output repomix-output.txt

repomix-clipboard: repomix
	cat repomix-output.txt | pbcopy

setup-git-hooks:
	./scripts/setup-git-hooks.sh
