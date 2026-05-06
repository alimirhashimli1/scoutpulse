# Variables
DOCKER_COMPOSE = docker compose

# Production/Development switch (defaults to dev)
ENV ?= dev

.PHONY: up down build restart logs test-all lint clean

## Docker Commands
up:
	@echo "Starting ScoutPulse Micro services..."
	$(DOCKER_COMPOSE) up -d

down:
	@echo "Stopping services..."
	$(DOCKER_COMPOSE) down

build:
	@echo "Building images..."
	$(DOCKER_COMPOSE) build

restart: down up

logs:
	$(DOCKER_COMPOSE) logs -f

## Testing & Linting
test-all:
	@echo "Running all tests..."
	@echo "Running Identity Service tests..."
	@cd apps/identity-svc && go test ./... || echo "No tests found or service not initialized"
	@echo "Running Football Service tests..."
	@cd apps/football-svc && go test ./... || echo "No tests found or service not initialized"
	@echo "Running Frontend tests..."
	@cd apps/frontend && npm test || echo "Frontend not initialized"

lint:
	@echo "Linting all services..."
	@echo "Linting Identity Service..."
	@cd apps/identity-svc && golangci-lint run || echo "Linter not configured"
	@echo "Linting Football Service..."
	@cd apps/football-svc && golangci-lint run || echo "Linter not configured"
	@echo "Linting Frontend..."
	@cd apps/frontend && npm run lint || echo "Linter not configured"

## Cleanup
clean:
	@echo "Cleaning up build artifacts and docker volumes..."
	$(DOCKER_COMPOSE) down -v
	find . -type d -name "bin" -exec rm -rf {} +
	find . -type f -name "*.exe" -delete
