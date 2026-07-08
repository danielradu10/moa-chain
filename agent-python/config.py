from pydantic_settings import BaseSettings


class Settings(BaseSettings):
    # Which LLM backend to use. Only "ollama" is supported today.
    llm_provider: str = "ollama"

    # Ollama server address. Each validator node runs its own local instance.
    ollama_base_url: str = "http://127.0.0.1:11434"

    # Model to use for all LLM calls. Must be pulled with `ollama pull <model>` first.
    ollama_model: str = "qwen2.5-coder:7b"

    # Temperature 0 makes the model deterministic — same input always produces the same output.
    llm_temperature: float = 0.0

    # Per-request timeout in seconds. Applied to every individual Ollama call.
    llm_timeout_seconds: float = 60.0

    # How many label/answer/judge LLM calls can be in-flight at the same time.
    # Set OLLAMA_NUM_PARALLEL on the Ollama process to the same value for full parallelism.
    label_max_concurrency: int = 4
    answer_max_concurrency: int = 4
    judge_max_concurrency: int = 4

    model_config = {"env_file": ".env", "env_file_encoding": "utf-8"}


settings = Settings()
