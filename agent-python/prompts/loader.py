import hashlib
from dataclasses import dataclass
from pathlib import Path

# Default directory: the prompts/ folder next to this file.
PROMPTS_DIR = Path(__file__).parent


@dataclass(frozen=True)
class ProtocolPrompt:
    name: str         # e.g. "labeler_v1"
    version: str      # same as name; used in request/response prompt_version fields
    content: str      # full prompt text sent as system_prompt to the LLM
    sha256_hash: str  # hex-encoded SHA-256 of the raw file bytes, returned to Go for verification


def load_protocol_prompt(name: str, prompts_dir: Path = PROMPTS_DIR) -> ProtocolPrompt:
    """Read a versioned prompt file, compute its SHA-256, and return a ProtocolPrompt.

    Raises FileNotFoundError immediately if the file does not exist.
    Called once at startup — never at request time.
    """
    path = prompts_dir / f"{name}.txt"

    # read_bytes() raises FileNotFoundError if the file is missing,
    # which propagates through the lifespan and prevents the service from starting.
    raw = path.read_bytes()

    sha256_hash = hashlib.sha256(raw).hexdigest()

    return ProtocolPrompt(
        name=name,
        version=name,
        content=raw.decode("utf-8"),
        sha256_hash=sha256_hash,
    )
