COMPOSE   ?= docker compose
GOOSE     ?= go run github.com/pressly/goose/v3/cmd/goose@v3.24.3
CONNECTORS := services/connectors
WORKER    := services/worker

ifneq (,$(wildcard .env))
include .env
endif

export ADZUNA_APP_ID ADZUNA_APP_KEY ADZUNA_COUNTRY ADZUNA_WHAT ADZUNA_WHERE
export POSTGRES_USER POSTGRES_PASSWORD POSTGRES_DB POSTGRES_DSN POSTGRES_PORT
export SQS_ENDPOINT_URL SQS_QUEUE_URL AWS_REGION AWS_ACCESS_KEY_ID AWS_SECRET_ACCESS_KEY

POSTGRES_DSN ?= postgres://jobsonar:jobsonar@localhost:5432/jobsonar?sslmode=disable

.PHONY: up down migrate test connector publish worker ingest demo wait-db show-jobs

up:
	$(COMPOSE) up -d

down:
	$(COMPOSE) down

wait-db:
	@echo "waiting for postgres..."
	@i=0; \
	until $(COMPOSE) exec -T postgres pg_isready -U $${POSTGRES_USER:-jobsonar} -d $${POSTGRES_DB:-jobsonar} >/dev/null 2>&1; do \
		i=$$((i+1)); \
		if [ $$i -ge 30 ]; then echo "postgres did not become ready"; exit 1; fi; \
		sleep 1; \
	done

migrate: wait-db
	$(GOOSE) -dir db/migrations postgres "$(POSTGRES_DSN)" up

test:
	cd $(CONNECTORS) && go test ./...
	cd $(WORKER) && go test ./...

connector:
	cd $(CONNECTORS) && go run ./cmd/connector

# Week 2: same connector, publishing to the raw-jobs SQS/ElasticMQ queue
# instead of stdout. Needs SQS_ENDPOINT_URL / SQS_QUEUE_URL (see .env.example).
publish:
	cd $(CONNECTORS) && go run ./cmd/connector -sink=sqs

# Drains raw-jobs into Postgres and exits once idle (WORKER_IDLE_EXIT_AFTER
# empty polls) rather than running forever — the right shape for `make
# ingest`, not yet for a long-running deployment.
worker:
	cd $(WORKER) && go run ./cmd/worker

# Week 2 demo: connector -> SQS -> worker -> deduped rows in Postgres.
ingest: up migrate publish
	$(MAKE) worker

# Week 1 demo: local stack + schema + Adzuna JSON lines on stdout.
# Needs ADZUNA_APP_ID / ADZUNA_APP_KEY in the environment or .env.
demo: up migrate
	$(MAKE) connector

# Quick look at what's landed in Postgres after `make ingest`.
show-jobs:
	$(COMPOSE) exec -T postgres psql -U $${POSTGRES_USER:-jobsonar} -d $${POSTGRES_DB:-jobsonar} -c \
		"SELECT company, title, location, source, posted_at FROM jobs ORDER BY first_seen_at DESC LIMIT 20;"
