from __future__ import annotations

from jobsonar_agent import config
from jobsonar_agent.llm import LLM
from jobsonar_agent.llm.fake import FakeLLM


def resolve_llm() -> LLM | None:
    """Deep-dive LLM. Bedrock is fail-closed without DEEP_DIVE_OPT_IN=1
    (returns None → graph does not call complete). Ollama/fake never
    count as premium."""
    backend = config.DEEP_DIVE_BACKEND
    if backend == "bedrock":
        if not config.DEEP_DIVE_OPT_IN:
            return None
        from jobsonar_agent.llm.bedrock import BedrockLLM

        return BedrockLLM()
    if backend == "ollama":
        from jobsonar_agent.llm.ollama import OllamaLLM

        return OllamaLLM()
    return FakeLLM()


def is_premium_backend() -> bool:
    return config.DEEP_DIVE_BACKEND == "bedrock" and config.DEEP_DIVE_OPT_IN


def resolve_tailor_llm() -> LLM:
    """Resume tailoring always stays local (golden rule 5)."""
    if config.DEEP_DIVE_BACKEND == "ollama":
        from jobsonar_agent.llm.ollama import OllamaLLM

        return OllamaLLM()
    return FakeLLM()
