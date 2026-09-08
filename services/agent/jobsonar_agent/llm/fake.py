from __future__ import annotations

import json

from jobsonar_agent.llm import LLM


class FakeLLM:
    """Deterministic deep-dive text. No network. Used by tests and when
    Ollama has no chat model (same role as FakeEmbedder)."""

    model = "fake"

    def complete(self, prompt: str, **kw) -> str:
        if "cover_letter_md" in prompt or "tailor this resume" in prompt.lower():
            return json.dumps({
                "resume_md": (
                    "# Jordan Casey\n"
                    "jordan.casey@example.com · +1 555-010-2938 · Remote · linkedin.com/in/jordancasey\n\n"
                    "## Summary\n"
                    "Local draft aligned to this posting from skills already on the resume — "
                    "JobSonar never submits this file.\n\n"
                    "## Skills\n"
                    "Kubernetes, Terraform, AWS, CI/CD, Python\n\n"
                    "## Experience\n"
                    "**Acme Cloud** — Senior Site Reliability Engineer (2021–Present)\n"
                    "- Ran the on-call rotation for a Kubernetes platform serving production traffic.\n"
                    "- Automated infrastructure with Terraform, cutting manual provisioning time.\n"
                    "- Partnered with product teams to ship CI/CD pipelines used company-wide.\n\n"
                    "## Education\n"
                    "**State University** — B.S. Computer Science (2016)"
                ),
                "cover_letter_md": (
                    "Dear Hiring Manager,\n\n"
                    "I am applying for this role and have mapped my existing experience "
                    "to the posted requirements. I have not invented employers or dates.\n\n"
                    "Thank you for your time."
                ),
                "notes_md": (
                    "Emphasized overlapping keywords from the JD. "
                    "Gaps stay in this note; do not submit from this tool."
                ),
            })
        return json.dumps({
            "justification_md": (
                "You already cover the matched skills on this posting. "
                "The composite score is a strong fit on the deterministic first pass; "
                "this note does not change the rank."
            ),
            "tailoring_md": (
                "Lead the summary with the matched skills. "
                "Close gaps listed as job-asks-not-on-profile with one concrete example each. "
                "Do not submit an application from this tool."
            ),
        })


class CountingLLM:
    """Wraps an LLM and counts complete() calls for the NFR-1 cost guard."""

    def __init__(self, inner: LLM):
        self.inner = inner
        self.calls = 0
        self.prompts: list[str] = []

    def complete(self, prompt: str, **kw) -> str:
        self.calls += 1
        self.prompts.append(prompt)
        return self.inner.complete(prompt, **kw)
