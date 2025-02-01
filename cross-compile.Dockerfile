# --------------------------------------------------------------------------------
#  Build stage for compiling the binary and preparing files for the scratch image.
# --------------------------------------------------------------------------------
FROM --platform=linux/amd64 golang:1.23.4-alpine3.21

ENV CGO_ENABLED=1
ENV GOOS=linux
ENV GOARCH=amd64
ENV GOCACHE=/workspace/.cache

RUN apk add --no-cache \
    # Important: required for mattn/go-sqlite3
    gcc \
    musl-dev

WORKDIR /workspace/

# Copy the source code.
COPY /go.mod .
COPY /go.sum .
COPY /cmd ./cmd
COPY /internal ./internal

# Download dependencies.
RUN go mod download
RUN go mod verify

# Build a statically linked binary.
RUN go build -ldflags='-s -w -extldflags "-static"' -o /dist/petrapp.linux_amd64 ./cmd/web
