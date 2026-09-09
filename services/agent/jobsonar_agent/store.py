from __future__ import annotations

import json
from collections.abc import Iterable

import psycopg

from jobsonar_agent import config
from jobsonar_agent.dedup import dedup_hash


def _vec(values: Iterable[float]) -> str:
    return "[" + ",".join(f"{x:.8f}" for x in values) + "]"


def _json_list(value) -> list:
    if value is None:
        return []
    if isinstance(value, str):
        value = json.loads(value)
    return list(value) if value else []


# Hard gates live in this CASE (CLAUDE.md golden rule 3 / FRD FR-12):
# must-have skills, seniority mismatch, or location mismatch force
# band='excluded' regardless of the composite the agent computed.
_SCORE_INSERT = """
INSERT INTO scores (job_id, profile_id, composite, skill_cov, semantic,
                     seniority_fit, location_fit, recency, band,
                     matched_skills, missing_skills, scored_at)
SELECT
    %(job_id)s::uuid, %(profile_id)s::uuid, %(composite)s, %(skill_cov)s, %(semantic)s,
    %(seniority_fit)s, %(location_fit)s, %(recency)s,
    CASE
        WHEN NOT (p.must_have_skills = '[]'::jsonb
                  OR p.must_have_skills <@ COALESCE(j.skills_extracted, '[]'::jsonb))
            THEN 'excluded'
        WHEN COALESCE(p.seniority, '') <> '' AND %(seniority_fit)s < 0.3
            THEN 'excluded'
        WHEN (COALESCE(p.location, '') <> '' OR COALESCE(p.remote_pref, '') <> '')
             AND %(location_fit)s = 0.0
            THEN 'excluded'
        ELSE %(band)s
    END,
    %(matched_skills)s::jsonb, %(missing_skills)s::jsonb, now()
FROM jobs j, profiles p
WHERE j.id = %(job_id)s::uuid AND p.id = %(profile_id)s::uuid
ON CONFLICT (job_id, profile_id) DO UPDATE SET
    composite = EXCLUDED.composite,
    skill_cov = EXCLUDED.skill_cov,
    semantic = EXCLUDED.semantic,
    seniority_fit = EXCLUDED.seniority_fit,
    location_fit = EXCLUDED.location_fit,
    recency = EXCLUDED.recency,
    band = EXCLUDED.band,
    matched_skills = EXCLUDED.matched_skills,
    missing_skills = EXCLUDED.missing_skills,
    scored_at = EXCLUDED.scored_at
"""


class Store:
    def __init__(self, dsn: str | None = None):
        self.dsn = dsn or config.POSTGRES_DSN

    def connect(self):
        return psycopg.connect(self.dsn)

    def pending_resumes(self) -> list[dict]:
        with self.connect() as conn, conn.cursor() as cur:
            cur.execute(
                """
                SELECT id::text, storage_uri, profile_id::text
                FROM resumes
                WHERE status = 'pending'
                ORDER BY created_at
                """
            )
            return [
                {"id": r[0], "storage_uri": r[1], "profile_id": r[2]}
                for r in cur.fetchall()
            ]

    def mark_resume(self, resume_id: str, status: str, parsed: dict | None, error: str = "") -> None:
        with self.connect() as conn, conn.cursor() as cur:
            cur.execute(
                """
                UPDATE resumes
                SET status = %s, parsed = %s, error = %s
                WHERE id = %s::uuid
                """,
                (status, json.dumps(parsed) if parsed is not None else None, error, resume_id),
            )
            conn.commit()

    def upsert_skills(self, profile_id: str, skills: list[str]) -> None:
        """Week 8: profiles are named and pre-seeded (amol/nitiraj) --
        this only ever updates an existing row by id, it never inserts."""
        raw = json.dumps(skills)
        with self.connect() as conn, conn.cursor() as cur:
            cur.execute(
                """
                UPDATE profiles
                SET skills = %s::jsonb, embedding = NULL, updated_at = now()
                WHERE id = %s::uuid
                """,
                (raw, profile_id),
            )
            conn.commit()

    def upsert_profile(self, profile_id: str, skills: list[str], embedding: list[float]) -> None:
        raw = json.dumps(skills)
        vec = _vec(embedding)
        with self.connect() as conn, conn.cursor() as cur:
            cur.execute(
                """
                UPDATE profiles
                SET skills = %s::jsonb, embedding = %s::vector, updated_at = now()
                WHERE id = %s::uuid
                """,
                (raw, vec, profile_id),
            )
            conn.commit()

    def profiles_missing_embedding(self) -> list[dict]:
        with self.connect() as conn, conn.cursor() as cur:
            cur.execute(
                """
                SELECT id::text, skills
                FROM profiles
                WHERE embedding IS NULL
                """
            )
            out = []
            for rid, skills in cur.fetchall():
                if isinstance(skills, str):
                    skills = json.loads(skills)
                out.append({"id": rid, "skills": skills or []})
            return out

    def set_profile_embedding(self, profile_id: str, embedding: list[float]) -> None:
        # Embed-only: do not bump updated_at. That column stale-marks every
        # scores row; a vector fill-in must not force a full rescore.
        with self.connect() as conn, conn.cursor() as cur:
            cur.execute(
                """
                UPDATE profiles
                SET embedding = %s::vector
                WHERE id = %s::uuid
                """,
                (_vec(embedding), profile_id),
            )
            conn.commit()

    def jobs_missing_embeddings(self, limit: int) -> list[dict]:
        with self.connect() as conn, conn.cursor() as cur:
            cur.execute(
                """
                SELECT j.id::text, j.title, j.description_md
                FROM jobs j
                LEFT JOIN job_embeddings e ON e.job_id = j.id
                WHERE e.job_id IS NULL
                ORDER BY j.last_seen_at DESC
                LIMIT %s
                """,
                (limit,),
            )
            return [
                {"id": r[0], "title": r[1] or "", "description_md": r[2] or ""}
                for r in cur.fetchall()
            ]

    def list_profiles(self) -> list[dict]:
        """Week 8: named multi-profile support. Every profile-scoped agent
        pass (scoring, shortlist, deep-dive, personalized search) iterates
        this instead of assuming a single implicit profile."""
        with self.connect() as conn, conn.cursor() as cur:
            cur.execute(
                """
                SELECT id::text, name, skills, seniority, location, remote_pref,
                       last_personalized_search_at
                FROM profiles
                ORDER BY name
                """
            )
            out = []
            for rid, name, skills, seniority, location, remote_pref, last_search in cur.fetchall():
                if isinstance(skills, str):
                    skills = json.loads(skills)
                out.append({
                    "id": rid,
                    "name": name,
                    "skills": skills or [],
                    "seniority": seniority,
                    "location": location,
                    "remote_pref": remote_pref,
                    "last_personalized_search_at": last_search,
                })
            return out

    def get_profile(self, profile_id: str) -> dict | None:
        with self.connect() as conn, conn.cursor() as cur:
            cur.execute(
                """
                SELECT id::text, name, skills, seniority, location, remote_pref
                FROM profiles
                WHERE id = %s::uuid
                """,
                (profile_id,),
            )
            row = cur.fetchone()
            if not row:
                return None
            rid, name, skills, seniority, location, remote_pref = row
            if isinstance(skills, str):
                skills = json.loads(skills)
            return {
                "id": rid,
                "name": name,
                "skills": skills or [],
                "seniority": seniority,
                "location": location,
                "remote_pref": remote_pref,
            }

    def jobs_for_scoring(self, profile_id: str, limit: int) -> list[dict]:
        """Jobs missing a scores row, or re-scored since last_seen_at/
        profile.updated_at moved -- same "backfill what's stale" shape as
        jobs_missing_embeddings."""
        with self.connect() as conn, conn.cursor() as cur:
            cur.execute(
                """
                SELECT j.id::text, j.title, j.description_md, j.location, j.remote_type,
                       j.posted_at, j.first_seen_at, j.skills_extracted,
                       (1 - (e.embedding <=> p.embedding))::float8 AS semantic
                FROM jobs j
                LEFT JOIN job_embeddings e ON e.job_id = j.id
                JOIN profiles p ON p.id = %(profile_id)s::uuid
                LEFT JOIN scores s ON s.job_id = j.id AND s.profile_id = p.id
                WHERE s.job_id IS NULL
                   OR s.scored_at < j.last_seen_at
                   OR s.scored_at < p.updated_at
                ORDER BY j.last_seen_at DESC
                LIMIT %(limit)s
                """,
                {"profile_id": profile_id, "limit": limit},
            )
            cols = (
                "id", "title", "description_md", "location", "remote_type",
                "posted_at", "first_seen_at", "skills_extracted", "semantic",
            )
            out = []
            for row in cur.fetchall():
                item = dict(zip(cols, row))
                item["skills_extracted"] = (
                    None if item["skills_extracted"] is None
                    else _json_list(item["skills_extracted"])
                )
                out.append(item)
            return out

    def upsert_score(
        self,
        job_id: str,
        profile_id: str,
        *,
        composite: float,
        skill_cov: float,
        semantic: float | None,
        seniority_fit: float,
        location_fit: float,
        recency: float,
        band: str,
        matched_skills: list[str],
        missing_skills: list[str],
    ) -> None:
        """Persists one job's sub-scores. Hard gates are the CASE in
        _SCORE_INSERT — decided by Postgres, not Python control flow.
        """
        self.write_score_batch(profile_id, [{
            "job_id": job_id,
            "composite": composite,
            "skill_cov": skill_cov,
            "semantic": semantic,
            "seniority_fit": seniority_fit,
            "location_fit": location_fit,
            "recency": recency,
            "band": band,
            "matched_skills": matched_skills,
            "missing_skills": missing_skills,
        }])

    def write_score_batch(self, profile_id: str, rows: list[dict]) -> None:
        """One connection / one commit for a scoring batch. Optional
        write_skills updates jobs.skills_extracted before the gate CASE
        reads it.
        """
        if not rows:
            return
        with self.connect() as conn, conn.cursor() as cur:
            for row in rows:
                if row.get("write_skills"):
                    cur.execute(
                        "UPDATE jobs SET skills_extracted = %s::jsonb WHERE id = %s::uuid",
                        (json.dumps(row.get("skills") or []), row["job_id"]),
                    )
                cur.execute(
                    _SCORE_INSERT,
                    {
                        "job_id": row["job_id"],
                        "profile_id": profile_id,
                        "composite": row["composite"],
                        "skill_cov": row["skill_cov"],
                        "semantic": row.get("semantic"),
                        "seniority_fit": row["seniority_fit"],
                        "location_fit": row["location_fit"],
                        "recency": row["recency"],
                        "band": row["band"],
                        "matched_skills": json.dumps(row.get("matched_skills") or []),
                        "missing_skills": json.dumps(row.get("missing_skills") or []),
                    },
                )
            conn.commit()

    def set_job_skills_extracted(self, job_id: str, skills: list[str]) -> None:
        with self.connect() as conn, conn.cursor() as cur:
            cur.execute(
                "UPDATE jobs SET skills_extracted = %s::jsonb WHERE id = %s::uuid",
                (json.dumps(skills), job_id),
            )
            conn.commit()

    def jobs_for_deep_dive(self, profile_id: str, band: str) -> list[dict]:
        """Shortlist: scores at `band` with no analyses row yet.
        A rescore must not rewrite existing prose (that made every
        resume upload re-prompt every strong job)."""
        with self.connect() as conn, conn.cursor() as cur:
            cur.execute(
                """
                SELECT j.id::text, j.title, j.company, j.description_md,
                       s.matched_skills, s.missing_skills, s.band
                FROM scores s
                JOIN jobs j ON j.id = s.job_id
                LEFT JOIN analyses a ON a.job_id = s.job_id AND a.profile_id = s.profile_id
                WHERE s.profile_id = %s::uuid
                  AND s.band = %s
                  AND a.job_id IS NULL
                ORDER BY s.composite DESC
                """,
                (profile_id, band),
            )
            out = []
            for rid, title, company, desc, matched, missing, jband in cur.fetchall():
                if isinstance(matched, str):
                    matched = json.loads(matched)
                if isinstance(missing, str):
                    missing = json.loads(missing)
                out.append({
                    "id": rid,
                    "title": title or "",
                    "company": company or "",
                    "description_md": desc or "",
                    "matched_skills": matched or [],
                    "missing_skills": missing or [],
                    "band": jband,
                })
            return out

    def upsert_analysis(
        self,
        job_id: str,
        profile_id: str,
        *,
        justification_md: str,
        tailoring_md: str,
        model: str,
    ) -> None:
        with self.connect() as conn, conn.cursor() as cur:
            cur.execute(
                """
                INSERT INTO analyses (job_id, profile_id, justification_md, tailoring_md, model, created_at)
                VALUES (%s::uuid, %s::uuid, %s, %s, %s, now())
                ON CONFLICT (job_id, profile_id) DO UPDATE SET
                    justification_md = EXCLUDED.justification_md,
                    tailoring_md = EXCLUDED.tailoring_md,
                    model = EXCLUDED.model,
                    created_at = EXCLUDED.created_at
                """,
                (job_id, profile_id, justification_md, tailoring_md, model),
            )
            conn.commit()

    def latest_done_resume(self, profile_id: str) -> dict | None:
        with self.connect() as conn, conn.cursor() as cur:
            cur.execute(
                """
                SELECT id::text, storage_uri
                FROM resumes
                WHERE status = 'done' AND profile_id = %s::uuid
                ORDER BY created_at DESC
                LIMIT 1
                """,
                (profile_id,),
            )
            row = cur.fetchone()
            if not row:
                return None
            return {"id": row[0], "storage_uri": row[1]}

    def pending_tailor_jobs(self) -> list[dict]:
        with self.connect() as conn, conn.cursor() as cur:
            cur.execute(
                """
                SELECT id::text, profile_id::text, title, company, jd_md, status,
                       resume_md, cover_letter_md, notes_md
                FROM tailor_jobs
                WHERE status = 'pending'
                   OR (status = 'done' AND COALESCE(resume_docx_uri, '') = '')
                ORDER BY created_at
                """
            )
            return [
                {
                    "id": r[0],
                    "profile_id": r[1],
                    "title": r[2] or "",
                    "company": r[3] or "",
                    "jd_md": r[4] or "",
                    "status": r[5] or "",
                    "resume_md": r[6] or "",
                    "cover_letter_md": r[7] or "",
                    "notes_md": r[8] or "",
                }
                for r in cur.fetchall()
            ]

    def finish_tailor_job(
        self,
        tailor_id: str,
        *,
        status: str,
        error: str = "",
        resume_id: str | None = None,
        profile_id: str | None = None,
        resume_md: str = "",
        cover_letter_md: str = "",
        notes_md: str = "",
        model: str = "",
        resume_docx_uri: str = "",
        cover_docx_uri: str = "",
    ) -> None:
        with self.connect() as conn, conn.cursor() as cur:
            cur.execute(
                """
                UPDATE tailor_jobs
                SET status = %s,
                    error = %s,
                    resume_id = COALESCE(%s::uuid, resume_id),
                    profile_id = COALESCE(%s::uuid, profile_id),
                    resume_md = %s,
                    cover_letter_md = %s,
                    notes_md = %s,
                    model = %s,
                    resume_docx_uri = %s,
                    cover_docx_uri = %s,
                    updated_at = now()
                WHERE id = %s::uuid
                """,
                (
                    status, error, resume_id, profile_id,
                    resume_md, cover_letter_md, notes_md, model,
                    resume_docx_uri, cover_docx_uri, tailor_id,
                ),
            )
            conn.commit()

    def upsert_discovered_job(self, job: dict, profile_id: str) -> str:
        """Writes one personalized-search result (Week 8). Dedupes against
        every other source via the same dedup_hash the Go connectors use
        (jobsonar_agent.dedup) -- a role a connector already ingested
        collapses to that same jobs row, just gains a job_sources +
        job_discoveries entry, rather than duplicating it. skills_extracted
        is left NULL so the next score_jobs pass extracts it for real."""
        h = dedup_hash(job["company"], job["title"], job.get("location") or "")
        with self.connect() as conn, conn.cursor() as cur:
            cur.execute(
                """
                INSERT INTO jobs (dedup_hash, source, source_url, title, company, location, remote_type, description_md)
                VALUES (%(hash)s, 'personalized-search', %(url)s, %(title)s, %(company)s, %(location)s, %(remote_type)s, %(description)s)
                ON CONFLICT (dedup_hash) DO UPDATE SET last_seen_at = now()
                RETURNING id::text
                """,
                {
                    "hash": h,
                    "url": job["source_url"],
                    "title": job["title"],
                    "company": job["company"],
                    "location": job.get("location") or "",
                    "remote_type": job.get("remote_type") or "",
                    "description": job.get("description_md") or "",
                },
            )
            job_id = cur.fetchone()[0]
            cur.execute(
                """
                INSERT INTO job_sources (job_id, source, source_url)
                VALUES (%s::uuid, 'personalized-search', %s)
                ON CONFLICT (job_id, source, source_url) DO NOTHING
                """,
                (job_id, job["source_url"]),
            )
            cur.execute(
                """
                INSERT INTO job_discoveries (job_id, profile_id, source, discovered_at)
                VALUES (%s::uuid, %s::uuid, 'personalized-search', now())
                ON CONFLICT (job_id, profile_id) DO UPDATE SET discovered_at = now()
                """,
                (job_id, profile_id),
            )
            conn.commit()
            return job_id

    def mark_personalized_search_run(self, profile_id: str) -> None:
        with self.connect() as conn, conn.cursor() as cur:
            cur.execute(
                "UPDATE profiles SET last_personalized_search_at = now() WHERE id = %s::uuid",
                (profile_id,),
            )
            conn.commit()

    def upsert_job_embeddings(self, rows: list[tuple[str, list[float]]], model: str) -> None:
        if not rows:
            return
        with self.connect() as conn, conn.cursor() as cur:
            for job_id, embedding in rows:
                cur.execute(
                    """
                    INSERT INTO job_embeddings (job_id, embedding, model)
                    VALUES (%s::uuid, %s::vector, %s)
                    ON CONFLICT (job_id) DO UPDATE
                    SET embedding = EXCLUDED.embedding,
                        model = EXCLUDED.model,
                        updated_at = now()
                    """,
                    (job_id, _vec(embedding), model),
                )
            conn.commit()
