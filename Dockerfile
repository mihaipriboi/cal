# syntax=docker/dockerfile:1
# Cal — all-in-one image: web build → headless server binary → slim runtime.
# One container serves the SPA + API and runs an embedded Postgres unless
# DATABASE_URL points at an external one.

# ---- web build ----
FROM node:22-alpine AS web
WORKDIR /src
COPY package.json package-lock.json ./
COPY apps/web/package.json apps/web/package.json
COPY packages/api-client/package.json packages/api-client/package.json
# No package needs an install script on Linux (fsevents is macOS-only).
RUN npm ci --ignore-scripts
COPY packages/api-client packages/api-client
COPY apps/web apps/web
RUN npm run build -w @cal/web

# ---- server build (headless desktop target: API + SPA + embedded Postgres) ----
FROM golang:1.26 AS server
WORKDIR /src
COPY apps/api/go.mod apps/api/go.sum apps/api/
COPY apps/desktop/go.mod apps/desktop/go.sum apps/desktop/
RUN cd apps/desktop && go mod download
COPY apps/api apps/api
COPY apps/desktop apps/desktop
COPY --from=web /src/apps/web/dist apps/desktop/frontend/dist
RUN cd apps/desktop && go build -tags headless -o /out/cal .

# ---- runtime ----
FROM debian:bookworm-slim
RUN apt-get update \
 && apt-get install -y --no-install-recommends ca-certificates tzdata curl \
 && rm -rf /var/lib/apt/lists/* \
 && useradd -m -u 10001 cal \
 && mkdir -p /data && chown cal:cal /data
COPY --from=server /out/cal /usr/local/bin/cal
USER cal
ENV HOME=/home/cal DATA_DIR=/data PORT=8080

# Optional: pre-warm the embedded Postgres binary cache (downloaded from Maven
# Central) into the image so a container without DATABASE_URL needs no network
# on first boot. Off by default: with an external database (deploy/compose.yml)
# it is never used. Build with --build-arg PREWARM_EMBEDDED_PG=true to enable.
ARG PREWARM_EMBEDDED_PG=false
RUN if [ "$PREWARM_EMBEDDED_PG" = "true" ]; then \
      (cal & echo $! > /tmp/cal.pid); \
      for i in $(seq 1 300); do \
        curl -fsS http://localhost:8080/api/health >/dev/null 2>&1 && break; \
        sleep 1; \
      done; \
      kill "$(cat /tmp/cal.pid)" 2>/dev/null; \
    fi; exit 0

EXPOSE 8080
VOLUME /data
HEALTHCHECK --start-period=120s --interval=30s --timeout=5s \
  CMD curl -fsS http://localhost:8080/api/health || exit 1
CMD ["cal"]
