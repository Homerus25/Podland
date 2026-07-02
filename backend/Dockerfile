# Dockerfile for Podland Backend
# Multi-stage build for smaller final image

# Build stage
FROM golang:1.21-alpine AS builder

WORKDIR /app

# Install dependencies
RUN apk add --no-cache git ca-certificates

# Copy go mod files
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Build the application
RUN CGO_ENABLED=0 GOOS=linux go build -o /podland ./cmd/server

# Final stage
FROM alpine:latest

WORKDIR /app

# Install ca-certificates for HTTPS
RUN apk --no-cache add ca-certificates tzdata

# Copy binary from builder
COPY --from=builder /podland /app/podland

# Copy migrations
COPY --from=builder /app/migrations ./migrations

# Copy default config
COPY --from=builder /app/.env.example .env.example

# Create data directory
RUN mkdir -p /app/data

# Set permissions
RUN chown -R nobody:nogroup /app
USER nobody:nogroup

# Expose port
EXPOSE 8080

# Health check
HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
    CMD wget --no-verbose --tries=1 --spider http://localhost:8080/api/health || exit 1

# Run the application
CMD ["/app/podland"]
