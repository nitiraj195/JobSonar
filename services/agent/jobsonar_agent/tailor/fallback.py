"""Deterministic ATS rewrite from the stored resume + JD keywords.

Used when the local chat model is fake, unavailable, or returns a stub.
Segments the resume into the same section convention the prompt asks the
LLM for (contact line, Summary, Skills, Experience, Education) so the DOCX
renderer produces a real-looking resume either way. Never invents
employers, dates, or certs — only reorders/labels the seeker's own text.
"""

from __future__ import annotations

import re

from jobsonar_agent.resume.parse import extract_skills

_CONTACT_TOKENS = ("@", "http", "linkedin", "github.com", "tel:", "+", "gmail")
_DATE_RANGE = re.compile(
    r"(19|20)\d{2}\s*(?:[-–—]|to)\s*(present|(19|20)\d{2})", re.IGNORECASE
)
_SECTION_ALIASES = {
    "summary": "summary",
    "objective": "summary",
    "profile": "summary",
    "technical skills": "skills",
    "core skills": "skills",
    "skills": "skills",
    "professional experience": "experience",
    "work experience": "experience",
    "employment history": "experience",
    "experience": "experience",
    "education": "education",
    "academics": "education",
    "certifications": "certifications",
    "certificates": "certifications",
    "projects": "projects",
}
_SECTION_RE = re.compile(
    r"^[#*\s]*(" + "|".join(sorted(_SECTION_ALIASES, key=len, reverse=True)) + r")[:\s]*$",
    re.IGNORECASE,
)


def fallback_draft(*, resume_text: str, profile: dict, jd: dict) -> tuple[str, str, str]:
    body = (resume_text or "").strip()
    if not body:
        return "", "", "Resume file was empty."
    title = (jd.get("title") or "").strip() or "the posted role"
    company = (jd.get("company") or "").strip() or "the company"
    jd_skills = extract_skills((jd.get("jd_md") or "") + " " + title)
    profile_skills = [s for s in (profile.get("skills") or []) if s]
    matched = [s for s in jd_skills if s.lower() in {p.lower() for p in profile_skills}]
    missing = [s for s in jd_skills if s.lower() not in {p.lower() for p in profile_skills}]

    lines = [ln.strip() for ln in body.splitlines()]
    contact_lines = {ln for ln in lines if ln and _is_contact(ln)}
    contact = " · ".join(list(dict.fromkeys(ln for ln in lines if ln in contact_lines))[:3])
    name = next((ln for ln in lines if ln and ln not in contact_lines), "Candidate")

    sections = _split_sections(lines, skip={name} | contact_lines)
    exp_lines = sections.get("experience") or sections.get("_unsectioned") or []
    skills_line = _clean(sections.get("skills"))
    summary_line = _clean(sections.get("summary"))

    resume_parts = [f"# {name}"]
    if contact:
        resume_parts.append(contact)
    resume_parts += [
        "",
        "## Summary",
        summary_line or _summary(profile_skills, matched, title),
        "",
        "## Skills",
        skills_line or ", ".join(matched or profile_skills[:16] or ["(see original resume)"]),
        "",
        "## Experience",
        _experience_entries(exp_lines),
    ]
    if sections.get("education"):
        resume_parts += ["", "## Education", _bullets(sections["education"])]
    if sections.get("certifications"):
        resume_parts += ["", "## Certifications", _bullets(sections["certifications"])]
    resume_parts += ["", "---", "_Draft only. JobSonar does not submit this file._"]
    resume_md = "\n".join(resume_parts)

    cover = (
        f"Dear Hiring Manager,\n\n"
        f"I am writing for the {title} role at {company}. "
        f"My background already includes {', '.join(matched[:6]) or 'the skills listed on my resume'}, "
        f"which I have moved to the top of the attached resume.\n\n"
        f"I would welcome the chance to discuss how this experience maps to the posting.\n\n"
        f"Sincerely,\n{name}"
    )
    notes = (
        "Reordered your existing resume toward this JD. "
        + (f"Emphasized: {', '.join(matched)}. " if matched else "")
        + (f"JD asks, not clearly on resume: {', '.join(missing)}. " if missing else "")
        + "No employers or dates were invented. Download the DOCX — this is not submitted."
    )
    return resume_md, cover, notes


def _is_contact(ln: str) -> bool:
    low = ln.lower()
    return any(tok in low for tok in _CONTACT_TOKENS)


def _split_sections(lines: list[str], skip: set[str]) -> dict[str, list[str]]:
    buckets: dict[str, list[str]] = {}
    current = "_unsectioned"
    for ln in lines:
        if not ln or ln in skip:
            continue
        m = _SECTION_RE.match(ln)
        if m:
            current = _SECTION_ALIASES[m.group(1).lower()]
            buckets.setdefault(current, [])
            continue
        buckets.setdefault(current, []).append(ln)
    return buckets


def _clean(section_lines: list[str] | None) -> str:
    if not section_lines:
        return ""
    return " ".join(ln.lstrip("-* ").strip() for ln in section_lines if ln.strip())


def _bullets(section_lines: list[str]) -> str:
    rows = [f"- {ln.lstrip('-* ').strip()}" for ln in section_lines if ln.strip()]
    return "\n".join(rows) or "(see original resume)"


def _experience_entries(lines: list[str]) -> str:
    if not lines:
        return "(original resume text could not be split)"
    entries: list[list[str]] = []
    current: list[str] = []
    for ln in lines:
        if _DATE_RANGE.search(ln) and current:
            entries.append(current)
            current = [ln]
        else:
            current.append(ln)
    if current:
        entries.append(current)
    blocks: list[str] = []
    for entry in entries:
        header, *rest = entry
        blocks.append(f"**{header.strip(' -*')}**")
        for r in rest:
            r = r.lstrip("-* ").strip()
            if r:
                blocks.append(f"- {r}")
    return "\n".join(blocks)


def _summary(profile_skills: list[str], matched: list[str], title: str) -> str:
    lead = ", ".join((matched or profile_skills)[:8]) or "experience listed below"
    return (
        f"Operator targeting {title}. Strengths already on the resume: {lead}. "
        "Bullets below are the original content, reordered so overlapping keywords are obvious."
    )
