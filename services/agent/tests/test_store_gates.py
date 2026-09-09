"""Hard-gate integration tests (Week 6 Day 4): the gate decision inside
Store.upsert_score's SQL, exercised against a real Postgres. Skipped if
one isn't reachable, so `pytest` stays usable offline; not gated on
POSTGRES_DSN merely being *set*, since .env always sets it to a working
local default (the same mistake already fixed once this project, in the
connectors' SQS integration test -- see services/connectors/internal/
queue/integration_test.go).
"""

from __future__ import annotations

import json
import uuid

import psycopg
import pytest

from jobsonar_agent import config
from jobsonar_agent.store import Store


@pytest.fixture
def store():
    try:
        conn = psycopg.connect(config.POSTGRES_DSN)
        conn.close()
    except psycopg.OperationalError:
        pytest.skip("no live Postgres reachable at POSTGRES_DSN")
    return Store()


@pytest.fixture
def profile_and_job(store):
    """One throwaway profile + job per test, cleaned up afterward so the
    demo dataset doesn't accumulate test rows."""
    profile_id = str(uuid.uuid4())
    job_id = str(uuid.uuid4())
    with store.connect() as conn, conn.cursor() as cur:
        cur.execute(
            """
            INSERT INTO profiles (id, name, skills, must_have_skills, seniority, location, remote_pref)
            VALUES (%s::uuid, %s, '[]'::jsonb, '[]'::jsonb, NULL, NULL, NULL)
            """,
            (profile_id, f"gate-test-{profile_id}"),
        )
        cur.execute(
            """
            INSERT INTO jobs (id, dedup_hash, source, source_url, title, company, location, remote_type, skills_extracted)
            VALUES (%s::uuid, %s, 'test', 'https://example.com/gate-test', 'Senior DevOps Engineer', 'Acme Gate Test', 'Pune, Maharashtra', '', '[]'::jsonb)
            """,
            (job_id, f"gate-test-{job_id}"),
        )
        conn.commit()
    try:
        yield profile_id, job_id
    finally:
        with store.connect() as conn, conn.cursor() as cur:
            cur.execute("DELETE FROM jobs WHERE id = %s::uuid", (job_id,))
            cur.execute("DELETE FROM profiles WHERE id = %s::uuid", (profile_id,))
            conn.commit()


def _score_row(store, job_id, profile_id):
    with store.connect() as conn, conn.cursor() as cur:
        cur.execute(
            "SELECT band, composite FROM scores WHERE job_id = %s::uuid AND profile_id = %s::uuid",
            (job_id, profile_id),
        )
        return cur.fetchone()


def _upsert(store, job_id, profile_id, **overrides):
    kwargs = dict(
        composite=0.9,
        skill_cov=1.0,
        semantic=None,
        seniority_fit=1.0,
        location_fit=1.0,
        recency=1.0,
        band="strong",
        matched_skills=[],
        missing_skills=[],
    )
    kwargs.update(overrides)
    store.upsert_score(job_id, profile_id, **kwargs)


def test_no_preferences_uses_computed_band(store, profile_and_job):
    profile_id, job_id = profile_and_job
    _upsert(store, job_id, profile_id, band="strong")
    band, composite = _score_row(store, job_id, profile_id)
    assert band == "strong"
    assert composite == 0.9


def test_must_have_skill_missing_forces_excluded(store, profile_and_job):
    profile_id, job_id = profile_and_job
    with store.connect() as conn, conn.cursor() as cur:
        cur.execute(
            "UPDATE profiles SET must_have_skills = %s::jsonb WHERE id = %s::uuid",
            (json.dumps(["kubernetes"]), profile_id),
        )
        conn.commit()
    # job's skills_extracted is [] -- must_have_skills is not a subset
    _upsert(store, job_id, profile_id, band="strong", composite=0.95)
    band, _ = _score_row(store, job_id, profile_id)
    assert band == "excluded"


def test_must_have_skill_present_passes(store, profile_and_job):
    profile_id, job_id = profile_and_job
    with store.connect() as conn, conn.cursor() as cur:
        cur.execute(
            "UPDATE profiles SET must_have_skills = %s::jsonb WHERE id = %s::uuid",
            (json.dumps(["kubernetes"]), profile_id),
        )
        cur.execute(
            "UPDATE jobs SET skills_extracted = %s::jsonb WHERE id = %s::uuid",
            (json.dumps(["kubernetes", "terraform"]), job_id),
        )
        conn.commit()
    _upsert(store, job_id, profile_id, band="strong")
    band, _ = _score_row(store, job_id, profile_id)
    assert band == "strong"


def test_seniority_mismatch_forces_excluded_even_with_high_composite(store, profile_and_job):
    profile_id, job_id = profile_and_job
    with store.connect() as conn, conn.cursor() as cur:
        cur.execute("UPDATE profiles SET seniority = 'senior' WHERE id = %s::uuid", (profile_id,))
        conn.commit()
    # seniority_fit below the 0.3 gate threshold, but a deliberately high
    # composite/semantic to prove the gate isn't overridable by them.
    _upsert(store, job_id, profile_id, seniority_fit=0.0, composite=0.99, semantic=1.0, band="strong")
    band, composite = _score_row(store, job_id, profile_id)
    assert band == "excluded"
    assert composite == 0.99  # the composite value itself is unchanged -- only band is forced


def test_location_mismatch_forces_excluded(store, profile_and_job):
    profile_id, job_id = profile_and_job
    with store.connect() as conn, conn.cursor() as cur:
        cur.execute("UPDATE profiles SET location = 'Amsterdam' WHERE id = %s::uuid", (profile_id,))
        conn.commit()
    _upsert(store, job_id, profile_id, location_fit=0.0, band="strong")
    band, _ = _score_row(store, job_id, profile_id)
    assert band == "excluded"


def test_partial_location_fit_is_not_gated(store, profile_and_job):
    profile_id, job_id = profile_and_job
    with store.connect() as conn, conn.cursor() as cur:
        cur.execute("UPDATE profiles SET location = 'Amsterdam', remote_pref = 'remote' WHERE id = %s::uuid", (profile_id,))
        conn.commit()
    # mixed signal (location_fit > 0 but < 1) must not trip the gate --
    # only a clean 0.0 (every available signal mismatched) does.
    _upsert(store, job_id, profile_id, location_fit=0.5, band="possible")
    band, _ = _score_row(store, job_id, profile_id)
    assert band == "possible"
