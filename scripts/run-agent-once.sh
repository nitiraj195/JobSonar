#!/bin/bash
# Runs one full agent pass (parse/embed/score/tailor/personalized-search-
# if-due) unattended. Invoked by launchd (see
# scripts/com.jobsonar.agent.plist) -- the local-machine equivalent of the
# k8s CronJob docs/PROJECT_STRUCTURE.md describes, which doesn't exist yet
# in this repo. Safe to run repeatedly: every step it calls is self-gating
# (score_jobs only re-scores stale rows; run_personalized_search only
# fires once per PERSONALIZED_SEARCH_INTERVAL_HOURS per profile).
set -euo pipefail

# launchd gives a fresh process a minimal PATH (/usr/gnu/bin:/usr/local/bin:
# /bin:/usr/bin) that does not include Rancher Desktop's docker (~/.rd/bin)
# or Homebrew -- without this, `docker compose` below fails with "command
# not found" every time launchd (not an interactive shell) runs this.
export PATH="/Users/nitiraj.patne/.rd/bin:/opt/homebrew/bin:/usr/local/bin:$PATH"

REPO_DIR="/Users/nitiraj.patne/Library/CloudStorage/OneDrive-Avalara/Desktop/nitiraj.patne/JobSonar"
AGENT_DIR="$REPO_DIR/services/agent"
AGENT_PY="$AGENT_DIR/.venv/bin/python"

cd "$REPO_DIR"
docker compose up -d postgres >/dev/null

for _ in $(seq 1 30); do
    if docker compose exec -T postgres pg_isready -U jobsonar >/dev/null 2>&1; then
        break
    fi
    sleep 1
done

export POSTGRES_DSN="postgres://jobsonar:jobsonar@localhost:5432/jobsonar?sslmode=disable"
# Fake backends: this unattended job must not depend on Ollama being up
# with a model pulled. Real embeddings/deep-dive still happen whenever
# you run `make agent`/`make embed` by hand with Ollama running -- this
# scheduled pass only guarantees personalized search + rescoring against
# whatever embeddings already exist.
export EMBED_BACKEND="${EMBED_BACKEND:-fake}"
export DEEP_DIVE_BACKEND="${DEEP_DIVE_BACKEND:-fake}"
export PERSONALIZED_SEARCH_OPT_IN=1
export PERSONALIZED_SEARCH_INTERVAL_HOURS="${PERSONALIZED_SEARCH_INTERVAL_HOURS:-24}"
export PERSONALIZED_SEARCH_MAX_RESULTS="${PERSONALIZED_SEARCH_MAX_RESULTS:-8}"
export CLAUDE_CLI_TIMEOUT_SECONDS="${CLAUDE_CLI_TIMEOUT_SECONDS:-600}"
export CLAUDE_CLI_PATH="${CLAUDE_CLI_PATH:-/Users/nitiraj.patne/.local/bin/claude}"

echo "=== $(date -u +%Y-%m-%dT%H:%M:%SZ) run-agent-once starting ==="
cd "$AGENT_DIR"
exec env PYTHONPATH=. "$AGENT_PY" -m jobsonar_agent --once
