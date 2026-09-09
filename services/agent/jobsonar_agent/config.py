import os


def env(key: str, default: str = "") -> str:
    v = os.environ.get(key)
    return v if v else default


POSTGRES_DSN = env(
    "POSTGRES_DSN",
    "postgres://jobsonar:jobsonar@localhost:5432/jobsonar?sslmode=disable",
)
OLLAMA_HOST = env("OLLAMA_HOST", "http://localhost:11434").rstrip("/")
EMBED_MODEL = env("EMBED_MODEL", "nomic-embed-text")
EMBED_DIM = int(env("EMBED_DIM", "768"))
EMBED_BATCH = int(env("EMBED_BATCH", "32"))
EMBED_BACKEND = env("EMBED_BACKEND", "ollama")  # ollama | fake
EMBED_TEXT_CHARS = int(env("EMBED_TEXT_CHARS", "2000"))
OTEL_CONSOLE = env("OTEL_CONSOLE", "") == "1"
SCORE_BATCH = int(env("SCORE_BATCH", "128"))
LLM_MODEL = env("LLM_MODEL", "llama3.2")
# fake (tests / no chat model) | ollama (local) | bedrock (opt-in only)
DEEP_DIVE_BACKEND = env("DEEP_DIVE_BACKEND", "fake").lower()
DEEP_DIVE_OPT_IN = env("DEEP_DIVE_OPT_IN", "") == "1"
SHORTLIST_BAND = env("SHORTLIST_BAND", "strong")
BEDROCK_MODEL = env("BEDROCK_MODEL", "anthropic.claude-3-haiku-20240307-v1:0")
AWS_REGION = env("AWS_REGION", "us-east-1")
DEEP_DIVE_DESC_CHARS = int(env("DEEP_DIVE_DESC_CHARS", "4000"))
# Tailor drafts stay on the local LLM. Resume text never goes to Bedrock.
TAILOR_RESUME_CHARS = int(env("TAILOR_RESUME_CHARS", "12000"))
TAILOR_JD_CHARS = int(env("TAILOR_JD_CHARS", "8000"))
TAILOR_DIR = env("TAILOR_DIR", "./data/tailor")

# Week 8: personalized web search (agent-driven, per profile). Off by
# default (fail-closed, same style as DEEP_DIVE_OPT_IN) -- it's a new
# source type outside "aggregator API or ATS endpoint" (golden rule 2)
# and new premium spend outside the shortlist-only Bedrock tiering
# (golden rule 4), both deliberate, flagged deviations, not defaults.
PERSONALIZED_SEARCH_OPT_IN = env("PERSONALIZED_SEARCH_OPT_IN", "") == "1"
PERSONALIZED_SEARCH_INTERVAL_HOURS = float(env("PERSONALIZED_SEARCH_INTERVAL_HOURS", "24"))
PERSONALIZED_SEARCH_MAX_RESULTS = int(env("PERSONALIZED_SEARCH_MAX_RESULTS", "8"))
# Shells out to the already-authenticated Claude Code CLI instead of a new
# Anthropic/Bedrock API key -- revisit with a real API client once this
# moves to a cloud deployment (CLAUDE.md "when unsure": flagged, not a
# silent workaround).
# A real run (8-result cap, one profile) measured 423s end to end --
# --output-format json buffers everything until the whole multi-step
# search+verify completes, so there's no partial output before then.
# 180s cut real runs off mid-search; 600s leaves headroom.
CLAUDE_CLI_PATH = env("CLAUDE_CLI_PATH", "claude")
CLAUDE_CLI_TIMEOUT_SECONDS = int(env("CLAUDE_CLI_TIMEOUT_SECONDS", "600"))
