COMPOSE ?= docker compose
ENV_FILE ?= .env
PROJECT ?= ov-dash

.DEFAULT_GOAL := help

.PHONY: help env backend-tidy backend-test backend-api backend-worker frontend-install frontend-dev frontend-build compose-config compose-build compose-up compose-up-backend compose-down compose-restart compose-logs compose-ps api-logs worker-logs db-logs redis-logs db-shell redis-cli backup-db restore-db clean

help:
	@printf '%s\n' \
		'ov-dash targets:' \
		'  make env                 Create .env from .env.example when missing' \
		'  make backend-test        Run Go backend tests' \
		'  make frontend-build      Build shadcn-admin frontend locally' \
		'  make compose-config      Validate Docker Compose configuration' \
		'  make compose-build       Build all service images' \
		'  make compose-up          Start full stack' \
		'  make compose-up-backend  Start PostgreSQL, Redis, API, and worker only' \
		'  make compose-logs        Follow all service logs' \
		'  make backup-db           Dump PostgreSQL into deploy/backups/' \
		'  make clean               Stop stack and remove local Compose volumes'

env:
	@if [ ! -f "$(ENV_FILE)" ]; then cp .env.example "$(ENV_FILE)"; echo "Created $(ENV_FILE). Edit secrets before deploying."; else echo "$(ENV_FILE) already exists."; fi

backend-tidy:
	cd backend && go mod tidy

backend-test:
	cd backend && go test ./...

backend-api:
	cd backend && go run ./cmd/api

backend-worker:
	cd backend && go run ./cmd/worker

frontend-install:
	cd frontend && pnpm install

frontend-dev:
	cd frontend && pnpm dev

frontend-build:
	cd frontend && pnpm build

compose-config:
	$(COMPOSE) --env-file $(ENV_FILE) config

compose-build:
	$(COMPOSE) --env-file $(ENV_FILE) build

compose-up:
	$(COMPOSE) --env-file $(ENV_FILE) up -d --build

compose-up-backend:
	$(COMPOSE) --env-file $(ENV_FILE) up -d --build postgres redis api worker

compose-down:
	$(COMPOSE) --env-file $(ENV_FILE) down

compose-restart:
	$(COMPOSE) --env-file $(ENV_FILE) restart

compose-logs:
	$(COMPOSE) --env-file $(ENV_FILE) logs -f --tail=200

compose-ps:
	$(COMPOSE) --env-file $(ENV_FILE) ps

api-logs:
	$(COMPOSE) --env-file $(ENV_FILE) logs -f --tail=200 api

worker-logs:
	$(COMPOSE) --env-file $(ENV_FILE) logs -f --tail=200 worker

db-logs:
	$(COMPOSE) --env-file $(ENV_FILE) logs -f --tail=200 postgres

redis-logs:
	$(COMPOSE) --env-file $(ENV_FILE) logs -f --tail=200 redis

db-shell:
	$(COMPOSE) --env-file $(ENV_FILE) exec postgres sh -lc 'psql -U "$$POSTGRES_USER" -d "$$POSTGRES_DB"'

redis-cli:
	$(COMPOSE) --env-file $(ENV_FILE) exec redis sh -lc 'redis-cli -a "$$REDIS_PASSWORD"'

backup-db:
	@mkdir -p deploy/backups
	$(COMPOSE) --env-file $(ENV_FILE) exec -T postgres sh -lc 'pg_dump -U "$$POSTGRES_USER" -d "$$POSTGRES_DB"' > deploy/backups/$(PROJECT)-$$(date +%Y%m%d%H%M%S).sql

restore-db:
	@test -n "$(FILE)" || (echo "Usage: make restore-db FILE=deploy/backups/backup.sql" && exit 1)
	$(COMPOSE) --env-file $(ENV_FILE) exec -T postgres sh -lc 'psql -U "$$POSTGRES_USER" -d "$$POSTGRES_DB"' < "$(FILE)"

clean:
	$(COMPOSE) --env-file $(ENV_FILE) down -v --remove-orphans
