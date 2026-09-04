SHELL := /bin/sh

.PHONY: bootstrap dev-setup dev-prepare dev-env-update dev-infra-up dev-infra-down dev-infra-reset dev-reset api worker frontend hash-password \
	migrate-up migrate-down-one migrate-status fmt lint test build image compose-check \
	prod-config-check prod-init prod-migrate prod-up prod-ps prod-logs

IMAGE ?= game-gen:dev
APP_VERSION ?= dev
VCS_REF ?= unknown
BUILD_DATE ?= unknown
PROD_ENV ?= deploy/compose/prod/.env
PROD_COMPOSE = docker compose --env-file $(PROD_ENV) -f deploy/compose/prod/compose.yaml

bootstrap:
	cd backend && go mod download
	cd frontend && npm ci

dev-setup:
	bash scripts/setup-dev.sh

dev-prepare:
	bash scripts/prepare-dev.sh

dev-env-update:
	bash scripts/prepare-dev.sh --pull

dev-infra-up:
	docker compose --env-file deploy/compose/dev/.env -f deploy/compose/dev/compose.yaml up -d --wait
	docker compose --env-file deploy/compose/dev/.env -f deploy/compose/dev/compose.yaml --profile setup run --rm minio-init

dev-infra-down:
	docker compose --env-file deploy/compose/dev/.env -f deploy/compose/dev/compose.yaml down

dev-infra-reset:
	docker compose --env-file deploy/compose/dev/.env -f deploy/compose/dev/compose.yaml down --volumes

dev-reset:
	bash scripts/reset-dev.sh

api:
	set -a; . ./.env; set +a; cd backend && go run ./cmd/api

worker:
	set -a; . ./.env; set +a; cd backend && go run ./cmd/worker

frontend:
	cd frontend && npm run dev

hash-password:
	@test -n "$(PASSWORD)" || (echo "usage: make hash-password PASSWORD='your-password'" >&2; exit 2)
	@cd backend && go run ./cmd/hash-password "$(PASSWORD)"

migrate-up:
	set -a; . ./.env; set +a; cd backend && go run ./cmd/migrate -command up

migrate-down-one:
	set -a; . ./.env; set +a; cd backend && go run ./cmd/migrate -command down-one

migrate-status:
	set -a; . ./.env; set +a; cd backend && go run ./cmd/migrate -command status

fmt:
	cd backend && gofmt -w $$(find . -name '*.go' -type f)
	cd frontend && npm run lint -- --fix

lint:
	cd backend && go vet ./...
	cd frontend && npm run lint
	cd frontend && npm run typecheck

test:
	cd backend && go test ./...
	cd frontend && npm run test

build:
	cd backend && go build ./cmd/api ./cmd/worker ./cmd/migrate
	cd frontend && npm run build

image:
	docker build -f deploy/docker/app.Dockerfile \
		--build-arg APP_VERSION=$(APP_VERSION) \
		--build-arg VCS_REF=$(VCS_REF) \
		--build-arg BUILD_DATE=$(BUILD_DATE) \
		-t $(IMAGE) .

compose-check:
	docker compose --env-file deploy/compose/dev/.env.example -f deploy/compose/dev/compose.yaml config --quiet
	docker compose --env-file deploy/compose/prod/.env.example -f deploy/compose/prod/compose.yaml config --quiet
	sh -n deploy/railway/minio-init.sh

prod-config-check:
	@test -f "$(PROD_ENV)" || (echo "missing $(PROD_ENV); copy deploy/compose/prod/.env.example first" >&2; exit 2)
	$(PROD_COMPOSE) config --quiet

prod-init: prod-config-check
	$(PROD_COMPOSE) up -d --wait mysql minio
	$(PROD_COMPOSE) --profile setup run --rm minio-init

prod-migrate: prod-config-check
	$(PROD_COMPOSE) --profile tools run --rm migrate

prod-up: prod-config-check
	$(PROD_COMPOSE) up -d --wait mysql minio app worker proxy

prod-ps: prod-config-check
	$(PROD_COMPOSE) ps

prod-logs: prod-config-check
	$(PROD_COMPOSE) logs -f app worker proxy
