from pydantic_settings import BaseSettings


class Settings(BaseSettings):
    llm_provider: str = "ollama"
    ollama_base_url: str = "http://127.0.0.1:11434"
    ollama_model: str = "qwen2.5-coder:7b"
    llm_temperature: float = 0.0
    llm_timeout_seconds: float = 60.0
    label_max_concurrency: int = 4
    answer_max_concurrency: int = 4
    judge_max_concurrency: int = 4

    model_config = {"env_file": ".env", "env_file_encoding": "utf-8"}


settings = Settings()
