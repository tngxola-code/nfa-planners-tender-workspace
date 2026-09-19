SHELL := /usr/bin/env bash
.DEFAULT_GOAL := help

help:  ## Show this help
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-30s\033[0m %s\n", $$1, $$2}'

# ---------------------------------------------------------------- bootstrap

bootstrap:  ## Install toolchain and dependencies
	cd backend && go mod download
	cd console && pnpm install --frozen-lockfile
	cd mobile && flutter pub get

init:  ## Create .env and local certs
	[ -f .env ] || cp .env.example .env
	@mkdir -p deploy/caddy/certs
	@echo "created .env"

# ---------------------------------------------------------------- local stack

up:  ## Start the local stack
	docker compose up -d

down:  ## Stop the local stack
	docker compose down

clean:  ## Stop and drop volumes
	docker compose down -v

ps:  ## Show container status
	docker compose ps

logs:  ## Tail logs
	docker compose logs -f --tail=100

migrate:  ## Apply migrations
	cd backend && goose -dir migrations postgres "$$DATABASE_URL" up

seed:  ## Load synthetic reference data
	cd backend && go run ./cmd/migrate seed

# ---------------------------------------------------------------- run

run-api:  ## Run the API
	cd backend && go run ./cmd/api

run-worker:  ## Run the worker
	cd backend && go run ./cmd/worker

run-console:  ## Run the console dev server
	cd console && pnpm dev

# ---------------------------------------------------------------- generate

generate:  ## Regenerate code from contract and schema
	cd backend && sqlc generate
	cd backend && go generate ./internal/api/...
	cd console && pnpm exec openapi-typescript ../../api/openapi.yaml -o src/lib/api/schema.d.ts
	cd mobile && dart run build_runner build --delete-conflicting-outputs
	$(MAKE) postman

sync-openapi:  ## Copy the contract into the embed path
	cp api/openapi.yaml backend/internal/httpx/openapi.yaml

postman:  ## Regenerate the Postman collection
	@mkdir -p tools/postman
	npx openapi-to-postmanv2 -s api/openapi.yaml -o tools/postman/collection.json -p

diagrams:  ## Re-run diagram generators
	cd docs/img && python3 request-path.py . && python3 trust-zones.py . && python3 data-layers.py .

# ---------------------------------------------------------------- tests

test-all: test-backend test-console test-mobile contract  ## Run every suite

test-backend: test-backend-unit test-backend-integration test-backend-contract

test-backend-unit:  ## Backend unit tests
	cd backend && go test -race -short ./...

test-backend-integration:  ## Backend integration tests
	cd backend && go test -race -tags=integration ./tests/integration/...

test-backend-contract:  ## Backend contract test
	cd backend && go test -tags=contract ./tests/contract/...

test-console: test-console-unit test-console-integration test-console-e2e

test-console-unit:  ## Console unit tests
	cd console && pnpm test:unit -- run

test-console-integration:  ## Console integration tests
	cd console && pnpm test:integration -- run

test-console-e2e:  ## Console end-to-end tests
	cd console && pnpm test:e2e

test-mobile: test-mobile-unit test-mobile-integration

test-mobile-unit:  ## Mobile unit and widget tests
	cd mobile && flutter test test/unit

test-mobile-integration:  ## Mobile integration tests
	cd mobile && flutter test integration_test

contract:  ## Newman contract run against the local stack
	newman run tools/postman/collection.json --bail

load-test:  ## k6 baseline
	k6 run tools/k6/baseline.js

dast:  ## OWASP ZAP baseline
	docker run --rm -t -v $$PWD:/zap/wrk:rw \
	  ghcr.io/zaproxy/zaproxy:stable zap-baseline.py \
	  -t http://host.docker.internal:8000 -r report.html

lint:  ## Run every linter
	cd backend && golangci-lint run ./...
	cd console && pnpm exec tsc --noEmit && pnpm exec eslint .
	cd mobile && flutter analyze
	spectral lint api/openapi.yaml --fail-severity=warn

# ---------------------------------------------------------------- build

build:  ## Build all binaries and bundles
	cd backend && go build -o bin/api ./cmd/api
	cd backend && go build -o bin/worker ./cmd/worker
	cd backend && go build -o bin/migrate ./cmd/migrate
	cd console && pnpm build

docker:  ## Build container images
	docker build -t nfa/api:$$(git rev-parse --short HEAD) -f backend/Dockerfile backend

# ---------------------------------------------------------------- quality

check-fast:  ## Pre-commit hooks over the whole tree
	lefthook run pre-commit --all-files

check-full:  ## Pre-push hooks without pushing
	lefthook run pre-push

check: check-fast check-full  ## Run all local quality gates

# ---------------------------------------------------------------- keycloak

keycloak-reset:  ## Drop the Keycloak database and re-import the realm
	@echo "dropping keycloak database..."
	@docker compose exec -T postgres psql -U nfa -d postgres \
		-c "DROP DATABASE IF EXISTS keycloak WITH (FORCE);" >/dev/null
	@docker compose exec -T postgres psql -U nfa -d postgres \
		-c "CREATE DATABASE keycloak OWNER nfa;" >/dev/null
	@echo "restarting keycloak..."
	docker compose restart keycloak
	@echo "waiting for realm to load..."
	@until curl -sf http://localhost:8081/realms/nfa/.well-known/openid-configuration >/dev/null 2>&1; do sleep 2; done
	@echo "realm loaded"

keycloak-verify:  ## Verify the realm is loaded and scopes behave
	./deploy/keycloak/verify.sh

# ---------------------------------------------------------------- secrets

secrets-edit:  ## Open the SOPS file (ENV=uat|prod)
	[ -n "$(ENV)" ] || { echo "usage: make secrets-edit ENV=uat"; exit 1; }
	sops deploy/secrets/$(ENV).enc.yaml

ports:  ## Print the local port map
	@echo "Caddy      http://localhost:8080  https://localhost:8443"
	@echo "API        http://localhost:8000"
	@echo "Console    http://localhost:3000"
	@echo "Keycloak   http://localhost:8081"
	@echo "Postgres   localhost:5432"
	@echo "OpenSearch http://localhost:9200"
	@echo "MinIO      http://localhost:9000  console http://localhost:9001"
	@echo "Redis      localhost:6379"
	@echo "Mailpit    http://localhost:8025"

.PHONY: help bootstrap init up down clean ps logs migrate seed \
        run-api run-worker run-console generate sync-openapi postman diagrams \
        test-all test-backend test-backend-unit test-backend-integration test-backend-contract \
        test-console test-console-unit test-console-integration test-console-e2e \
        test-mobile test-mobile-unit test-mobile-integration \
        contract load-test dast lint build docker check-fast check-full check \
        keycloak-reset keycloak-verify \
        secrets-edit ports
