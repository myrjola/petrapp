# -------------------------------------------------------
#  Build stage for Go binary compilation
# -------------------------------------------------------
FROM --platform=linux/amd64 golang:1.25.0-alpine AS go-builder

# Install build dependencies if needed for any packages with C dependencies
# For a simpler build without static linking, we may not need these
RUN apk add --no-cache build-base

WORKDIR /app

# Copy Go module files first for better layer caching
COPY go.mod go.sum ./
RUN go mod download

# Copy the source code
COPY . .

# Build the Go binary
RUN go build -o ./bin/petrapp ./cmd/web

# -------------------------------------------------------
#  Build stage for preparing UI files
# -------------------------------------------------------
FROM --platform=linux/amd64 alpine:3.21.0 AS ui-builder

WORKDIR /workspace/

# Hash CSS for cache busting and copy UI files to dist
COPY /ui ./ui
RUN filehash=`md5sum ./ui/static/main.css | awk '{ print $1 }'` && \
    sed -i "s/\/main.css/\/main.${filehash}.css/g" ui/templates/base.gohtml && \
    mv ./ui/static/main.css ui/static/main.${filehash}.css

# -----------------------------------------------------------------------------
#  Dependency image for litestream
# -----------------------------------------------------------------------------
FROM --platform=linux/amd64 litestream/litestream:0.3.13 AS litestream

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

# Copy UI files
COPY --from=ui-builder /workspace/ui /dist/ui

# Configure Litestream for backups to object storage
COPY /litestream.yml /etc/litestream.yml
COPY --from=litestream /usr/local/bin/litestream /dist/litestream

# Copy the compiled Go binary
COPY --from=go-builder /app/bin/petrapp /dist/petrapp

# Set environment variables
ENV TZ=Europe/Helsinki
ENV PETRAPP_ADDR=":4000"
ENV PETRAPP_PPROF_ADDR=":6060"
ENV PETRAPP_TEMPLATE_PATH="/dist/ui/templates"

EXPOSE 4000 6060 9090

WORKDIR /dist

# Switch to non-root user
USER petrapp:petrapp

ENTRYPOINT [ "./litestream", "replicate", "-exec", "./petrapp" ]
