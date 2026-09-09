"""Claude Code CLI adapter for personalized job-discovery web search
(Week 8). Shells out to the already-authenticated `claude` CLI in
headless (-p) mode instead of a new Anthropic/Bedrock API key -- the
local-only equivalent of CLAUDE.md rule 7's "thin adapter" -- swap this
file for a real API/Bedrock client once this moves to a cloud deployment.

Verified manually against a real installation (claude 2.1.259) before
relying on this: `--allowedTools WebSearch,WebFetch --permission-prompts
none` does not hang waiting for approval, and the tool genuinely
performs a live search (confirmed by asking it for today's actual date
and a same-day headline, both outside any training cutoff). Two
caveats found the same way:
- `usage.server_tool_use.web_search_requests` in the JSON envelope
  stayed 0 even on a confirmed-live search -- not reliable telemetry,
  don't build cost tracking on it (log total_cost_usd instead, below).
- `--output-format json` buffers everything until the whole multi-step
  search+verify finishes -- a real 8-result search took 423s. This is
  minutes, not seconds; CLAUDE_CLI_TIMEOUT_SECONDS must have real
  headroom or every run looks like a timeout.
"""

from __future__ import annotations

import json
import logging
import subprocess
import time

from jobsonar_agent import config

log = logging.getLogger("jobsonar.agent")


class ClaudeCLIError(RuntimeError):
    pass


class ClaudeCLISearch:
    def search(self, prompt: str) -> str:
        t0 = time.monotonic()
        try:
            proc = subprocess.run(
                [
                    config.CLAUDE_CLI_PATH, "-p", prompt,
                    "--output-format", "json",
                    "--allowedTools", "WebSearch,WebFetch",
                    "--permission-prompts", "none",
                ],
                capture_output=True,
                text=True,
                timeout=config.CLAUDE_CLI_TIMEOUT_SECONDS,
            )
        except FileNotFoundError as exc:
            raise ClaudeCLIError(f"'{config.CLAUDE_CLI_PATH}' CLI not found on PATH") from exc
        except subprocess.TimeoutExpired as exc:
            raise ClaudeCLIError(f"claude CLI timed out after {config.CLAUDE_CLI_TIMEOUT_SECONDS}s") from exc
        elapsed = time.monotonic() - t0
        if proc.returncode != 0:
            raise ClaudeCLIError(proc.stderr.strip() or f"claude CLI exited {proc.returncode}")
        try:
            data = json.loads(proc.stdout)
        except json.JSONDecodeError as exc:
            raise ClaudeCLIError("claude CLI did not return a valid JSON envelope") from exc
        if data.get("is_error"):
            raise ClaudeCLIError(str(data.get("result") or "claude CLI reported an error"))
        # Real spend, logged for visibility -- this is new premium-model
        # cost outside the shortlist-only Bedrock tiering (golden rule 4),
        # so it should be easy to see in the logs, not buried.
        log.info("claude CLI search: %.1fs, $%.4f", elapsed, data.get("total_cost_usd") or 0.0)
        return str(data.get("result") or "")
