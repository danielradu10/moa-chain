from pydantic_settings import BaseSettings


class Settings(BaseSettings):
    # Which LLM backend to use: "ollama" or "openai".
    llm_provider: str = "ollama"

    # ── Ollama settings ──────────────────────────────────────────────────────
    # Ollama server address. Each validator node runs its own local instance.
    ollama_base_url: str = "http://127.0.0.1:11434"

    # Model to use when LLM_PROVIDER=ollama. Must be pulled with `ollama pull <model>` first.
    ollama_model: str = "qwen2.5-coder:7b"

    # Explicit Ollama generation bounds used by qualified MR2 judges.
    llm_num_ctx: int = 4096
    llm_num_predict: int = 256
    llm_think: bool = False

    # ── OpenAI settings ──────────────────────────────────────────────────────
    # Required when LLM_PROVIDER=openai.
    openai_api_key: str = ""

    # Model to use when LLM_PROVIDER=openai.
    openai_model: str = "gpt-4o-mini"

    # ── Shared LLM settings ──────────────────────────────────────────────────
    # Temperature controls the diversity of validator answers.
    # 0.0 = fully deterministic (all validators produce identical output — defeats MoA diversity).
    # 0.5 = balanced: answers are grounded but validators explore different approaches.
    # >0.8 = risks hallucinations, increasing INSUFFICIENT_CORRECT_ANSWERS round failures.
    #
    # TODO: run integration tests at 0.3, 0.5, 0.7 and compare:
    #   - answer variance across validators in MR2 (how different are the candidate answers?)
    #   - judge classification distribution (Correct / Hallucination / Wrong ratios)
    #   - final synthesized answer quality once MR3 (answer aggregation) is implemented
    llm_temperature: float = 0.5

    # Per-request timeout in seconds applied to every individual LLM call.
    llm_timeout_seconds: float = 60.0

    # How many label/answer/judge LLM calls can be in-flight at the same time.
    # For Ollama: set OLLAMA_NUM_PARALLEL on the Ollama process to the same value.
    label_max_concurrency: int = 4
    answer_max_concurrency: int = 4
    judge_max_concurrency: int = 4

    @property
    def model(self) -> str:
        """Active model name for the configured provider — used by health and logging."""
        if self.llm_provider == "openai":
            return self.openai_model
        return self.ollama_model

    model_config = {"env_file": ".env", "env_file_encoding": "utf-8"}


settings = Settings()
