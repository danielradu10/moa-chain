import json

from pydantic_settings import BaseSettings


class Settings(BaseSettings):
    # Which LLM backend to use: "ollama" or "openai".
    llm_provider: str = "ollama"
    mock_model: str = "mocked-agent"

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

    # ── Anthropic settings ────────────────────────────────────────────────────
    # Required when LLM_PROVIDER=anthropic.
    anthropic_api_key: str = ""

    # Model to use when LLM_PROVIDER=anthropic.
    anthropic_model: str = "claude-sonnet-4-6"

    # Maximum tokens Anthropic is allowed to generate per call.
    # Anthropic requires this to be set explicitly; 4096 is a safe default
    # that covers all five MoA operations without being wasteful.
    anthropic_max_tokens: int = 4096

    # Output effort level for Anthropic SDK v1.2+ (optional, model-dependent).
    # When set, passed as output_config.effort. Leave empty for models that
    # do not support it (e.g. claude-haiku-4-5).
    # Valid values: "low", "medium", "high", "xhigh", "max".
    anthropic_effort: str = ""

    # ── Google Gemini settings ────────────────────────────────────────────────
    # Required when LLM_PROVIDER=gemini.
    gemini_api_key: str = ""

    # Model to use when LLM_PROVIDER=gemini.
    gemini_model: str = "gemini-2.0-flash"

    # ── DeepSeek settings ────────────────────────────────────────────────────
    # Required when LLM_PROVIDER=deepseek.
    deepseek_api_key: str = ""

    # Model and official OpenAI-compatible API endpoint.
    deepseek_model: str = "deepseek-v4-flash"
    deepseek_base_url: str = "https://api.deepseek.com"

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

    # ── Experiment recording ──────────────────────────────────────────────────
    # Set EXPERIMENT_DIR to the run directory to enable per-call JSONL recording.
    # Leave empty to disable (default for normal operation and qualification tests).
    experiment_dir: str = ""
    # Stable identity of this validator process — recorded in every call record.
    validator_id: str = ""
    validator_name: str = ""
    # Public experiment identity. It can differ from llm_provider/model when a
    # deterministic mock owns preprocessing and a separately declared real
    # provider is used only for judging/MR3.
    agent_provider: str = ""
    agent_model: str = ""
    # The HTTP endpoint this agent is serving on — recorded in manifests.
    agent_endpoint: str = ""

    # Optional deterministic preprocessing override. These values bypass the
    # provider only for /label and /answer; judging and MR3 remain real.
    mock_preprocessing_label: str = ""
    mock_preprocessing_answer: str = ""
    # JSON array of answers a deterministic Byzantine judge treats as CORRECT.
    # Empty preserves the original one-Byzantine behavior by matching only the
    # local preprocessing answer.
    mock_judge_correct_answers: str = ""
    # Selects the real-provider adversarial prompt only for /synthesize.
    byzantine_mr3_synthesis: bool = False

    @property
    def mocked_judge_correct_answer_set(self) -> set[str]:
        if not self.mock_judge_correct_answers:
            return {self.mock_preprocessing_answer}
        answers = json.loads(self.mock_judge_correct_answers)
        if not isinstance(answers, list) or not all(isinstance(answer, str) for answer in answers):
            raise ValueError("MOCK_JUDGE_CORRECT_ANSWERS must be a JSON array of strings")
        return set(answers)

    @property
    def model(self) -> str:
        """Active model name for the configured provider — used by health and logging."""
        provider = self.llm_provider.strip().lower()
        if provider == "mock":
            return self.mock_model
        if provider == "openai":
            return self.openai_model
        if provider == "anthropic":
            return self.anthropic_model
        if provider == "gemini":
            return self.gemini_model
        if provider == "deepseek":
            return self.deepseek_model
        return self.ollama_model

    @property
    def reported_provider(self) -> str:
        return self.agent_provider or self.llm_provider

    @property
    def reported_model(self) -> str:
        return self.agent_model or self.model

    model_config = {"env_file": ".env", "env_file_encoding": "utf-8"}


settings = Settings()
