# Makefile for Podland Backend

.PHONY: all build run clean deps lint test test-integration testhost-build migrate generate-keys init-db

# Go parameters
GOCMD=go
GOBUILD=$(GOCMD) build
GOCLEAN=$(GOCMD) clean
GOMOD=$(GOCMD) mod
BINARY_NAME=podland

# Build flags
LDFLAGS=
GCFLAGS=

# Default port
PORT=8080

# Database path
DATABASE_PATH=./podland.db

# Default admin password for initialization
ADMIN_PASSWORD=admin123

all: build

build:
	$(GOBUILD) -ldflags "$(LDFLAGS)" -gcflags "$(GCFLAGS)" -o $(BINARY_NAME) ./cmd/server

run: build
	@echo "Starting Podland Backend on port $(PORT)..."
	@echo "Database: $(DATABASE_PATH)"
	PORT=$(PORT) DATABASE_PATH=$(DATABASE_PATH) LOG_LEVEL=debug ./$(BINARY_NAME)

debug: build
	@echo "Starting Podland Backend in debug mode..."
	PORT=$(PORT) DATABASE_PATH=$(DATABASE_PATH) LOG_LEVEL=debug LOG_FORMAT=text ./$(BINARY_NAME)

clean:
	$(GOCLEAN)
	@rm -f $(BINARY_NAME)
	@rm -f $(DATABASE_PATH)
	@echo "Cleaned up"

# Dependency management
tidy:
	$(GOMOD) tidy

# Testing
test:
	$(GOCMD) test -v ./...

# Integration tests — boots the containerized podman test host (see the
# testhost-build target below), brings the backend up in-process via
# internal/app.App + httptest.NewServer, and runs the end-to-end suite
# in backend/test/integration/. Gated behind a build tag so `make test`
# / `go test ./...` never boot a container or depend on podman.
#
# Runs are serialized per host: the test host uses a fixed container
# name and a fixed published port (127.0.0.1:24422), so a second
# concurrent run fails fast instead of racing over shared volumes.
#
# Override the registered `podman system connection` name with
# PODLAND_TEST_CONNECTION_NAME (default "podland-it"). To target an
# externally managed host instead of the container (e.g. a manually
# started podman machine on macOS), set ALL of
# PODLAND_TEST_SSH_{HOST,PORT,USER,SOCKET,KEY_PATH} — the container
# lifecycle is skipped entirely.
test-integration: testhost-build
	$(GOCMD) test -v -tags=integration ./test/integration/...

# Container test host image — a podman machine stand-in built from
# test/integration/testhost/Dockerfile (pinned Fedora + podman,
# fuse-overlayfs, openssh-server, systemd; lingering `core` user with
# rootless podman behind /run/user/1000/podman/podman.sock). Layer
# caching makes repeat builds a no-op.
#
# TESTHOST_IMAGE must match testhostImage in test/integration/testhost.go
# (the harness fails fast naming this target when the image is missing).
TESTHOST_IMAGE := podland-testhost

# Manual smoke run — the run contract test/integration/testhost.go
# automates (systemd is PID 1; sshd reachable on 127.0.0.1:24422; as
# core: user manager up, /run/user/1000/podman/podman.sock listening,
# `podman run busybox` working over fuse-overlayfs):
#
#   podman run --detach --name podland-it-testhost \
#     --privileged --systemd=always --security-opt label=disable \
#     --device /dev/fuse -p 127.0.0.1:24422:22 \
#     -v podland-it-testhost-store:/home/core/.local/share/containers \
#     -v podland-it-testhost-ssh:/etc/ssh \
#     podland-testhost /sbin/init
#
# The two volumes are persistent by design: the store caches images for
# fast re-runs and the /etc/ssh volume keeps the host key stable so the
# bindings' known_hosts auto-append happens exactly once. On hosts
# without podman the build is skipped so `make test-integration` still
# reaches the suite, which skips cleanly.
testhost-build:
	@if ! command -v podman >/dev/null 2>&1; then \
		echo "podman not found — skipping test-host image build (integration suite will skip)"; \
	else \
		podman build -t $(TESTHOST_IMAGE) test/integration/testhost; \
	fi

# Run migrations
migrate:
	@echo "Running database migrations..."
	@cd migrations && go run .
	@echo "Migrations complete"

initDB:
	cd cmd/initdb && go run .

# Generate JWT keys
keys:
	chmod +x scripts/generate-keys.sh
	./scripts/generate-keys.sh

# Generate encryption key
generate-encryption-key:
	@echo "Generate a 32-byte base64-encoded key for encryption:"
	@echo "ENCRYPTION_KEY=\"$$(openssl rand -base64 32)\""

# Initialize project
init:
	mkdir -p migrations scripts
	cp .env.example .env
	./scripts/generate-keys.sh
	@echo "Project initialized. Edit .env with your settings."

# Initialize database with admin user
init-db:
	@echo "Initializing database with admin user..."
	@cd cmd/initdb && go run . -username admin -password $(ADMIN_PASSWORD) -db $(DATABASE_PATH)
	@echo "Database initialization complete"

# Docker commands
podman-build:
	podman build -t podland-backend .

podman-run:
	podman run -p $(PORT):8080 -v $(PWD)/podland.db:/app/podland.db podland-backend

# Help
help:
	@echo "Available commands:"
	@echo "  make build     - Build the application"
	@echo "  make run       - Build and run the application"
	@echo "  make debug     - Run in debug mode"
	@echo "  make clean     - Clean build artifacts"
	@echo "  make test      - Run tests"
	@echo "  make test-integration - Run integration tests against the containerized podman test host"
	@echo "  make testhost-build - Build the container test host image"
	@echo "  make migrate   - Run database migrations"
	@echo "  make keys      - Generate JWT keys"
	@echo "  make podman-build - Build Docker image"
	@echo "  make podman-run  - Run Docker container"
