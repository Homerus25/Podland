# Dockerfile for Podland Backend
# Multi-stage build for smaller final image

# Build stage
FROM golang:1.26-alpine AS builder

WORKDIR /app

# Install dependencies
RUN apk add --no-cache git ca-certificates gcc musl-dev build-base linux-headers pkgconf linux-stable-dev gpgme-dev btrfs-progs-dev

# Copy go mod files
COPY go.mod go.sum ./
COPY --from=podman . /home/homerus/repos/podman/
RUN go mod download

# Copy source code
COPY . .

# Build the application
RUN CGO_ENABLED=1 GOOS=linux  go build -o /podland ./cmd/server


# Final stage
FROM alpine:latest

WORKDIR /app

# Install ca-certificates for HTTPS
RUN apk --no-cache add ca-certificates tzdata
RUN apk --no-cache add gpgme-dev openssl

# Copy binary from builder
COPY --from=builder /podland /app/podland

# Copy migrations
COPY --from=builder /app/migrations ./migrations

# Copy default config
COPY --from=builder /app/.env.example .env.example


RUN mkdir -p /app/scripts

COPY ./scripts/generate-keys.sh /app/scripts/generate-keys.sh
RUN /app/scripts/generate-keys.sh

# TODO: build db on startup
# TODO: change dir for volume
COPY ./podland.db /app/podland.db

# Create data directory
RUN mkdir -p /app/data

RUN mkdir -p /.ssh

EXPOSE 8080

HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
    CMD wget --no-verbose --tries=1 --spider http://localhost:8080/api/health || exit 1

CMD ["/app/podland"]
