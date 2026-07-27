# Makefile for Podland Backend

.PHONY: all build run clean deps lint test migrate generate-keys init-db

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

# Run migrations
migrate:
	@echo "Running database migrations..."
	@cd migrations && go run .
	@echo "Migrations complete"

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
docker-build:
	docker build -t podland-backend .

docker-run:
	docker run -p $(PORT):8080 -v $(PWD)/podland.db:/app/podland.db podland-backend

# Help
help:
	@echo "Available commands:"
	@echo "  make build     - Build the application"
	@echo "  make run       - Build and run the application"
	@echo "  make debug     - Run in debug mode"
	@echo "  make clean     - Clean build artifacts"
	@echo "  make test      - Run tests"
	@echo "  make migrate   - Run database migrations"
	@echo "  make keys      - Generate JWT keys"
	@echo "  make docker-build - Build Docker image"
	@echo "  make docker-run  - Run Docker container"
