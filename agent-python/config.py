from pydantic_settings import BaseSettings


class Settings(BaseSettings):
    # Which LLM backend to use. Only "ollama" is supported today.
    llm_provider: str = "ollama"

    # Ollama server address. Each validator node runs its own local instance.
    ollama_base_url: str = "http://127.0.0.1:11434"

    # Model to use for all LLM calls. Must be pulled with `ollama pull <model>` first.
    ollama_model: str = "qwen2.5-coder:7b"

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

    # Per-request timeout in seconds. Applied to every individual Ollama call.
    llm_timeout_seconds: float = 60.0

    # Explicit Ollama generation bounds used by qualified MR2 judges.
    llm_num_ctx: int = 4096
    llm_num_predict: int = 256
    llm_think: bool = False

    # How many label/answer/judge LLM calls can be in-flight at the same time.
    # Set OLLAMA_NUM_PARALLEL on the Ollama process to the same value for full parallelism.
    label_max_concurrency: int = 4
    answer_max_concurrency: int = 4
    judge_max_concurrency: int = 4

    model_config = {"env_file": ".env", "env_file_encoding": "utf-8"}


settings = Settings()
