from __future__ import annotations

import logging
from pathlib import Path

from jobsonar_agent import config
from jobsonar_agent.llm import LLM
from jobsonar_agent.llm.factory import resolve_tailor_llm
from jobsonar_agent.otel import tracer
from jobsonar_agent.resume.parse import extract_text
from jobsonar_agent.store import Store
from jobsonar_agent.tailor.docx import write_markdown_docx
from jobsonar_agent.tailor.fallback import fallback_draft
from jobsonar_agent.tailor.prompt import build_tailor_prompt, parse_tailor

log = logging.getLogger("jobsonar.agent")


def drain_tailor_jobs(store: Store, llm: LLM | None = None) -> int:
    """Process pending tailor_jobs and backfill missing DOCX. Local LLM only."""
    llm = llm or resolve_tailor_llm()
    n = 0
    for row in store.pending_tailor_jobs():
        with tracer().start_as_current_span("tailor.job") as span:
            span.set_attribute("tailor.id", row["id"])
            try:
                _run_one(store, llm, row)
                n += 1
            except Exception as exc:
                store.finish_tailor_job(
                    row["id"],
                    status="error",
                    error=type(exc).__name__,
                )
                log.warning("tailor %s: %s", row["id"], type(exc).__name__)
    return n


def _run_one(store: Store, llm: LLM, row: dict) -> None:
    resume = store.latest_done_resume()
    if not resume:
        store.finish_tailor_job(row["id"], status="error", error="upload a resume first")
        return
    text = extract_text(resume["storage_uri"])
    if not text.strip():
        store.finish_tailor_job(row["id"], status="error", error="resume file is empty")
        return
    profile = store.current_profile() or {"skills": []}
    resume_md = (row.get("resume_md") or "").strip()
    cover = (row.get("cover_letter_md") or "").strip()
    notes = (row.get("notes_md") or "").strip()
    model = getattr(llm, "model", "") or type(llm).__name__

    if row.get("status") != "done" or len(resume_md) < 400:
        prompt = build_tailor_prompt(profile=profile, resume_text=text, jd=row)
        raw = llm.complete(prompt)
        parsed = parse_tailor(raw)
        if parsed:
            resume_md, cover, notes = parsed
        if len(resume_md) < 400:
            resume_md, cover, notes = fallback_draft(resume_text=text, profile=profile, jd=row)
            model = f"{model}+fallback"

    out_dir = Path(config.TAILOR_DIR).resolve()
    resume_docx = write_markdown_docx(resume_md, out_dir / f"{row['id']}-resume.docx")
    cover_docx = write_markdown_docx(cover, out_dir / f"{row['id']}-cover.docx")
    store.finish_tailor_job(
        row["id"],
        status="done",
        resume_id=resume["id"],
        profile_id=profile.get("id"),
        resume_md=resume_md,
        cover_letter_md=cover,
        notes_md=notes,
        model=model,
        resume_docx_uri=resume_docx,
        cover_docx_uri=cover_docx,
    )
    log.info("tailor %s: wrote docx", row["id"])
