# Technical Requirements Document (TRD) — JobSonar

Version 0.1 · Status: Draft · Companion to [FRD.md](FRD.md)

## 1. Architecture summary

Hybrid: managed AWS services for stateful/security-heavy plumbing; self-hosted Go/Python containers for compute-heavy, portable, high-learning layers. Tiered inference (local first pass → Bedrock on shortlist). See [ARCHITECTURE.md](ARCHITECTURE.md).

## 2. Components & responsibilities

| Component | Language | Runtime | Responsibility |
|---|---|---|---|
| `connectors` | Go | k8s CronJob | Fetch + normalise each source, publish unified `Job` to queue |
| `queue` | — | Amazon SQS (+ DLQ) | Buffer normalised jobs, decouple ingest from processing |
| `worker` | Go | k8s Deployment | Drain queue, compute `dedup_hash`, upsert to DB |
| `embedder` | Python | k8s Deployment | Generate embeddings via local model, write vectors |
| `agent` | Python | k8s Deployment / Job | Tiered scoring + gap analysis (LangGraph) |
| `api` | Go (Fiber) | k8s Deployment | REST API for profile, jobs, applications, analytics |
| `web` | React | S3 + CloudFront | UI (Kanban tracker, match views, funnel) |
| `db` | — | RDS Postgres 16 + pgvector | Structured data + vectors |
| `model-server` | — | Ollama (self-hosted) | Local LLM + embedding model |

## 3. Data model (core tables)

```
profiles(id pk, skills jsonb, embedding vector(768), updated_at,        -- Week 5: skills + local embedding
         seniority, location, remote_pref, must_have_skills jsonb)     -- Week 6: hard-gate preferences, all optional (comp_floor not yet needed, deferred)

resumes(id, variant_name, storage_uri, parsed jsonb, status, error, created_at)  -- pending/done/error; no raw text in parsed
jobs(id pk, dedup_hash unique, source, source_url, title, company, location,
     remote_type, description_md, skills_extracted jsonb, salary_min, salary_max,
     currency, posted_at, first_seen_at, last_seen_at, status)
job_sources(job_id, source, source_url)           -- many URLs per deduped job
companies(id pk, name, ats, board_token, created_at)  -- target ATS boards; unique(ats, board_token)
job_embeddings(job_id, embedding vector(768), model, updated_at)
scores(job_id, profile_id, composite, skill_cov, semantic, seniority_fit,       -- Week 6: written by the agent's scoring
       location_fit, recency, band, matched_skills jsonb, missing_skills jsonb, scored_at)  -- pass; API only reads this
analyses(job_id, profile_id, justification_md, tailoring_md, model, created_at)  -- shortlist only
applications(id, job_id, resume_variant, status, applied_at, notes, contacts jsonb)
application_events(application_id, from_status, to_status, at)
company_reviews(company_key, role_key, company, role_title, rating, review_count,
         summary, snippets jsonb, links jsonb, provider, status, error, fetched_at)
         -- cache of company+role reputation. Glassdoor/Mouthshut have no public
         -- read API; provider=links (outbound URLs only) or brave (official Search API)
tailor_jobs(id, profile_id, resume_id, job_id, source, title, company, jd_md,
         resume_md, cover_letter_md, notes_md, model, resume_docx_uri, cover_docx_uri,
         status, error, created_at, updated_at)
         -- on-demand JD → tailored resume + cover letter DOCX. Agent writes files under
         -- TAILOR_DIR; API streams them. Never submitted. Raw resume stays on disk.
```

`dedup_hash = sha256(lower(company) || '|' || normalize(title) || '|' || normalize(location))`.

## 4. Interfaces

### 4.1 Connector contract (Go)

```go
type Connector interface {
    Name() string
    Fetch(ctx context.Context, q SearchParams) ([]RawJob, error)
    Normalize(raw RawJob) (Job, error)
    RateLimit() RateLimit
}
```

New sources implement this interface only. Connector failure is isolated (one source down ≠ pipeline down).

`Normalize` runs inside the connector, before the message reaches the queue — connectors publish the unified `Job`, not the raw source payload. The worker only computes `dedup_hash` and upserts; it never re-normalises. (Connectors and workers are separate Go modules under `services/`, so a worker cannot import a connector's internal `Normalize` — keeping normalisation in the connector avoids that seam entirely, rather than working around it.)

### 4.2 LLM/embedding abstraction (Python)

```python
class LLM(Protocol):
    def complete(self, prompt: str, **kw) -> str: ...

class Embedder(Protocol):
    def embed(self, texts: list[str]) -> list[list[float]]: ...
```

Implementations: `OllamaLLM`, `BedrockLLM`, `LocalEmbedder`, `BedrockEmbedder`. The cascade is a config knob, not a rewrite: first-pass node uses `OllamaLLM`, shortlist node uses `BedrockLLM`.

### 4.3 Key REST endpoints (Go API)

```
GET  /profile            skill list + has_embedding + latest resume status
PUT  /profile            replace skill list (clears embedding until next agent pass)
POST /profile/resume     store PDF/DOCX as pending; Python agent parses (no Go parse)
GET  /jobs               rank by scores.composite; omit band=excluded; include named sub-scores + matched/missing; has_analysis flag
                         query: has_salary=1 (posted range only); sort=match|salary
                         sort=salary orders by FX-normalised pay, then review rating, then composite
                         (INR + value < 1000 is treated as LPA for ranking only; Adzuna IN quirk)
                         payload includes salary_min/max/currency + review (links/rating; snippets omitted on list)
GET  /jobs/{id}          job detail + full breakdown + optional analysis (justification/tailoring); excluded jobs still returned so the gate is explainable
                         refreshes a stale company+role review via Brave (if keyed) or link-only
POST /reviews/refresh    cache reviews for salary-listed jobs (max 40). No Glassdoor/Mouthshut scrape.
POST /tailor             queue a JD (json jd_md, job_id, or .md/.txt upload). 202 pending; agent drafts locally
GET  /tailor             recent tailor_jobs (no JD body)
GET  /tailor/{id}        JD + resume_md + cover_letter_md + notes when status=done
GET  /tailor/{id}/resume.docx   download tailored resume (DOCX)
GET  /tailor/{id}/cover.docx    download cover letter (DOCX)
GET  /applications       tracker rows
POST /applications       create application (default status saved)
PATCH /applications/{id} change status (appends event)
GET  /analytics/funnel   conversion funnel data
POST /companies          add target company to ATS list
```

## 5. Non-functional requirements

- **NFR-1 Cost.** At rest the system runs near AWS free tier; premium-model spend must track shortlist size, not total jobs (tiered inference enforced).
- **NFR-2 Portability.** Same manifests run on kind/k3s locally and EKS in cloud. No hard AWS SDK calls outside a thin adapter layer.
- **NFR-3 Security.** No long-lived cloud keys in manifests (IRSA); secrets sourced from Secrets Manager via External Secrets Operator; resume PII encrypted at rest; local embeddings by default so PII need not leave the cluster.
- **NFR-4 Resilience.** Source failure isolated; SQS DLQ for poison messages; idempotent upserts (dedup_hash) so re-processing is safe.
- **NFR-5 Observability.** OpenTelemetry traces/metrics from every service; exportable to CloudWatch or self-hosted Grafana. Track per-source fetch counts, dedup ratio, score latency, premium-model call count/cost.
- **NFR-6 Determinism guardrail.** Hard gates (must-have skills, seniority, location) evaluated in SQL, never delegated to the LLM.
- **NFR-7 Rate-limit compliance.** Each connector respects source rate limits and backs off on 429; optional scraper respects robots.txt.

## 6. Tech decisions & justification (buy vs build)

| Decision | Choice | Rule it follows |
|---|---|---|
| Queue | SQS | Buy: free at volume, zero ops, DLQ built in |
| DB | RDS + pgvector | Buy: databases are the worst thing to self-host solo (backup/patch/HA) |
| Embeddings | local model | Build: high volume, cheap, PII stays local |
| Reasoning | tiered local→Bedrock | Both: local economics, frontier quality where a human reads it |
| Connectors/workers/API | Go containers | Build: I/O-bound sweet spot + cloud-native learning + portable |
| UI hosting | S3/CloudFront | Buy: cents, no server to secure |
| Secrets | Secrets Manager + ESO | Buy custody + Build consumption |

## 7. Environments

- **local** — docker-compose or kind; Ollama for all inference; Postgres container; no cloud.
- **cloud** — EKS; RDS; SQS; Bedrock enabled for shortlist; ESO + IRSA.

## 8. Testing

Unit tests per connector (recorded fixtures), dedup property tests, scoring golden tests (fixed resume+job → expected sub-scores within tolerance), NFR-1 cost-guard test (deep-dive `complete()` count ≤ shortlist size), contract tests for the API, and a smoke test that runs one connector end-to-end into the DB.
