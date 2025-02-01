# -------------------------------------------------------
#  Build stage for preparing files for the scratch image.
# -------------------------------------------------------
FROM --platform=linux/amd64 alpine:3.21.0 AS build

RUN apk add --no-cache \
    # Update CA certificates
    ca-certificates \
    # Add time zone data
    tzdata

RUN adduser \
  --disabled-password \
  --gecos "" \
  --home "/nonexistent" \
  --shell "/sbin/nologin" \
  --no-create-home \
  --uid 65532 \
  petrapp

WORKDIR /workspace/

# Hash CSS for cache busting and copy UI files to dist.
COPY /ui ./ui
RUN filehash=`md5sum ./ui/static/main.css | awk '{ print $1 }'` && \
    sed -i "s/\/main.css/\/main.${filehash}.css/g" ui/templates/base.gohtml && \
    mv ./ui/static/main.css ui/static/main.${filehash}.css

# -----------------------------------------------------------------------------
#  Dependency images including binaries we can copy over to the scratch image.
# -----------------------------------------------------------------------------
FROM --platform=linux/amd64 litestream/litestream:0.3.13 AS litestream
FROM --platform=linux/amd64 keinos/sqlite3:3.47.2 AS sqlite3

# -----------------------------------------------------------------------------
#  Main stage for copying files over to the scratch image.
# -----------------------------------------------------------------------------
FROM --platform=linux/amd64 scratch

COPY --from=build /usr/share/zoneinfo /usr/share/zoneinfo
COPY --from=build /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY --from=build /etc/passwd /etc/passwd
COPY --from=build /etc/group /etc/group
COPY --from=build /workspace/ui /dist/ui

# Configure Litestream for backups to object storage.
COPY /litestream.yml /etc/litestream.yml
COPY --from=litestream /usr/local/bin/litestream /dist/litestream

# Copy sqlite3 binary for database operations with `make fly-sqlite3` command.
COPY --from=sqlite3 /usr/bin/sqlite3 /usr/bin/sqlite3
COPY --from=sqlite3 /usr/lib/libz.so.1 /usr/lib/libz.so.1
COPY --from=sqlite3 /lib/ld-musl-x86_64.so.1 /lib/ld-musl-x86_64.so.1

USER petrapp:petrapp

ENV TZ=Europe/Helsinki
ENV PETRAPP_ADDR=":4000"
# pprof only available from internal network
ENV PETRAPP_PPROF_ADDR=":6060"
ENV PETRAPP_TEMPLATE_PATH="/dist/ui/templates"

EXPOSE 4000 6060 9090

WORKDIR /dist

# Copy the cross-compiled binary created with 'make cross-compile' or other means.
COPY /bin/petrapp.linux_amd64 petrapp

ENTRYPOINT [ "./litestream", "replicate", "-exec", "./petrapp" ]
