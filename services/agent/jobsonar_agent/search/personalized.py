"""Personalized job discovery (Week 8). Builds a search prompt from
*derived* profile fields only -- skills/seniority/location/remote
preference, never raw resume text (golden rule 5) -- runs it through a
web-search-capable Claude, and parses the reply into normalized job
rows. Writing/deduping those rows into Postgres is store.py's job; this
module only talks to the search backend and parses its answer.
"""

from __future__ import annotations

import re

from jobsonar_agent import config
from jobsonar_agent.graph.prompt import _first_object, _try_json
from jobsonar_agent.llm.claude_cli import ClaudeCLISearch

_FENCE = re.compile(r"```(?:json)?\s*(.*?)\s*```", re.DOTALL | re.IGNORECASE)


def build_search_prompt(profile: dict, limit: int) -> str:
    skills = ", ".join(profile.get("skills") or []) or "unspecified"
    return (
        "I'm helping a job seeker find open roles. Please search the web for real, "
        "currently open job postings that match this profile "
        f"(skills: {skills}; seniority: {profile.get('seniority') or 'unspecified'}; "
        f"location preference: {profile.get('location') or 'unspecified'}; "
        f"remote preference: {profile.get('remote_pref') or 'unspecified'}).\n\n"
        f"Find up to {limit} postings from real job boards or company career pages "
        "(LinkedIn, Naukri, Indeed, Glassdoor, or a company's own careers page). "
        "Only include postings you actually found via web search with a real, "
        "working URL -- never invent a company, title, or link. If you're not sure "
        "a posting is still open, leave it out rather than guess.\n\n"
        "Reply with JSON only, no prose, no markdown code fences, in exactly this shape:\n"
        '{"jobs": [{"title": "", "company": "", "location": "", '
        '"remote_type": "remote|hybrid|onsite|", "source_url": "", "description_md": ""}]}\n'
        'If you find none, reply {"jobs": []}.'
    )


def parse_discovered_jobs(text: str) -> list[dict]:
    if not text or not text.strip():
        return []
    raw = text.strip()
    m = _FENCE.search(raw)
    if m:
        raw = m.group(1).strip()
    data = _try_json(raw) or _try_json(_first_object(raw))
    if not isinstance(data, dict):
        return []
    jobs = data.get("jobs")
    if not isinstance(jobs, list):
        return []
    out = []
    for j in jobs:
        if not isinstance(j, dict):
            continue
        title = str(j.get("title") or "").strip()
        company = str(j.get("company") or "").strip()
        url = str(j.get("source_url") or "").strip()
        # Incomplete rows are skipped, not stored half-formed -- a job
        # without a real title/company/URL isn't independently verifiable.
        if not title or not company or not url:
            continue
        out.append({
            "title": title,
            "company": company,
            "location": str(j.get("location") or "").strip(),
            "remote_type": str(j.get("remote_type") or "").strip(),
            "source_url": url,
            "description_md": str(j.get("description_md") or "").strip(),
        })
    return out


def discover_jobs_for_profile(profile: dict, searcher=None) -> list[dict]:
    searcher = searcher or ClaudeCLISearch()
    prompt = build_search_prompt(profile, config.PERSONALIZED_SEARCH_MAX_RESULTS)
    raw = searcher.search(prompt)
    return parse_discovered_jobs(raw)[: config.PERSONALIZED_SEARCH_MAX_RESULTS]
