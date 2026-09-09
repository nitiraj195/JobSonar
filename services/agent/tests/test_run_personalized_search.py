"""run.run_personalized_search: opt-in gate, interval gate, and the
"always advance the timestamp, even on failure" backoff -- all against
an in-memory fake store, no network/CLI/DB."""

from __future__ import annotations

from datetime import datetime, timedelta, timezone

from jobsonar_agent import config, run


class _FakeStore:
    def __init__(self, profiles):
        self._profiles = profiles
        self.discovered: list[tuple[dict, str]] = []
        self.marked: list[str] = []

    def list_profiles(self):
        return self._profiles

    def upsert_discovered_job(self, job, profile_id):
        self.discovered.append((job, profile_id))
        return "job-id"

    def mark_personalized_search_run(self, profile_id):
        self.marked.append(profile_id)


def _profile(name="amol", last=None):
    return {"id": f"id-{name}", "name": name, "skills": [], "last_personalized_search_at": last}


def test_opt_in_off_by_default_does_nothing(monkeypatch):
    monkeypatch.setattr(config, "PERSONALIZED_SEARCH_OPT_IN", False)
    store = _FakeStore([_profile()])
    assert run.run_personalized_search(store) == 0
    assert store.marked == []


def test_never_searched_profile_runs_when_opted_in(monkeypatch):
    monkeypatch.setattr(config, "PERSONALIZED_SEARCH_OPT_IN", True)
    monkeypatch.setattr(
        run, "discover_jobs_for_profile",
        lambda profile: [{"title": "SRE", "company": "Acme", "source_url": "https://x.test/1"}],
    )
    store = _FakeStore([_profile(last=None)])
    total = run.run_personalized_search(store)
    assert total == 1
    assert len(store.discovered) == 1
    assert store.marked == ["id-amol"]


def test_recently_searched_profile_is_skipped(monkeypatch):
    monkeypatch.setattr(config, "PERSONALIZED_SEARCH_OPT_IN", True)
    monkeypatch.setattr(config, "PERSONALIZED_SEARCH_INTERVAL_HOURS", 24)
    called = []
    monkeypatch.setattr(run, "discover_jobs_for_profile", lambda profile: called.append(profile) or [])
    recent = datetime.now(timezone.utc) - timedelta(hours=1)
    store = _FakeStore([_profile(last=recent)])
    total = run.run_personalized_search(store)
    assert total == 0
    assert called == []
    assert store.marked == []  # not due -- never even touched


def test_overdue_profile_runs_again(monkeypatch):
    monkeypatch.setattr(config, "PERSONALIZED_SEARCH_OPT_IN", True)
    monkeypatch.setattr(config, "PERSONALIZED_SEARCH_INTERVAL_HOURS", 24)
    monkeypatch.setattr(run, "discover_jobs_for_profile", lambda profile: [])
    stale = datetime.now(timezone.utc) - timedelta(hours=25)
    store = _FakeStore([_profile(last=stale)])
    run.run_personalized_search(store)
    assert store.marked == ["id-amol"]


def test_failure_still_advances_timestamp_so_it_does_not_retry_every_tick(monkeypatch):
    monkeypatch.setattr(config, "PERSONALIZED_SEARCH_OPT_IN", True)

    def boom(profile):
        raise RuntimeError("cli unavailable")

    monkeypatch.setattr(run, "discover_jobs_for_profile", boom)
    store = _FakeStore([_profile(last=None)])
    total = run.run_personalized_search(store)
    assert total == 0
    assert store.marked == ["id-amol"]  # advanced despite the failure


def test_each_profile_handled_independently(monkeypatch):
    monkeypatch.setattr(config, "PERSONALIZED_SEARCH_OPT_IN", True)

    def fake_discover(profile):
        if profile["name"] == "amol":
            raise RuntimeError("boom")
        return [{"title": "SRE", "company": "Acme", "source_url": "https://x.test/1"}]

    monkeypatch.setattr(run, "discover_jobs_for_profile", fake_discover)
    store = _FakeStore([_profile("amol"), _profile("nitiraj")])
    total = run.run_personalized_search(store)
    assert total == 1  # only nitiraj's result counted
    assert sorted(store.marked) == ["id-amol", "id-nitiraj"]  # both still advanced
