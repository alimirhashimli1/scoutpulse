DOCKER_COMPOSE = docker compose

# Every Go module in the repo. Adding a service means adding it here and to
# go.work; nothing else in this file needs to change.
MODULES = apps/identity-svc apps/football-svc libs/auth libs/db libs/platform

.PHONY: up down build restart logs migrate test test-all lint tidy clean help

help:
	@echo "up        Start the stack (requires .env, see .env.example)"
	@echo "down      Stop the stack"
	@echo "build     Build service images"
	@echo "restart   down + up"
	@echo "logs      Follow service logs"
	@echo "migrate   Apply pending database migrations"
	@echo "test      Unit tests only (no Docker required)"
	@echo "test-all  Unit + integration tests (requires Docker)"
	@echo "lint      Run golangci-lint over every module"
	@echo "tidy      Run go mod tidy over every module"
	@echo "clean     Stop the stack and DELETE all database volumes"

## Docker
up:
	@echo "Starting ScoutPulse Micro services..."
	$(DOCKER_COMPOSE) up -d

down:
	$(DOCKER_COMPOSE) down

build:
	$(DOCKER_COMPOSE) build

restart: down up

logs:
	$(DOCKER_COMPOSE) logs -f

# Migrations run as one-shot compose jobs against a schema_migrations table,
# so this is safe to re-run and applies only what is pending.
migrate:
	$(DOCKER_COMPOSE) up migrate-identity migrate-football

## Testing & Linting
# Failures propagate: a broken module fails the target rather than printing a
# note and continuing.
test:
	@for m in $(MODULES); do \
		echo "==> $$m"; \
		(cd $$m && go test -short ./...) || exit 1; \
	done

test-all:
	@for m in $(MODULES); do \
		echo "==> $$m"; \
		(cd $$m && go test ./...) || exit 1; \
	done

lint:
	@for m in $(MODULES); do \
		echo "==> $$m"; \
		(cd $$m && golangci-lint run --config=$(CURDIR)/.golangci.yml ./...) || exit 1; \
	done

tidy:
	@for m in $(MODULES); do \
		echo "==> $$m"; \
		(cd $$m && go mod tidy) || exit 1; \
	done

## Cleanup
clean:
	@echo "This deletes all database volumes."
	$(DOCKER_COMPOSE) down -v
	@find . -type d -name "bin" -exec rm -rf {} +
	@find . -type f -name "*.exe" -delete
