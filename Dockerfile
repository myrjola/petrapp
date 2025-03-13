# -------------------------------------------------------
#  Build stage for preparing files
# -------------------------------------------------------
FROM --platform=linux/amd64 alpine:3.21.0 AS build

WORKDIR /workspace/

# Hash CSS for cache busting and copy UI files to dist.
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
COPY --from=build /workspace/ui /dist/ui

# Configure Litestream for backups to object storage
COPY /litestream.yml /etc/litestream.yml
COPY --from=litestream /usr/local/bin/litestream /dist/litestream

# Set environment variables
ENV TZ=Europe/Helsinki
ENV PETRAPP_ADDR=":4000"
ENV PETRAPP_PPROF_ADDR=":6060"
ENV PETRAPP_TEMPLATE_PATH="/dist/ui/templates"

EXPOSE 4000 6060 9090

WORKDIR /dist

# Copy the binary
COPY /bin/petrapp.linux_amd64 petrapp

# Switch to non-root user
USER petrapp:petrapp

ENTRYPOINT [ "./litestream", "replicate", "-exec", "./petrapp" ]
