"""NFR-1: premium/deep-dive complete() count tracks shortlist size, not
total jobs. Stretch/possible/excluded jobs never get a prompt.
"""

from __future__ import annotations

from jobsonar_agent.graph.cascade import run_cascade
from jobsonar_agent.llm.fake import CountingLLM, FakeLLM


class _MemStore:
    def __init__(self, jobs: list[dict], profile: dict):
        self._jobs = jobs
        self._profile = profile
        self.analyses: list[dict] = []
        self.bands: list[str] = []

    def list_profiles(self):
        return [self._profile]

    def jobs_for_deep_dive(self, profile_id: str, band: str):
        self.bands.append(band)
        return [j for j in self._jobs if j["band"] == band]

    def upsert_analysis(self, job_id, profile_id, **kw):
        self.analyses.append({"job_id": job_id, "profile_id": profile_id, **kw})


def _jobs():
    strong = [
        {
            "id": "s1",
            "title": "Senior DevOps",
            "company": "A",
            "description_md": "kubernetes terraform",
            "matched_skills": ["kubernetes"],
            "missing_skills": [],
            "band": "strong",
        },
        {
            "id": "s2",
            "title": "SRE Lead",
            "company": "B",
            "description_md": "prometheus grafana",
            "matched_skills": ["prometheus"],
            "missing_skills": ["kafka"],
            "band": "strong",
        },
    ]
    rest = []
    for i, band in enumerate(["stretch"] * 6 + ["possible"] * 2):
        rest.append({
            "id": f"x{i}",
            "title": f"Other {i}",
            "company": "Z",
            "description_md": "sales quota",
            "matched_skills": [],
            "missing_skills": ["java"],
            "band": band,
        })
    return strong + rest


PROFILE = {
    "id": "p1",
    "name": "test",
    "skills": ["kubernetes", "prometheus"],
    "seniority": None,
    "location": "Pune",
    "remote_pref": None,
}


def test_nfr1_deep_dive_calls_equal_shortlist_not_all_jobs():
    store = _MemStore(_jobs(), PROFILE)
    llm = CountingLLM(FakeLLM())
    out = run_cascade(store, llm, first_pass_fn=lambda s: {"first_pass": {}})
    shortlist = out["shortlist_ids"]
    assert store.bands == ["strong", "strong"]  # shortlist node + deep_dive reload
    assert set(shortlist) == {"s1", "s2"}
    assert llm.calls == 2
    assert llm.calls == len(shortlist)
    assert llm.calls < len(store._jobs)
    assert set(out["prompted_job_ids"]) == {"s1", "s2"}
    assert out["premium_calls"] == 0  # fake is not Bedrock
    prompted = "\n".join(llm.prompts)
    assert "s1" not in prompted  # we don't put ids in the prompt; titles instead
    for jid in ("x0", "x1", "x7"):
        assert jid not in out["prompted_job_ids"]
    assert "sales quota" not in prompted
    assert "kubernetes" in prompted or "prometheus" in prompted
    assert len(store.analyses) == 2


def test_nfr1_empty_shortlist_skips_complete():
    only_stretch = [j for j in _jobs() if j["band"] != "strong"]
    store = _MemStore(only_stretch, PROFILE)
    llm = CountingLLM(FakeLLM())
    out = run_cascade(store, llm, first_pass_fn=lambda s: {"first_pass": {}})
    assert out.get("shortlist_ids") == []
    assert llm.calls == 0
    assert store.analyses == []


def test_nfr1_bedrock_opt_in_unset_means_zero_calls(monkeypatch):
    monkeypatch.setattr("jobsonar_agent.config.DEEP_DIVE_BACKEND", "bedrock")
    monkeypatch.setattr("jobsonar_agent.config.DEEP_DIVE_OPT_IN", False)
    from jobsonar_agent.llm.factory import resolve_llm

    assert resolve_llm() is None
    store = _MemStore(_jobs(), PROFILE)
    llm = CountingLLM(FakeLLM())
    # once() would pass None, not the counting wrapper
    out = run_cascade(store, None, first_pass_fn=lambda s: {"first_pass": {}})
    assert set(out["shortlist_ids"]) == {"s1", "s2"}
    assert out["deep_dive_calls"] == 0
    assert out["premium_calls"] == 0
    assert llm.calls == 0
    assert store.analyses == []


def test_nfr1_premium_counter_only_when_bedrock_opted_in(monkeypatch):
    monkeypatch.setattr("jobsonar_agent.config.DEEP_DIVE_BACKEND", "bedrock")
    monkeypatch.setattr("jobsonar_agent.config.DEEP_DIVE_OPT_IN", True)
    store = _MemStore(_jobs(), PROFILE)
    llm = CountingLLM(FakeLLM())
    out = run_cascade(store, llm, first_pass_fn=lambda s: {"first_pass": {}})
    assert llm.calls == 2
    assert out["premium_calls"] == 2
    assert out["premium_calls"] <= len(out["shortlist_ids"])
