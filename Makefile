SHELL := /bin/bash

API_DIR := apps/api
WEB_DIR := apps/web
COMPOSE := docker compose --env-file infra/.env -f infra/docker-compose.yml

# Exports variables from apps/api/.env (if present) into the current shell.
LOAD_API_ENV := set -a; [ -f .env ] && . ./.env; set +a

.DEFAULT_GOAL := help

.PHONY: help
help: ## Show available commands
	@grep -E '^[a-zA-Z_-]+:.*?## ' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "  %-18s %s\n", $$1, $$2}'

## Database (local Docker, optional) ------------------------------------------
.PHONY: db-up db-down db-logs
db-up: ## Start local PostgreSQL in Docker
	$(COMPOSE) up -d
db-down: ## Stop local PostgreSQL (data is kept)
	$(COMPOSE) down
db-logs: ## Tail PostgreSQL logs
	$(COMPOSE) logs -f postgres

## API ------------------------------------------------------------------------
.PHONY: api-tidy api-run api-test api-vet api-fmt api-fmt-check migrate-up migrate-status
api-tidy: ## Resolve Go dependencies (creates go.sum)
	cd $(API_DIR) && go mod tidy
api-run: ## Run the API (reads apps/api/.env)
	cd $(API_DIR) && $(LOAD_API_ENV) && go run ./cmd/api
api-test: ## Run Go tests
	cd $(API_DIR) && go test ./...
api-vet: ## Run go vet
	cd $(API_DIR) && go vet ./...
api-fmt: ## Format Go code
	cd $(API_DIR) && gofmt -w .
api-fmt-check: ## Fail if Go code is not formatted
	@cd $(API_DIR) && out="$$(gofmt -l .)"; if [ -n "$$out" ]; then echo "Run 'make api-fmt'. Unformatted:"; echo "$$out"; exit 1; fi
migrate-up: ## Apply pending database migrations
	cd $(API_DIR) && $(LOAD_API_ENV) && go run ./cmd/migrate up
migrate-status: ## Show which migrations are applied
	cd $(API_DIR) && $(LOAD_API_ENV) && go run ./cmd/migrate status

## Web ------------------------------------------------------------------------
.PHONY: web-install web-dev web-build web-lint web-typecheck web-test web-fmt web-fmt-check
web-install: ## Install web dependencies
	cd $(WEB_DIR) && npm install
web-dev: ## Run the dashboard dev server
	cd $(WEB_DIR) && npm run dev
web-build: ## Production build of the dashboard
	cd $(WEB_DIR) && npm run build
web-lint: ## Lint the dashboard
	cd $(WEB_DIR) && npm run lint
web-typecheck: ## Type-check the dashboard
	cd $(WEB_DIR) && npm run typecheck
web-test: ## Run dashboard tests
	cd $(WEB_DIR) && npm test
web-fmt: ## Format the dashboard code
	cd $(WEB_DIR) && npm run format
web-fmt-check: ## Fail if dashboard code is not formatted
	cd $(WEB_DIR) && npm run format:check

## Everything -----------------------------------------------------------------
.PHONY: check
check: api-fmt-check api-vet api-test web-fmt-check web-lint web-typecheck web-test ## Run every format, lint, type, and test check
