# syntax=docker/dockerfile:1
# -------------------------------------------------------
#  Build stage for Go binary compilation
# -------------------------------------------------------
FROM --platform=linux/amd64 golang:1.26.4-alpine AS go-builder

# Install build dependencies if needed for any packages with C dependencies
# For a simpler build without static linking, we may not need these
RUN apk add --no-cache build-base

WORKDIR /app

# Copy Go module files first for better layer caching
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod go mod download

# Copy the source code
COPY . .

# Build the Go binary. cmd/petra/ui is embedded into the binary via //go:embed
# (see cmd/petra/assets.go), so the runtime image needs no ui/ directory and
# assets are content-fingerprinted at startup rather than by a build-time sed.
#
# The BuildKit cache mounts persist the Go build and module caches on the
# builder across builds, so a typical commit recompiles only the packages it
# touched instead of the whole dependency graph from cold (~90s -> a few s).
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    go build -o ./bin/petrapp ./cmd/petra

# -----------------------------------------------------------------------------
#  Dependency image for litestream
# -----------------------------------------------------------------------------
FROM --platform=linux/amd64 litestream/litestream:0.5.10 AS litestream

# -----------------------------------------------------------------------------
#  Final stage using Alpine
# -----------------------------------------------------------------------------
FROM --platform=linux/amd64 alpine:3.21.0

# Install necessary packages
RUN apk add --no-cache \
    ca-certificates \
    tzdata \
    sqlite

# Create non-root user
RUN adduser \
  --disabled-password \
  --gecos "" \
  --home "/nonexistent" \
  --shell "/sbin/nologin" \
  --no-create-home \
  --uid 65532 \
  petrapp

# Configure Litestream for backups to object storage
COPY /litestream.yml /etc/litestream.yml
COPY --from=litestream /usr/local/bin/litestream /dist/litestream

# Copy the compiled Go binary
COPY --from=go-builder /app/bin/petrapp /dist/petrapp

# Set environment variables
ENV TZ=Europe/Helsinki
ENV PETRAPP_ADDR=":4000"
ENV PETRAPP_PPROF_ADDR=":6060"

EXPOSE 4000 6060 9090

WORKDIR /dist

# Switch to non-root user
USER petrapp:petrapp

ENTRYPOINT [ "./litestream", "replicate", "-exec", "./petrapp" ]
