"""LangGraph cascade: first_pass (Week 6 scoring, no LLM) → shortlist
→ deep_dive (LLM on strong jobs only).
"""

from __future__ import annotations

import logging
from typing import Any, Callable, TypedDict

from langgraph.graph import END, START, StateGraph

from jobsonar_agent import config
from jobsonar_agent.graph.prompt import build_prompt, parse_analysis
from jobsonar_agent.llm import LLM
from jobsonar_agent.llm.factory import is_premium_backend
from jobsonar_agent.otel import tracer
from jobsonar_agent.store import Store

log = logging.getLogger("jobsonar.agent")

FirstPassFn = Callable[[dict[str, Any]], dict[str, Any]]


class AgentState(TypedDict, total=False):
    first_pass: dict[str, int]
    shortlist_ids: list[str]
    shortlist_by_profile: dict[str, list[str]]
    deep_dive_calls: int
    premium_calls: int
    prompted_job_ids: list[str]


def build_graph(
    store: Store,
    llm: LLM | None,
    first_pass_fn: FirstPassFn,
) -> Any:
    def first_pass(state: AgentState) -> dict[str, Any]:
        with tracer().start_as_current_span("graph.first_pass"):
            counts = first_pass_fn(state)
        out = dict(counts) if counts else {}
        out.setdefault("premium_calls", 0)
        out.setdefault("deep_dive_calls", 0)
        out.setdefault("prompted_job_ids", [])
        return out

    def shortlist(state: AgentState) -> dict[str, Any]:
        with tracer().start_as_current_span("graph.shortlist") as span:
            by_profile: dict[str, list[str]] = {}
            flat: list[str] = []
            for profile in store.list_profiles():
                jobs = store.jobs_for_deep_dive(profile["id"], config.SHORTLIST_BAND)
                ids = [j["id"] for j in jobs]
                if ids:
                    by_profile[profile["id"]] = ids
                    flat.extend(ids)
            span.set_attribute("shortlist.size", len(flat))
            span.set_attribute("shortlist.band", config.SHORTLIST_BAND)
            return {"shortlist_by_profile": by_profile, "shortlist_ids": flat}

    def route_after_shortlist(state: AgentState) -> str:
        return "deep_dive" if state.get("shortlist_by_profile") else END

    def deep_dive(state: AgentState) -> dict[str, Any]:
        by_profile = dict(state.get("shortlist_by_profile") or {})
        if llm is None or not by_profile:
            return {"deep_dive_calls": 0, "premium_calls": 0, "prompted_job_ids": []}
        profiles = {p["id"]: p for p in store.list_profiles()}
        prompted: list[str] = []
        calls = 0
        premium = 0
        premium_on = is_premium_backend()
        with tracer().start_as_current_span("graph.deep_dive") as span:
            for profile_id, ids in by_profile.items():
                profile = profiles.get(profile_id)
                if not profile:
                    continue
                pending = store.jobs_for_deep_dive(profile_id, config.SHORTLIST_BAND)
                by_id = {j["id"]: j for j in pending}
                allowed = set(ids)
                for jid in ids:
                    if jid not in allowed:
                        continue
                    job = by_id.get(jid)
                    if not job:
                        continue
                    log.info("deep_dive job=%s profile=%s model=%s", jid, profile["name"], config.LLM_MODEL)
                    try:
                        raw = llm.complete(build_prompt(profile, job))
                    except Exception as exc:
                        log.warning("deep_dive job=%s failed: %s", jid, type(exc).__name__)
                        continue
                    calls += 1
                    if premium_on:
                        premium += 1
                    parsed = parse_analysis(raw)
                    if not parsed:
                        log.warning("deep_dive job=%s: unparseable output", jid)
                        continue
                    justification, tailoring = parsed
                    store.upsert_analysis(
                        jid,
                        profile_id,
                        justification_md=justification,
                        tailoring_md=tailoring,
                        model=config.LLM_MODEL if config.DEEP_DIVE_BACKEND != "fake" else "fake",
                    )
                    prompted.append(jid)
            span.set_attribute("deep_dive.calls", calls)
            span.set_attribute("deep_dive.premium_calls", premium)
        return {
            "deep_dive_calls": calls,
            "premium_calls": premium,
            "prompted_job_ids": prompted,
        }

    g = StateGraph(AgentState)
    g.add_node("first_pass", first_pass)
    g.add_node("shortlist", shortlist)
    g.add_node("deep_dive", deep_dive)
    g.add_edge(START, "first_pass")
    g.add_edge("first_pass", "shortlist")
    g.add_conditional_edges(
        "shortlist",
        route_after_shortlist,
        {"deep_dive": "deep_dive", END: END},
    )
    g.add_edge("deep_dive", END)
    return g.compile()


def run_cascade(store: Store, llm: LLM | None, first_pass_fn: FirstPassFn) -> AgentState:
    graph = build_graph(store, llm, first_pass_fn)
    return graph.invoke({})
