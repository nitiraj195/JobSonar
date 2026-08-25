COMPOSE   ?= docker compose
GOOSE     ?= go run github.com/pressly/goose/v3/cmd/goose@v3.24.3
CONNECTORS := services/connectors
WORKER    := services/worker
API       := services/api
AGENT     := services/agent
AGENT_PY  := $(if $(wildcard $(AGENT)/.venv/bin/python),.venv/bin/python,python3)
WEB       := web

ifneq (,$(wildcard .env))
include .env
endif

export ADZUNA_APP_ID ADZUNA_APP_KEY ADZUNA_COUNTRY ADZUNA_WHAT ADZUNA_WHERE
export JOOBLE_API_KEY JOOBLE_BASE_URL API_ADDR
export POSTGRES_USER POSTGRES_PASSWORD POSTGRES_DB POSTGRES_DSN POSTGRES_PORT
export SQS_ENDPOINT_URL SQS_QUEUE_URL AWS_REGION AWS_ACCESS_KEY_ID AWS_SECRET_ACCESS_KEY
export OLLAMA_HOST EMBED_MODEL EMBED_BACKEND
RESUME_DIR ?= $(CURDIR)/data/resumes
export RESUME_DIR

POSTGRES_DSN ?= postgres://jobsonar:jobsonar@localhost:5432/jobsonar?sslmode=disable
LIMIT        ?= 20

.PHONY: up down migrate seed test connector publish worker ingest api web web-build demo wait-db show-jobs show jobs agent embed ollama-pull agent-install

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

seed: wait-db
	$(COMPOSE) exec -T postgres psql -U $${POSTGRES_USER:-jobsonar} -d $${POSTGRES_DB:-jobsonar} -f - < db/seed/greenhouse.sql
	$(COMPOSE) exec -T postgres psql -U $${POSTGRES_USER:-jobsonar} -d $${POSTGRES_DB:-jobsonar} -f - < db/seed/profile.sql

test:
	cd $(CONNECTORS) && go test ./...
	cd $(WORKER) && go test ./...
	cd $(API) && go test ./...
	cd $(AGENT) && PYTHONPATH=. $(AGENT_PY) -m pytest -q

api:
	cd $(API) && go run ./cmd/api

web:
	cd $(WEB) && npm install && npm run dev

web-build:
	cd $(WEB) && npm install && npm run build

agent-install:
	cd $(AGENT) && PY=$$(command -v python3.11 || command -v python3) && $$PY -m venv .venv && .venv/bin/python -m pip install -U pip setuptools && .venv/bin/pip install -e ".[dev]"

# Long-running parse/embed loop (Postgres only; no HTTP to the API).
# EMBED_BACKEND=fake when Ollama has no nomic-embed-text model.
agent:
	cd $(AGENT) && PYTHONPATH=. $(AGENT_PY) -m jobsonar_agent

# One drain-and-exit pass: pending resumes, missing profile/job vectors.
embed:
	cd $(AGENT) && PYTHONPATH=. $(AGENT_PY) -m jobsonar_agent --once

ollama-pull:
	$(COMPOSE) exec -T ollama ollama pull nomic-embed-text

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

# List persisted jobs. Usage:
#   make show-jobs
#   make show jobs
#   make show-jobs LIMIT=50
show-jobs: wait-db
	@echo "$(LIMIT)" | grep -Eq '^[0-9]+$$' || (echo "LIMIT must be a non-negative integer"; exit 1)
	@$(COMPOSE) exec -T postgres psql -U $${POSTGRES_USER:-jobsonar} -d $${POSTGRES_DB:-jobsonar} -P pager=off -c "\
		SELECT count(*) AS total_jobs FROM jobs;"
	@$(COMPOSE) exec -T postgres psql -U $${POSTGRES_USER:-jobsonar} -d $${POSTGRES_DB:-jobsonar} -P pager=off -c "\
		SELECT source, title, company, location, status, posted_at \
		FROM jobs \
		ORDER BY last_seen_at DESC, title \
		LIMIT $(LIMIT);"

# `make show` and `make show jobs` both list jobs (second word is a no-op).
show:
	@$(MAKE) --no-print-directory show-jobs LIMIT=$(LIMIT)
jobs:
	@if [ "$(filter show,$(MAKECMDGOALS))" = "" ]; then \
		$(MAKE) --no-print-directory show-jobs LIMIT=$(LIMIT); \
	fi

# Week 2 demo: connector -> SQS -> worker -> deduped rows in Postgres.
ingest: up migrate publish
	$(MAKE) worker

# Week 1 demo: local stack + schema + Adzuna JSON lines on stdout.
# Needs ADZUNA_APP_ID / ADZUNA_APP_KEY in the environment or .env.
demo: up migrate
	$(MAKE) connector
