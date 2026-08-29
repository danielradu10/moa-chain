import pytest

from config import Settings
from providers.factory import create_provider
from providers.ollama_provider import OllamaProvider


def _settings(**overrides) -> Settings:
    """Build a Settings instance with test-safe defaults."""
    return Settings(
        llm_provider=overrides.pop("llm_provider", "ollama"),
        ollama_base_url=overrides.pop("ollama_base_url", "http://localhost:11434"),
        ollama_model=overrides.pop("ollama_model", "test-model"),
        openai_api_key=overrides.pop("openai_api_key", ""),
        openai_model=overrides.pop("openai_model", "gpt-4o-mini"),
        **overrides,
    )


# ── ollama ────────────────────────────────────────────────────────────────────

def test_factory_returns_ollama_provider():
    provider = create_provider(_settings(llm_provider="ollama"))
    assert isinstance(provider, OllamaProvider)


def test_factory_ollama_is_default():
    provider = create_provider(_settings())
    assert isinstance(provider, OllamaProvider)


def test_factory_ollama_case_insensitive():
    provider = create_provider(_settings(llm_provider="Ollama"))
    assert isinstance(provider, OllamaProvider)


# ── openai ────────────────────────────────────────────────────────────────────

def test_factory_openai_missing_api_key_raises():
    with pytest.raises(ValueError, match="OPENAI_API_KEY"):
        create_provider(_settings(llm_provider="openai", openai_api_key=""))


def test_factory_openai_with_key_returns_provider():
    from providers.openai_provider import OpenAIProvider  # noqa: PLC0415

    provider = create_provider(_settings(llm_provider="openai", openai_api_key="sk-test"))
    assert isinstance(provider, OpenAIProvider)


# ── unknown provider ──────────────────────────────────────────────────────────

def test_factory_unknown_provider_raises():
    with pytest.raises(ValueError, match="Unknown LLM_PROVIDER"):
        create_provider(_settings(llm_provider="gemini"))


def test_factory_empty_provider_raises():
    with pytest.raises(ValueError, match="Unknown LLM_PROVIDER"):
        create_provider(_settings(llm_provider=""))
