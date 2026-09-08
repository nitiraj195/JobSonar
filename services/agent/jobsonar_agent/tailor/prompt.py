"""JD + local resume → tailored resume and cover letter.

Resume text is sent only to the local LLM (fake/ollama). Never log the
prompt (golden rule 5). Never apply on the seeker's behalf.
"""

from __future__ import annotations

import json
import re

from jobsonar_agent import config
from jobsonar_agent.graph.prompt import _first_object, _section, _try_json

_FENCE = re.compile(r"```(?:json)?\s*(.*?)\s*```", re.DOTALL | re.IGNORECASE)


def build_tailor_prompt(*, profile: dict, resume_text: str, jd: dict) -> str:
    skills = ", ".join(profile.get("skills") or []) or "(none)"
    resume = (resume_text or "")[: config.TAILOR_RESUME_CHARS]
    jd_body = (jd.get("jd_md") or "")[: config.TAILOR_JD_CHARS]
    return (
        "You are an assist-to-apply writer. Do not apply, submit, or contact the employer.\n"
        "Rewrite the resume and draft a cover letter tailored to this job description.\n\n"
        "resume_md MUST follow this exact structure (omit a section only if the resume truly "
        "has nothing for it):\n"
        "  # Full Name\n"
        "  email · phone · location · linkedin/portfolio   (one line, only fields present on the resume)\n"
        "  \n"
        "  ## Summary\n"
        "  2-3 sentences aimed at this role.\n"
        "  \n"
        "  ## Skills\n"
        "  Comma-separated skills already on the resume that this JD also asks for.\n"
        "  \n"
        "  ## Experience\n"
        "  Most relevant role first. One block per role:\n"
        "  **Company** — Title (Start–End)\n"
        "  - bullet\n"
        "  - bullet   (3-6 bullets per role, reworded from the original resume for this JD)\n"
        "  \n"
        "  ## Education\n"
        "  **School** — Degree (Year)\n\n"
        "Rules:\n"
        "- Do not invent employers, dates, titles, degrees, or certifications.\n"
        "- Reorder and reword existing bullets so JD keywords already on the resume are obvious; "
        "never fabricate new experience.\n"
        "- If the JD asks for something not in the resume, list it under notes_md — do not fabricate it.\n"
        "- Cover letter: 3 short paragraphs of plain prose (no headings or bullet lists), first "
        "person, no salary demands.\n"
        "- Output markdown only inside the JSON strings. Use **bold** only for the Company/School "
        "name at the start of each Experience/Education entry — nowhere else.\n\n"
        f"Profile skills: {skills}\n"
        f"Seniority: {profile.get('seniority') or '(unset)'}\n"
        f"Location: {profile.get('location') or '(unset)'}\n\n"
        f"Job title: {jd.get('title') or '(from JD)'}\n"
        f"Company: {jd.get('company') or '(from JD)'}\n"
        f"Job description:\n{jd_body}\n\n"
        f"Current resume text:\n{resume}\n\n"
        "Reply with JSON only:\n"
        '{"resume_md": "full tailored resume markdown",'
        ' "cover_letter_md": "cover letter markdown",'
        ' "notes_md": "what you emphasized and genuine gaps"}\n'
    )


def parse_tailor(text: str) -> tuple[str, str, str] | None:
    if not text or not text.strip():
        return None
    raw = text.strip()
    m = _FENCE.search(raw)
    if m:
        raw = m.group(1).strip()
    data = _try_json(raw)
    if data is None:
        data = _try_json(_first_object(raw))
    if isinstance(data, dict):
        resume = str(data.get("resume_md") or data.get("resume") or "").strip()
        cover = str(data.get("cover_letter_md") or data.get("cover_letter") or "").strip()
        notes = str(data.get("notes_md") or data.get("notes") or "").strip()
        if resume or cover:
            return resume, cover, notes
    resume = _section(text, ("tailored resume", "resume"))
    cover = _section(text, ("cover letter",))
    notes = _section(text, ("notes", "gaps"))
    if resume or cover:
        return resume, cover, notes
    return None
