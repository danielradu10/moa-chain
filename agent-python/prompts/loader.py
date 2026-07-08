import hashlib
from dataclasses import dataclass
from pathlib import Path

PROMPTS_DIR = Path(__file__).parent


@dataclass(frozen=True)
class ProtocolPrompt:
    name: str
    version: str
    content: str
    sha256_hash: str


def load_protocol_prompt(name: str, prompts_dir: Path = PROMPTS_DIR) -> ProtocolPrompt:
    path = prompts_dir / f"{name}.txt"
    raw = path.read_bytes()
    sha256_hash = hashlib.sha256(raw).hexdigest()
    return ProtocolPrompt(
        name=name,
        version=name,
        content=raw.decode("utf-8"),
        sha256_hash=sha256_hash,
    )
