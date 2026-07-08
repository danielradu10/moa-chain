from contextlib import asynccontextmanager
from pathlib import Path

import pytest
from fastapi import FastAPI
from fastapi.testclient import TestClient

from prompts.loader import ProtocolPrompt, load_protocol_prompt


def test_hash_is_stable_for_same_file(tmp_path: Path) -> None:
    (tmp_path / "stable_v1.txt").write_bytes(b"Hello, protocol!")
    p1 = load_protocol_prompt("stable_v1", prompts_dir=tmp_path)
    p2 = load_protocol_prompt("stable_v1", prompts_dir=tmp_path)
    assert p1.sha256_hash == p2.sha256_hash


def test_hash_changes_for_different_content(tmp_path: Path) -> None:
    (tmp_path / "content_a_v1.txt").write_bytes(b"Content A")
    (tmp_path / "content_b_v1.txt").write_bytes(b"Content B")
    p_a = load_protocol_prompt("content_a_v1", prompts_dir=tmp_path)
    p_b = load_protocol_prompt("content_b_v1", prompts_dir=tmp_path)
    assert p_a.sha256_hash != p_b.sha256_hash


def test_missing_prompt_file_raises_file_not_found(tmp_path: Path) -> None:
    with pytest.raises(FileNotFoundError):
        load_protocol_prompt("nonexistent_v1", prompts_dir=tmp_path)


def test_missing_prompt_raises_at_startup_not_at_request_time(tmp_path: Path) -> None:
    @asynccontextmanager
    async def bad_lifespan(a: FastAPI):
        load_protocol_prompt("missing_v1", prompts_dir=tmp_path)
        yield

    test_app = FastAPI(lifespan=bad_lifespan)

    with pytest.raises(FileNotFoundError):
        with TestClient(test_app):
            pass


def test_health_includes_prompt_versions_and_hashes(client: TestClient) -> None:
    data = client.get("/health").json()
    assert "prompt_versions" in data
    assert "prompt_hashes" in data
    assert "labeler_v1" in data["prompt_versions"]
    assert "answerer_v1" in data["prompt_versions"]
    assert "labeler_v1" in data["prompt_hashes"]
    assert "answerer_v1" in data["prompt_hashes"]


def test_prompt_hash_is_sha256_hex(client: TestClient) -> None:
    data = client.get("/health").json()
    for hash_val in data["prompt_hashes"].values():
        assert len(hash_val) == 64
        assert all(c in "0123456789abcdef" for c in hash_val)


def test_protocol_prompt_fields(tmp_path: Path) -> None:
    content = b"system prompt content"
    (tmp_path / "test_v1.txt").write_bytes(content)
    p = load_protocol_prompt("test_v1", prompts_dir=tmp_path)
    assert p.name == "test_v1"
    assert p.version == "test_v1"
    assert p.content == content.decode("utf-8")
    assert len(p.sha256_hash) == 64
