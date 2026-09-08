from __future__ import annotations

import argparse
import logging
import time

from jobsonar_agent import config
from jobsonar_agent.embed.fake import FakeEmbedder
from jobsonar_agent.embed.ollama import OllamaEmbedder
from jobsonar_agent.graph.cascade import build_graph, run_cascade
from jobsonar_agent.llm import Embedder
from jobsonar_agent.llm.factory import resolve_llm
from jobsonar_agent.otel import tracer
from jobsonar_agent.resume.parse import parse_resume
from jobsonar_agent.score.composite import band, composite
from jobsonar_agent.score.location import fit as location_fit
from jobsonar_agent.score.recency import fit as recency_fit
from jobsonar_agent.score.seniority import fit as seniority_fit
from jobsonar_agent.score.skill_coverage import coverage, extract_job_skills
from jobsonar_agent.store import Store
from jobsonar_agent.tailor.run import drain_tailor_jobs

log = logging.getLogger("jobsonar.agent")


def profile_text(skills: list[str]) -> str:
    return " ".join(skills)


def job_text(title: str, description: str) -> str:
    # nomic does not need the full posting; cap tokens so a 200-job
    # backfill is tens of seconds, not minutes.
    cap = config.EMBED_TEXT_CHARS
    body = (description or "")[:cap]
    return f"{title}\n{body}".strip() or title or "job"


def parse_pending(store: Store, embedder: Embedder) -> int:
    n = 0
    for row in store.pending_resumes():
        rid = row["id"]
        with tracer().start_as_current_span("resume.parse") as span:
            span.set_attribute("resume.id", rid)
            try:
                parsed = parse_resume(row["storage_uri"])
                skills = parsed.get("skills") or []
                if not skills:
                    store.mark_resume(rid, "error", parsed, "no skills matched lexicon")
                    log.info("resume %s: no skills matched", rid)
                    continue
                store.mark_resume(rid, "done", parsed, "")
                span.set_attribute("resume.skill_count", len(skills))
                try:
                    vecs = embedder.embed([profile_text(skills)])
                    store.upsert_profile(skills, vecs[0])
                except Exception as exc:
                    store.upsert_skills(skills)
                    log.warning("resume %s: parsed, embed failed (%s)", rid, type(exc).__name__)
                log.info("resume %s: parsed %d skills", rid, len(skills))
                n += 1
            except Exception as exc:
                store.mark_resume(rid, "error", None, type(exc).__name__)
                log.warning("resume %s: %s", rid, type(exc).__name__)
    return n


def embed_profiles(store: Store, embedder: Embedder) -> int:
    rows = [p for p in store.profiles_missing_embedding() if p["skills"]]
    if not rows:
        return 0
    vecs = embedder.embed([profile_text(p["skills"]) for p in rows])
    for p, vec in zip(rows, vecs):
        store.set_profile_embedding(p["id"], vec)
    log.info("embedded %d profiles", len(rows))
    return len(rows)


def embed_jobs(store: Store, embedder: Embedder) -> int:
    total = 0
    while True:
        batch = store.jobs_missing_embeddings(config.EMBED_BATCH)
        if not batch:
            break
        texts = [job_text(j["title"], j["description_md"]) for j in batch]
        vecs = embedder.embed(texts)
        store.upsert_job_embeddings(
            [(j["id"], v) for j, v in zip(batch, vecs)],
            config.EMBED_MODEL,
        )
        total += len(batch)
        log.info("embedded %d jobs (batch)", len(batch))
    return total


def score_jobs(store: Store) -> int:
    """Week 6: composite + named sub-scores for every job missing/stale a
    scores row, against the current profile. Hard gates are evaluated by
    Store.upsert_score's SQL, not here -- this function only computes the
    continuous, explainable sub-scores and hands them off."""
    profile = store.current_profile()
    if not profile:
        return 0
    total = 0
    while True:
        batch = store.jobs_for_scoring(profile["id"], config.SCORE_BATCH)
        if not batch:
            break
        prepared = []
        for j in batch:
            # None = never extracted (re-parse JD). [] = extracted, no hits
            # — reuse so a rescore does not walk every posting again.
            if j.get("skills_extracted") is None:
                job_skills = extract_job_skills(j["title"] or "", j["description_md"] or "")
                write_skills = True
            else:
                job_skills = j.get("skills_extracted") or []
                write_skills = False
            skill_cov, matched, missing = coverage(profile["skills"], job_skills)
            sen_fit = seniority_fit(profile["seniority"], j["title"] or "")
            loc_fit = location_fit(profile["location"], profile["remote_pref"], j["location"] or "", j["remote_type"] or "")
            rec = recency_fit(j["posted_at"], j["first_seen_at"])
            comp = composite(skill_cov, j["semantic"], sen_fit, loc_fit, rec)
            prepared.append({
                "job_id": j["id"],
                "write_skills": write_skills,
                "skills": job_skills,
                "composite": comp,
                "skill_cov": skill_cov,
                "semantic": j["semantic"],
                "seniority_fit": sen_fit,
                "location_fit": loc_fit,
                "recency": rec,
                "band": band(comp),
                "matched_skills": matched,
                "missing_skills": missing,
            })
        store.write_score_batch(profile["id"], prepared)
        total += len(batch)
        log.info("scored %d jobs (batch)", len(batch))
    return total


def _first_pass(store: Store, embedder: Embedder):
    def first_pass(_state):
        parsed = parse_pending(store, embedder)
        profiles = embed_profiles(store, embedder)
        jobs = embed_jobs(store, embedder)
        scored = score_jobs(store)
        return {
            "first_pass": {
                "resumes": parsed,
                "profiles": profiles,
                "jobs": jobs,
                "scored": scored,
            },
        }

    return first_pass


def _counts(result: dict) -> dict[str, int]:
    fp = result.get("first_pass") or {}
    return {
        "resumes": fp.get("resumes", 0),
        "profiles": fp.get("profiles", 0),
        "jobs": fp.get("jobs", 0),
        "scored": fp.get("scored", 0),
        "deep_dive": len(result.get("prompted_job_ids") or []),
        "premium_calls": result.get("premium_calls") or 0,
        "tailored": result.get("tailored") or 0,
    }


def once(store: Store | None = None, embedder: Embedder | None = None, graph=None) -> dict[str, int]:
    store = store or Store()
    if embedder is None:
        embedder = FakeEmbedder() if config.EMBED_BACKEND == "fake" else OllamaEmbedder()
    if graph is not None:
        counts = _counts(graph.invoke({}))
    else:
        llm = resolve_llm()
        counts = _counts(run_cascade(store, llm, _first_pass(store, embedder)))
    try:
        counts["tailored"] = drain_tailor_jobs(store)
    except Exception as exc:
        log.warning("tailor drain failed: %s", type(exc).__name__)
        counts["tailored"] = 0
    return counts


def main(argv: list[str] | None = None) -> int:
    logging.basicConfig(level=logging.INFO, format="%(asctime)s %(levelname)s %(message)s")
    parser = argparse.ArgumentParser(description="JobSonar local embed/parse worker")
    parser.add_argument("--once", action="store_true", help="one drain-and-exit pass")
    args = parser.parse_args(argv)
    store = Store()
    embedder = FakeEmbedder() if config.EMBED_BACKEND == "fake" else OllamaEmbedder()
    llm = resolve_llm()
    # Compile once. Rebuilding LangGraph every 1s loop was wasted work
    # on the hot path after a resume upload.
    graph = build_graph(store, llm, _first_pass(store, embedder))
    if args.once:
        counts = once(store, embedder, graph=graph)
        log.info(
            "done resumes=%s profiles=%s jobs=%s scored=%s deep_dive=%s premium=%s tailored=%s",
            counts["resumes"], counts["profiles"], counts["jobs"], counts["scored"],
            counts.get("deep_dive", 0), counts.get("premium_calls", 0),
            counts.get("tailored", 0),
        )
        return 0
    while True:
        try:
            once(store, embedder, graph=graph)
        except Exception as exc:
            log.warning("agent pass failed: %s", type(exc).__name__)
        time.sleep(1)


if __name__ == "__main__":
    raise SystemExit(main())
