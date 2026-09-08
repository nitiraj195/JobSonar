# Functional Requirements Document (FRD) — JobSonar

Version 0.1 · Owner: you · Status: Draft

## 1. Purpose & scope

JobSonar helps a single job seeker find, evaluate, and track relevant roles. It ingests jobs from legitimate sources, ranks them against a resume with an explainable score, and manages the application lifecycle with analytics. Scope is a **single-user** tool (multi-user is out of scope for v1).

## 2. Actors

- **Seeker** — the user. Uploads a resume, sets preferences, reviews matches, tracks applications.
- **System agents** — the sourcing connectors and the matching agent, which act autonomously on a schedule.

## 3. Functional requirements

Each requirement has an ID (`FR-n`), a priority (P0 must-have / P1 should / P2 nice), and acceptance criteria.

### 3.1 Resume & profile

- **FR-1 (P0) Resume upload & parse.** Seeker uploads a PDF/DOCX resume. System parses it into a structured profile: skills (with inferred years), titles, seniority, domains, location, remote preference, target comp.
  - *Accept:* uploading a resume produces a profile object with ≥1 skill and a seniority band; parse failures surface a clear error.
- **FR-2 (P1) Profile editing.** Seeker can correct any parsed field and add must-have skills, excluded companies, and comp floor.
  - *Accept:* edits persist and are used by the next scoring run.
- **FR-3 (P1) Multiple resume variants.** Seeker can store named resume variants (e.g. "platform", "security").
  - *Accept:* each application records which variant was used.

### 3.2 Sourcing

- **FR-4 (P0) Multi-source ingestion.** System fetches jobs from ≥2 aggregator APIs (Adzuna, Jooble) and ≥1 ATS provider (Greenhouse/Lever/Ashby) on a schedule.
  - *Accept:* a scheduled run inserts new jobs from each configured source; a failing source does not block others.
- **FR-5 (P0) Target-company ATS list.** Seeker maintains a list of companies; system pulls their ATS boards directly.
  - *Accept:* adding a company to the list causes its open roles to appear after the next run.
- **FR-6 (P0) Normalisation.** All sources map to one job schema before storage.
  - *Accept:* jobs from different sources are indistinguishable in shape downstream.
- **FR-7 (P0) Deduplication.** The same role seen on multiple sources collapses to one job with multiple source URLs.
  - *Accept:* a role present on Adzuna and a company's Greenhouse board appears once, listing both URLs.
- **FR-8 (P1) Freshness & closure tracking.** System records `first_seen` / `last_seen`; jobs absent for N runs are marked closed.
  - *Accept:* a job removed from all sources is flagged closed within N scheduled runs.
- **FR-9 (P2) Optional scraper connector.** A scraper (JobSpy) can be enabled per-source, disabled by default.

### 3.3 Matching & scoring

- **FR-10 (P0) Explainable match score.** Each job receives a composite score built from named sub-scores: skill coverage, semantic similarity, seniority fit, location/remote fit, recency.
  - *Accept:* every scored job exposes the sub-score breakdown, not just a single number.
- **FR-11 (P0) Matched vs missing skills.** The system lists which required/preferred skills the resume has and which it lacks.
  - *Accept:* each job shows two lists: matched and missing skills.
- **FR-12 (P0) Hard gates.** Deterministic filters (must-have skills, seniority band, location) cannot be overridden by the semantic score.
  - *Accept:* a job failing a hard gate is excluded or clearly flagged regardless of semantic similarity.
- **FR-13 (P1) Tunable weights.** Seeker adjusts sub-score weights (sliders).
  - *Accept:* changing a weight re-ranks the list without re-fetching.
- **FR-14 (P1) Confidence band.** Scores present as bands (strong / possible / stretch), not false-precision decimals.
- **FR-15 (P1) Deep analysis on shortlist.** Jobs above a threshold receive a premium-model analysis: gap explanation, resume-tailoring hints, match justification.
  - *Accept:* shortlisted jobs carry a written justification; non-shortlisted jobs do not incur premium-model cost.

### 3.4 Application tracking

- **FR-16 (P0) Pipeline stages.** Track each job through saved → applied → screen → interview → offer → closed (rejected/ghosted).
  - *Accept:* status changes are recorded with timestamps forming a timeline.
- **FR-17 (P0) Application record.** Per application: applied date, resume variant, notes, contacts, source.
- **FR-18 (P1) Duplicate-apply guard.** System warns if the seeker is about to apply to an already-applied role (across sources).
- **FR-19 (P1) Ghosting nudge.** Applications with no movement for N days are surfaced for follow-up.

### 3.5 Analytics & delivery

- **FR-20 (P1) Conversion funnel.** Show apply → response → interview → offer rates, sliced by role type and match band.
  - *Accept:* the seeker can see, e.g., response rate for "strong" vs "stretch" matches.
- **FR-21 (P2) Digest.** Daily/weekly summary of new matches above threshold.
- **FR-22 (P2) Company brief.** On demand, a short brief per application (rating, size, recent news). v1 ships company+role review links (Glassdoor, Mouthshut, web search) and optional Brave Search snippets. There is no official Glassdoor or Mouthshut read API; scraping those sites is out of scope.
- **FR-24 (P1) Posted-salary filter and high-pay rank.** Seeker can list only jobs that posted a salary range and rank those by pay (currency-normalised), with match score as a tie-break.
  - *Accept:* `GET /jobs?has_salary=1&sort=salary` returns only rows with `salary_min` or `salary_max`, highest approximate USD pay first.
- **FR-23 (P2) Assisted apply.** One-click open of the application page with a tailored resume suggestion — never automated submission.
- **FR-25 (P1) JD tailor + cover letter.** Seeker pastes a JD or uploads `.md`/`.txt` (or picks a stored job). The local agent drafts a tailored resume and cover letter from the stored resume file. Drafts are never submitted.
  - *Accept:* `POST /tailor` returns 202; after `make embed` / `make agent`, `GET /tailor/{id}` has `resume_md` and `cover_letter_md`. No raw resume is sent to Bedrock.

## 4. Data retention & privacy

- Resume and profile are **PII**; stored encrypted at rest, never sent to third-party LLM APIs unless the seeker opts in. Local models are the default for embeddings.
- PR-run / transient scan data and closed jobs may be pruned on a schedule.

## 5. Out of scope (v1)

Multi-user/teams, automated form submission, mobile app, paid data providers, recruiter-side features.
