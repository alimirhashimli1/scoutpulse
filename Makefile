DOCKER_COMPOSE = docker compose

# Every Go module in the repo. Adding a service means adding it here and to
# go.work; `make new-service` does both for you.
MODULES = apps/identity-svc apps/football-svc libs/auth libs/db libs/platform tests/integration

.PHONY: help up down build restart logs migrate keys test test-all lint tidy fmt clean new-service observability

help:
	@echo "Setup"
	@echo "  keys         Generate a JWT key pair into .env (run once)"
	@echo ""
	@echo "Running"
	@echo "  up           Start the stack (needs .env, see .env.example)"
	@echo "  down         Stop the stack"
	@echo "  build        Build service images"
	@echo "  restart      down + up"
	@echo "  logs         Follow service logs"
	@echo "  migrate      Apply pending database migrations"
	@echo "  observability  Start Prometheus and Jaeger alongside the stack"
	@echo ""
	@echo "Development"
	@echo "  test         Unit tests only (no Docker required)"
	@echo "  test-all     Unit + integration tests (needs Docker)"
	@echo "  lint         golangci-lint over every module"
	@echo "  fmt          gofmt over every module"
	@echo "  tidy         go mod tidy over every module"
	@echo "  new-service  Scaffold a service: make new-service NAME=x PORT=8082"
	@echo ""
	@echo "  clean        Stop the stack and DELETE all database volumes"

## ------------------------------------------------------------------ setup

# Generates an RSA key pair and writes it into .env. identity-svc signs with
# the private key; every other service verifies with the public one and
# therefore cannot mint tokens.
keys:
	@if [ ! -f .env ]; then cp .env.example .env; echo "Created .env from .env.example"; fi
	@if grep -q '^JWT_PRIVATE_KEY=' .env; then \
		echo ".env already contains JWT_PRIVATE_KEY; remove it first to regenerate."; \
		exit 1; \
	fi
	@echo "Generating RSA key pair..."
	@# Generated in Go rather than by shelling out to openssl, which is absent
	@# from a default Windows install. Go is already required to build anything
	@# here, so this removes a dependency instead of adding one.
	@cd libs/auth && go run ./cmd/genkeys >> ../../.env
	@echo "Key pair written to .env"

## ----------------------------------------------------------------- docker
up:
	@echo "Starting ScoutPulse Micro..."
	$(DOCKER_COMPOSE) up -d
	@echo "Gateway on http://localhost:8000"

down:
	$(DOCKER_COMPOSE) down

build:
	$(DOCKER_COMPOSE) build

restart: down up

logs:
	$(DOCKER_COMPOSE) logs -f

observability:
	$(DOCKER_COMPOSE) --profile observability up -d
	@echo "Prometheus on http://localhost:9090, Jaeger on http://localhost:16686"

# Migrations run against a schema_migrations table, so this is safe to re-run
# and applies only what is pending.
migrate:
	$(DOCKER_COMPOSE) up migrate-identity migrate-football

## ------------------------------------------------------- testing & linting
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

fmt:
	@gofmt -w apps libs tests
	@echo "Formatted."

tidy:
	@for m in $(MODULES); do \
		echo "==> $$m"; \
		(cd $$m && go mod tidy) || exit 1; \
	done

## ---------------------------------------------------------------- scaffold
# Scaffolds a service from apps/_template so every app starts with the same
# platform wiring instead of a hand-copied approximation of the last one.
new-service:
	@if [ -z "$(NAME)" ] || [ -z "$(PORT)" ]; then \
		echo "Usage: make new-service NAME=transfer-feed PORT=8082"; \
		exit 1; \
	fi
	@if [ -d "apps/$(NAME)-svc" ]; then echo "apps/$(NAME)-svc already exists"; exit 1; fi
	@mkdir -p apps/$(NAME)-svc/internal/handler apps/$(NAME)-svc/internal/service \
	          apps/$(NAME)-svc/internal/repository apps/$(NAME)-svc/migrations
	@for f in main.go go.mod Dockerfile; do \
		sed -e 's/SERVICE_NAME/$(NAME)-svc/g' -e 's/SERVICE_PORT/$(PORT)/g' \
			apps/_template/$$f.tmpl > apps/$(NAME)-svc/$$f; \
	done
	@sed -i.bak 's#\t./libs/platform#\t./apps/$(NAME)-svc\n\t./libs/platform#' go.work && rm -f go.work.bak
	@sed -i.bak 's#^MODULES = #MODULES = apps/$(NAME)-svc #' Makefile && rm -f Makefile.bak
	@cd apps/$(NAME)-svc && go mod tidy
	@echo ""
	@echo "Created apps/$(NAME)-svc on port $(PORT)."
	@echo "Still to do:"
	@echo "  1. Add a route block to deploy/gateway/Caddyfile"
	@echo "  2. Add a scrape target to deploy/prometheus/prometheus.yml"
	@echo "  3. Add the service to docker-compose.yml"
	@echo "  4. Add apps/$(NAME)-svc to the CI matrix in .github/workflows/main.yml"

## ---------------------------------------------------------------- cleanup
clean:
	@echo "This deletes all database volumes."
	$(DOCKER_COMPOSE) down -v
	@find . -type d -name "bin" -exec rm -rf {} +
	@find . -type f -name "*.exe" -delete
