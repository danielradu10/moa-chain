from config import Settings
from providers.base import LLMProvider
from providers.ollama_provider import OllamaProvider


def create_provider(settings: Settings) -> LLMProvider:
    """Instantiate and return the LLM provider selected by settings.llm_provider.

    Raises ValueError for unknown providers or missing required configuration
    (e.g. OPENAI_API_KEY not set when LLM_PROVIDER=openai).
    This is called once at startup inside the FastAPI lifespan — a ValueError
    here aborts startup immediately rather than failing at the first request.
    """
    name = settings.llm_provider.strip().lower()

    if name == "ollama":
        return OllamaProvider(
            base_url=settings.ollama_base_url,
            model=settings.ollama_model,
            temperature=settings.llm_temperature,
            num_ctx=settings.llm_num_ctx,
            num_predict=settings.llm_num_predict,
            think=settings.llm_think,
        )

    if name == "openai":
        # Validate before attempting to import so the error message is clear.
        if not settings.openai_api_key:
            raise ValueError(
                "LLM_PROVIDER=openai requires OPENAI_API_KEY to be set"
            )
        # Imported here so that the openai package is only required when actually used.
        from providers.openai_provider import OpenAIProvider  # noqa: PLC0415

        return OpenAIProvider(
            api_key=settings.openai_api_key,
            model=settings.openai_model,
            temperature=settings.llm_temperature,
            timeout_seconds=settings.llm_timeout_seconds,
        )

    raise ValueError(
        f"Unknown LLM_PROVIDER={settings.llm_provider!r}. "
        "Supported values: 'ollama', 'openai'."
    )
