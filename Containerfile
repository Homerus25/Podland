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
RUN sed -i 's|^replace go.podman.io/podman/v6 => .*|replace go.podman.io/podman/v6 => /srv/podman|' go.mod && go mod download
COPY --from=backend . .
RUN sed -i 's|^replace go.podman.io/podman/v6 => .*|replace go.podman.io/podman/v6 => /srv/podman|' go.mod && rm -rf internal/webui/dist
COPY --from=ui /ui/dist ./internal/webui/dist/
RUN CGO_ENABLED=1 GOOS=linux go build -o /podland ./cmd/server
RUN cd ./cmd/initdb && go run . -username admin -password admin123 -db /podland.db

# --- Stage 3: runtime -------------------------------------------------------
FROM alpine:latest
RUN apk --no-cache add ca-certificates tzdata gpgme-dev openssl
WORKDIR /app
COPY --from=builder /podland ./podland
COPY --from=builder /src/migrations ./migrations
COPY --from=builder /src/.env.example .env.example
COPY --from=builder /src/scripts/generate-keys.sh ./scripts/generate-keys.sh
COPY --from=builder /podland.db ./podland.db 
RUN mkdir -p /app/data /.ssh
EXPOSE 8080
HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
    CMD wget --no-verbose --tries=1 --spider http://localhost:8080/api/health || exit 1
# A fresh boot generates the JWT signing keys unless they are mounted in;
# the SQLite database is created and migrated on first start.
ENTRYPOINT ["/bin/sh", "-c", "[ -f jwt-private.pem ] || ./scripts/generate-keys.sh; exec ./podland"]
