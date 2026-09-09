"""Unit tests for personalized search prompt-building/parsing (Week 8).
No network, no CLI subprocess -- the search backend is always injected
or monkeypatched here."""

from __future__ import annotations

from jobsonar_agent import config
from jobsonar_agent.search.personalized import (
    build_search_prompt,
    discover_jobs_for_profile,
    parse_discovered_jobs,
)

PROFILE = {
    "id": "p1",
    "name": "test",
    "skills": ["kubernetes", "terraform"],
    "seniority": "senior",
    "location": "Pune",
    "remote_pref": "remote",
}


def test_build_search_prompt_includes_profile_fields():
    prompt = build_search_prompt(PROFILE, 5)
    assert "kubernetes" in prompt
    assert "senior" in prompt
    assert "Pune" in prompt
    assert "up to 5" in prompt
    assert '"jobs"' in prompt


def test_build_search_prompt_handles_unset_fields():
    prompt = build_search_prompt({"id": "p2", "skills": []}, 3)
    assert "unspecified" in prompt


def test_parse_discovered_jobs_happy_path():
    text = (
        '{"jobs": [{"title": "SRE", "company": "Acme", "location": "Pune", '
        '"remote_type": "remote", "source_url": "https://example.test/1", '
        '"description_md": "Own on-call."}]}'
    )
    out = parse_discovered_jobs(text)
    assert len(out) == 1
    assert out[0]["title"] == "SRE"
    assert out[0]["company"] == "Acme"
    assert out[0]["source_url"] == "https://example.test/1"


def test_parse_discovered_jobs_strips_code_fence():
    text = '```json\n{"jobs": [{"title": "SRE", "company": "Acme", "source_url": "https://x.test/1"}]}\n```'
    out = parse_discovered_jobs(text)
    assert len(out) == 1
    assert out[0]["title"] == "SRE"


def test_parse_discovered_jobs_skips_incomplete_rows():
    text = (
        '{"jobs": ['
        '{"title": "SRE", "company": "Acme", "source_url": "https://x.test/1"},'
        '{"title": "", "company": "Acme", "source_url": "https://x.test/2"},'
        '{"title": "SRE2", "company": "", "source_url": "https://x.test/3"},'
        '{"title": "SRE3", "company": "Acme", "source_url": ""}'
        ']}'
    )
    out = parse_discovered_jobs(text)
    assert len(out) == 1
    assert out[0]["title"] == "SRE"


def test_parse_discovered_jobs_empty_or_garbage_returns_empty():
    assert parse_discovered_jobs("") == []
    assert parse_discovered_jobs("not json at all") == []
    assert parse_discovered_jobs('{"jobs": []}') == []
    assert parse_discovered_jobs('{"jobs": "not a list"}') == []


class _FakeSearcher:
    def __init__(self, text: str):
        self.text = text
        self.prompts: list[str] = []

    def search(self, prompt: str) -> str:
        self.prompts.append(prompt)
        return self.text


def test_discover_jobs_for_profile_caps_at_configured_limit(monkeypatch):
    monkeypatch.setattr(config, "PERSONALIZED_SEARCH_MAX_RESULTS", 2)
    jobs_json = ",".join(
        f'{{"title": "Job {i}", "company": "Acme", "source_url": "https://x.test/{i}"}}'
        for i in range(5)
    )
    fake = _FakeSearcher(f'{{"jobs": [{jobs_json}]}}')
    out = discover_jobs_for_profile(PROFILE, searcher=fake)
    assert len(out) == 2
    assert len(fake.prompts) == 1
    assert "Pune" in fake.prompts[0]


def test_discover_jobs_for_profile_returns_empty_on_no_results():
    fake = _FakeSearcher('{"jobs": []}')
    assert discover_jobs_for_profile(PROFILE, searcher=fake) == []
