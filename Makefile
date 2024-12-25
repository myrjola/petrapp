.PHONY: init test lint

init:
	@echo "Installing Go dependencies..."
	go mod tidy
	go mod download

	@echo "Installing golangci-lint and building custom version for nilaway plugin to ./custom-gcl"
	curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh | sh -s v1.62.2
	golangci-lint custom

	@echo "Dependencies installed successfully."

test:
	@echo "Running tests..."
	go test --race ./...

lint:
	@echo "Running linter..."
	./custom-gcl run

dev:
	@echo "Running dev server..."
	go run github.com/myrjola/sheerluck/cmd/web
