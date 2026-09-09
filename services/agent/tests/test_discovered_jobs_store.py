"""Store.upsert_discovered_job against a real Postgres (Week 8): dedup
against other sources, skills_extracted left NULL for re-extraction,
and idempotent re-discovery. Skipped if Postgres isn't reachable, same
convention as test_store_gates.py.
"""

from __future__ import annotations

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
def profile(store):
    profile_id = str(uuid.uuid4())
    with store.connect() as conn, conn.cursor() as cur:
        cur.execute(
            "INSERT INTO profiles (id, name, skills) VALUES (%s::uuid, %s, '[]'::jsonb)",
            (profile_id, f"discover-test-{profile_id}"),
        )
        conn.commit()
    try:
        yield profile_id
    finally:
        with store.connect() as conn, conn.cursor() as cur:
            cur.execute("DELETE FROM profiles WHERE id = %s::uuid", (profile_id,))
            conn.commit()


def _cleanup_job(store, job_id):
    with store.connect() as conn, conn.cursor() as cur:
        cur.execute("DELETE FROM jobs WHERE id = %s::uuid", (job_id,))
        conn.commit()


def test_discovered_job_gets_null_skills_extracted(store, profile):
    job = {
        "title": "Staff SRE",
        "company": "Discover Co",
        "location": "Pune",
        "remote_type": "remote",
        "source_url": "https://example.test/discover-1",
        "description_md": "Own the platform.",
    }
    job_id = store.upsert_discovered_job(job, profile)
    try:
        with store.connect() as conn, conn.cursor() as cur:
            cur.execute("SELECT skills_extracted, source FROM jobs WHERE id = %s::uuid", (job_id,))
            skills_extracted, source = cur.fetchone()
            assert skills_extracted is None
            assert source == "personalized-search"

            cur.execute(
                "SELECT source FROM job_sources WHERE job_id = %s::uuid",
                (job_id,),
            )
            assert cur.fetchone()[0] == "personalized-search"

            cur.execute(
                "SELECT source FROM job_discoveries WHERE job_id = %s::uuid AND profile_id = %s::uuid",
                (job_id, profile),
            )
            assert cur.fetchone()[0] == "personalized-search"
    finally:
        _cleanup_job(store, job_id)


def test_rediscovering_the_same_job_does_not_duplicate(store, profile):
    job = {
        "title": "Staff SRE",
        "company": "Discover Co Two",
        "location": "Pune",
        "source_url": "https://example.test/discover-2",
    }
    job_id_1 = store.upsert_discovered_job(job, profile)
    job_id_2 = store.upsert_discovered_job(job, profile)
    try:
        assert job_id_1 == job_id_2
        with store.connect() as conn, conn.cursor() as cur:
            cur.execute("SELECT count(*) FROM job_discoveries WHERE job_id = %s::uuid", (job_id_1,))
            assert cur.fetchone()[0] == 1
    finally:
        _cleanup_job(store, job_id_1)


def test_dedupes_against_a_job_from_another_source(store, profile):
    from jobsonar_agent.dedup import dedup_hash

    # Same (company, title, location) a connector would have already
    # ingested -- the real dedup_hash, computed the same way the Go
    # worker does, so this seed row is indistinguishable from one it
    # would have created.
    h = dedup_hash("Shared Co", "Backend Engineer", "Remote")
    ctx_id = uuid.uuid4()
    with store.connect() as conn, conn.cursor() as cur:
        cur.execute(
            """
            INSERT INTO jobs (id, dedup_hash, source, source_url, title, company, location)
            VALUES (%s::uuid, %s, 'adzuna', 'https://adzuna.test/existing', 'Backend Engineer', 'Shared Co', 'Remote')
            """,
            (ctx_id, h),
        )
        # A real connector run also writes job_sources -- mirror that so
        # this seed is indistinguishable from one the worker produced.
        cur.execute(
            "INSERT INTO job_sources (job_id, source, source_url) VALUES (%s::uuid, 'adzuna', 'https://adzuna.test/existing')",
            (ctx_id,),
        )
        conn.commit()
    try:
        # Discovered independently via personalized search -- must
        # collapse to the same jobs row, not create a second one.
        job = {"title": "Backend Engineer", "company": "Shared Co", "location": "Remote", "source_url": "https://personalized.test/found"}
        job_id = store.upsert_discovered_job(job, profile)
        assert job_id == str(ctx_id)
        with store.connect() as conn, conn.cursor() as cur:
            cur.execute("SELECT count(*) FROM job_sources WHERE job_id = %s::uuid", (ctx_id,))
            assert cur.fetchone()[0] == 2  # adzuna + personalized-search, same job
    finally:
        _cleanup_job(store, ctx_id)
