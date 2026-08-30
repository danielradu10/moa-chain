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
        anthropic_api_key=overrides.pop("anthropic_api_key", ""),
        anthropic_model=overrides.pop("anthropic_model", "claude-sonnet-4-6"),
        anthropic_effort=overrides.pop("anthropic_effort", "medium"),
        gemini_api_key=overrides.pop("gemini_api_key", ""),
        gemini_model=overrides.pop("gemini_model", "gemini-2.0-flash"),
        deepseek_api_key=overrides.pop("deepseek_api_key", ""),
        deepseek_model=overrides.pop("deepseek_model", "deepseek-v4-flash"),
        deepseek_base_url=overrides.pop(
            "deepseek_base_url", "https://api.deepseek.com"
        ),
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


# ── anthropic ─────────────────────────────────────────────────────────────────

def test_factory_anthropic_missing_api_key_raises():
    with pytest.raises(ValueError, match="ANTHROPIC_API_KEY"):
        create_provider(_settings(llm_provider="anthropic", anthropic_api_key=""))


def test_factory_anthropic_with_key_returns_provider():
    from providers.anthropic_provider import AnthropicProvider  # noqa: PLC0415

    provider = create_provider(_settings(llm_provider="anthropic", anthropic_api_key="sk-ant-test"))
    assert isinstance(provider, AnthropicProvider)


def test_factory_anthropic_case_insensitive():
    from providers.anthropic_provider import AnthropicProvider  # noqa: PLC0415

    provider = create_provider(_settings(llm_provider="Anthropic", anthropic_api_key="sk-ant-test"))
    assert isinstance(provider, AnthropicProvider)


# ── gemini ────────────────────────────────────────────────────────────────────

def test_factory_gemini_missing_api_key_raises():
    with pytest.raises(ValueError, match="GEMINI_API_KEY"):
        create_provider(_settings(llm_provider="gemini", gemini_api_key=""))


def test_factory_gemini_with_key_returns_provider():
    from providers.gemini_provider import GeminiProvider  # noqa: PLC0415

    provider = create_provider(_settings(llm_provider="gemini", gemini_api_key="ai-test"))
    assert isinstance(provider, GeminiProvider)


def test_factory_gemini_case_insensitive():
    from providers.gemini_provider import GeminiProvider  # noqa: PLC0415

    provider = create_provider(_settings(llm_provider="Gemini", gemini_api_key="ai-test"))
    assert isinstance(provider, GeminiProvider)


# ── deepseek ──────────────────────────────────────────────────────────────────

def test_factory_deepseek_missing_api_key_raises():
    with pytest.raises(ValueError, match="DEEPSEEK_API_KEY"):
        create_provider(_settings(llm_provider="deepseek", deepseek_api_key=""))


def test_factory_deepseek_with_key_returns_provider():
    from providers.deepseek_provider import DeepSeekProvider  # noqa: PLC0415

    provider = create_provider(
        _settings(llm_provider="deepseek", deepseek_api_key="ds-test")
    )
    assert isinstance(provider, DeepSeekProvider)
    assert provider.base_url == "https://api.deepseek.com"


def test_factory_deepseek_case_insensitive_and_model_for_health():
    from providers.deepseek_provider import DeepSeekProvider  # noqa: PLC0415

    cfg = _settings(
        llm_provider="DeepSeek",
        deepseek_api_key="ds-test",
        deepseek_model="deepseek-v4-pro",
    )
    assert isinstance(create_provider(cfg), DeepSeekProvider)
    assert cfg.model == "deepseek-v4-pro"


# ── unknown provider ──────────────────────────────────────────────────────────

def test_factory_unknown_provider_raises():
    with pytest.raises(ValueError, match="Unknown LLM_PROVIDER"):
        create_provider(_settings(llm_provider="bedrock"))


def test_factory_empty_provider_raises():
    with pytest.raises(ValueError, match="Unknown LLM_PROVIDER"):
        create_provider(_settings(llm_provider=""))
