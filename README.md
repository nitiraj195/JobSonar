# JobSonar

An agentic, resume-driven job tracker. It sources jobs from free aggregator APIs and company ATS boards, scores each opening against your resume with an explainable match model, tracks applications as a personal ATS, and gives you a conversion funnel so you learn which applications actually turn into interviews.

---

## Why this project exists

Job boards optimise for applications sent, not offers received. JobSonar inverts that: it tells you **why** each job is a fit, **what gap** to close, and — over time — **which kinds of applications convert for you**. That feedback loop is the differentiator; matching and tracking are commodity.

Two design commitments carried through the whole project:

1. **Sourcing that won't collapse.** Primary layer is legitimate free APIs (Adzuna, Jooble) and company ATS JSON endpoints (Greenhouse, Lever, Ashby). Scraping is an optional, last-resort connector — never the foundation.
2. **Explainable, tiered matching.** A cheap local model scores every job; a premium model does deep analysis only on the shortlist. Cost tracks value, not volume.

---

## The plan in one paragraph

Build in two vertical stripes. **Stripe one** is the boring-but-real data path — one connector (Adzuna) → queue → normalise/dedup → Postgres+pgvector → a dumb keyword score → a tracker UI — end to end on a laptop with docker-compose. **Stripe two** adds the tiered AI agent (local first pass + Bedrock deep dive), the funnel analytics, and lifts the whole thing onto EKS with the hybrid AWS/self-hosted split. Most people build the fancy agent first and never get sourcing working. We do it backwards.

---

## Tech stack (hybrid)

| Layer | Choice | Buy / Build |
|---|---|---|
| Connectors | Go services, k8s CronJobs | Build (self-host) |
| Queue | Amazon SQS | Buy (managed, free tier) |
| Normalise / dedup | Go workers (goroutines) | Build (self-host) |
| Storage | RDS Postgres + pgvector | Buy (managed) |
| Embeddings | local `bge`/`nomic` model | Build (self-host) |
| Matching agent | tiered: Ollama first pass → Bedrock Claude on shortlist | Both |
| Agent orchestration | LangGraph (Python) | Build (portable) |
| API | Go (Fiber) | Build |
| UI | React on S3 + CloudFront | Buy (hosting) |
| Secrets | Secrets Manager → External Secrets Operator | Buy (custody) + Build (consume) |
| Identity | IRSA (least-privilege per pod) | Build |
| Observability | OpenTelemetry → CloudWatch / Grafana | Portable seam |

**Language split:** Go owns the I/O-bound plumbing (connectors, queue consumers, workers, API); Python owns the AI (embeddings, LLM orchestration, resume parsing). They communicate only via the queue and the database — no tight coupling.

---

## Status

- ✅ **Week 1** — Foundations & one connector. See [`plan/completed/week-1.md`](plan/completed/week-1.md).
- ✅ **Week 2** — Queue + worker + dedup. See [`plan/completed/week-2.md`](plan/completed/week-2.md).
- ✅ **Week 3** — Jooble + Greenhouse + Fiber API. See [`plan/completed/week-3.md`](plan/completed/week-3.md).
- ✅ **Week 4** — UI, keyword score, application tracker. See [`plan/completed/week-4.md`](plan/completed/week-4.md).
- ✅ **Week 5** — Resume parse + local embeddings. See [`plan/completed/week-5.md`](plan/completed/week-5.md).
- Full roadmap: [`docs/WEEKLY_PLAN.md`](docs/WEEKLY_PLAN.md).

---

## Local machine setup

Everything below runs on a laptop with Docker, Go, Python, and Node — no cloud account needed.

**Prerequisites:** Docker (Postgres+pgvector, ElasticMQ, Ollama), Go 1.24+, Python 3.9+, Node.js/npm.

### 1. Configure environment

```
cp .env.example .env
```

Fill in `ADZUNA_APP_ID`/`ADZUNA_APP_KEY` ([developer.adzuna.com](https://developer.adzuna.com)) and/or `JOOBLE_API_KEY` ([jooble.org/api/about](https://jooble.org/api/about)) if you want real job data — both are free-tier. Everything else in `.env.example` already has a working local default.

### 2. Start the local stack

```
make up        # Postgres+pgvector, ElasticMQ, Ollama
make migrate   # apply db/migrations
make seed      # target Greenhouse companies + a starter skill profile
```

### 3. Ingest jobs

```
make ingest      # connector -> SQS -> worker -> Postgres (needs ADZUNA_* and/or JOOBLE_* keys)
make show-jobs   # list what landed (or: make show jobs)
```

### 4. Set up the agent (resume parsing + embeddings)

```
make agent-install   # one-time: creates services/agent/.venv
make ollama-pull     # pulls nomic-embed-text
```

If `ollama-pull` fails (a corporate TLS proxy commonly blocks it), skip it and use `EMBED_BACKEND=fake` in `.env` instead — deterministic local vectors, no model download.

### 5. Run the services

Each runs in its own terminal and stays running:

```
make api     # Go Fiber API — http://localhost:8080
make agent   # long-running: parses uploaded resumes, embeds jobs/profile
make web     # Vite dev server — http://localhost:5173 (proxies /jobs /profile /applications /companies to :8080)
```

(`make agent` runs forever, watching for new resume uploads. For a single parse-and-embed pass instead — e.g. right after uploading — run `make embed` instead of leaving `make agent` running.)

### 6. Use the UI, including resume (PDF/DOCX) upload

Open **http://localhost:5173**. The Jobs page has a "Resume (PDF or DOCX)" file input — pick a file, it uploads via `POST /profile/resume`, and the page polls automatically until the agent finishes parsing and embedding it, then re-ranks jobs by semantic similarity. No separate profile page or upload command needed.

---

## Document index

- [`docs/FRD.md`](docs/FRD.md) — Functional Requirements
- [`docs/TRD.md`](docs/TRD.md) — Technical Requirements
- [`docs/ARCHITECTURE.md`](docs/ARCHITECTURE.md) — Architecture, data flow, diagram
- [`docs/PROJECT_STRUCTURE.md`](docs/PROJECT_STRUCTURE.md) — Repo layout
- [`docs/WEEKLY_PLAN.md`](docs/WEEKLY_PLAN.md) — 10-week build plan
- [`docs/SKILLS_AND_COMMANDS.md`](docs/SKILLS_AND_COMMANDS.md) — Claude Code skills + slash commands
- [`CLAUDE.md`](CLAUDE.md) — Working agreement for Claude Code

---

## Non-goals (deliberately excluded)

- **Auto-apply / automated form submission.** Violates most portals' ToS, produces low-quality applications, risks account bans. JobSonar *assists* a human to apply fast (tailored resume, pre-filled draft, one-click open) — it never submits on your behalf unseen.
- **Scraping LinkedIn / Indeed as a primary source.** Retired APIs and aggressive bot management. Optional scraper connector only, behind the same interface, respecting robots.txt and rate limits.
