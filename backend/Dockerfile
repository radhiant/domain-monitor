# syntax=docker/dockerfile:1

# ---------------------------------------------------------------------------
# Stage 1 — build the agent
#
# CGO stays off: the SQLite driver is modernc.org/sqlite, a pure-Go
# translation, so the result is one static binary with no libc to match at
# runtime. That is what lets the final image be Alpine rather than a
# distribution chosen to satisfy a linker.
# ---------------------------------------------------------------------------
FROM golang:1.25-alpine AS build

WORKDIR /src

# Manifests first so the module download layer is cached until they change.
COPY backend/go.mod backend/go.sum ./
RUN go mod download

COPY backend/ ./

# Tests run inside the build: a broken parser or classifier fails the image
# rather than shipping a dashboard that quietly monitors nothing.
RUN go test ./... && \
    CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /out/domain-monitor ./cmd/agent


# ---------------------------------------------------------------------------
# Stage 2 — run
# ---------------------------------------------------------------------------
FROM alpine:3.20

# ca-certificates for RDAP over HTTPS and for verifying the certificates of
# every monitored site; tzdata so log timestamps match the office clock.
RUN apk add --no-cache ca-certificates tzdata wget && \
    adduser -D -H -u 10001 monitor

COPY --from=build /out/domain-monitor /usr/local/bin/domain-monitor

# /data holds the SQLite history, /config the target list.
RUN mkdir -p /data /config && chown -R monitor:monitor /data

USER monitor

ENV HTTP_ADDR=:9292 \
    TARGETS_FILE=/config/list-domain.txt \
    DB_PATH=/data/domain-monitor.db

EXPOSE 9292

HEALTHCHECK --interval=30s --timeout=5s --start-period=10s --retries=3 \
  CMD wget -q --spider http://127.0.0.1:9292/api/v1/health || exit 1

ENTRYPOINT ["/usr/local/bin/domain-monitor"]
