# One-container Podland image: the Go backend binary with the compiled
# React SPA embedded, serving UI and API on :8080. Everything is sourced
# from named contexts — a bare repo checkout alone is not enough:
#
#   podman build \
#     --build-context backend=backend \
#     --build-context frontend=frontend \
#     --build-context podman=<local podman fork> \
#     -t podland .
#
# `podman` is the patched fork of Podman's Go bindings that
# backend/go.mod carries as a replace directive.

# --- Stage 1: frontend production build -----------------------------------
FROM node:22-alpine AS ui
WORKDIR /ui
COPY --from=frontend package.json package-lock.json ./
RUN npm ci
COPY --from=frontend index.html vite.config.ts tsconfig.json tsconfig.node.json ./
COPY --from=frontend src ./src
RUN npm run build

# --- Stage 2: backend build, SPA embedded ---------------------------------
FROM golang:1.26-alpine AS builder
RUN apk add --no-cache git ca-certificates gcc musl-dev build-base linux-headers pkgconf linux-stable-dev gpgme-dev btrfs-progs-dev
WORKDIR /src
COPY --from=backend go.mod go.sum ./
COPY --from=podman . /srv/podman
# Checkouts disagree on where the fork lives (host clone vs devbox), so
# the replace directive is normalized to one in-image path — here for
# the dependency layer, and again once the full backend tree re-copies
# go.mod below.
# Cache mounts keep the module and compiler caches in the builder's storage
# across builds, so a source change recompiles incrementally instead of from
# scratch. The module cache must be mounted on both RUN lines: modules
# downloaded into the mount are invisible to later steps without it.
RUN --mount=type=cache,target=/go/pkg/mod \
    sed -i 's|^replace go.podman.io/podman/v6 => .*|replace go.podman.io/podman/v6 => /srv/podman|' go.mod && go mod download
COPY --from=backend . .
RUN sed -i 's|^replace go.podman.io/podman/v6 => .*|replace go.podman.io/podman/v6 => /srv/podman|' go.mod && rm -rf internal/webui/dist
COPY --from=ui /ui/dist ./internal/webui/dist/
RUN --mount=type=cache,target=/go/pkg/mod --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=1 GOOS=linux go build -o /podland ./cmd/server

# --- Stage 3: runtime -------------------------------------------------------
FROM alpine:latest
# Pinned podlet release for compose-to-quadlet conversion (spec #336);
# override at build time with --build-arg PODLET_VERSION=<version>.
ARG PODLET_VERSION=0.3.2
RUN apk --no-cache add ca-certificates tzdata gpgme-dev openssl xz tar
# Prebuilt x86_64 musl binary from the containers/podlet GitHub release,
# verified against the published .sha256 before install. The published
# file ends in a blank line that busybox sha256sum -c rejects, hence the
# grep -v.
RUN wget -q -O /tmp/podlet-x86_64-unknown-linux-musl.tar.xz "https://github.com/containers/podlet/releases/download/v${PODLET_VERSION}/podlet-x86_64-unknown-linux-musl.tar.xz" \
    && wget -q -O /tmp/podlet-x86_64-unknown-linux-musl.tar.xz.sha256 "https://github.com/containers/podlet/releases/download/v${PODLET_VERSION}/podlet-x86_64-unknown-linux-musl.tar.xz.sha256" \
    && (cd /tmp && grep -v '^$' podlet-x86_64-unknown-linux-musl.tar.xz.sha256 | sha256sum -c -) \
    && tar -xJf /tmp/podlet-x86_64-unknown-linux-musl.tar.xz -C /tmp \
    && install -m 0755 /tmp/podlet-x86_64-unknown-linux-musl/podlet /usr/local/bin/podlet \
    && rm -rf /tmp/podlet-x86_64-unknown-linux-musl /tmp/podlet-x86_64-unknown-linux-musl.tar.xz /tmp/podlet-x86_64-unknown-linux-musl.tar.xz.sha256
WORKDIR /app
COPY --from=builder /podland ./podland
COPY --from=builder /src/migrations ./migrations
COPY --from=builder /src/.env.example .env.example
COPY --from=builder /src/scripts/generate-keys.sh ./scripts/generate-keys.sh
RUN mkdir -p /app/data /.ssh
# The database lives in /app/data so a created admin (and every other
# row) survives container recreation when that directory is mounted as
# a volume: podman run -v podland-data:/app/data ...
ENV DATABASE_PATH=/app/data/podland.db
EXPOSE 8080
HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
    CMD wget --no-verbose --tries=1 --spider http://localhost:8080/api/health || exit 1
# A fresh image ships no database and no credentials: first boot creates
# and migrates the SQLite database, generates the JWT signing keys unless
# they are mounted in, and serves the setup landing page — the first
# admin is created through the UI (POST /api/setup/user).
ENTRYPOINT ["/bin/sh", "-c", "[ -f jwt-private.pem ] || ./scripts/generate-keys.sh; exec ./podland"]
