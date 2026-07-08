import asyncio

import pytest
from fastapi.testclient import TestClient

from errors import AgentServiceError, ErrorCode
from providers.fake_provider import FakeProvider
from schemas import LabelEntry, LabelResult


@pytest.fixture
def label_client(client: TestClient):
    fake = FakeProvider()
    client.app.state.provider = fake
    return client, fake


def _result(tx_hash: str, subdomain: str, confidence: float = 0.9) -> LabelResult:
    return LabelResult(
        tx_hash=tx_hash,
        labels=[LabelEntry(subdomain=subdomain, confidence=confidence)],
    )


VALID_REQUEST = {
    "prompt_version": "labeler_v1",
    "allowed_subdomains": ["databases", "security"],
    "transactions": [{"tx_hash": "0xabc", "prompt": "Design a rate-limited API."}],
}


def test_label_valid_single_transaction(label_client) -> None:
    client, fake = label_client
    fake.set_structured_response(_result("0xabc", "databases"))
    resp = client.post("/label", json=VALID_REQUEST)
    assert resp.status_code == 200
    data = resp.json()
    assert data["prompt_version"] == "labeler_v1"
    assert len(data["prompt_hash"]) == 64
    assert len(data["results"]) == 1
    assert data["results"][0]["tx_hash"] == "0xabc"
    assert data["results"][0]["labels"][0]["subdomain"] == "databases"
    assert data["results"][0]["labels"][0]["confidence"] == 0.9


def test_label_valid_multiple_transactions(label_client) -> None:
    client, fake = label_client

    async def per_tx(system_prompt, user_payload, response_schema, timeout_seconds):
        return _result(user_payload["tx_hash"], "databases")

    fake.structured_chat = per_tx

    resp = client.post("/label", json={
        "prompt_version": "labeler_v1",
        "allowed_subdomains": ["databases"],
        "transactions": [
            {"tx_hash": "0xaaa", "prompt": "Prompt A"},
            {"tx_hash": "0xbbb", "prompt": "Prompt B"},
        ],
    })
    assert resp.status_code == 200
    results = resp.json()["results"]
    assert len(results) == 2
    assert results[0]["tx_hash"] == "0xaaa"
    assert results[1]["tx_hash"] == "0xbbb"


def test_label_unknown_subdomain_returns_error(label_client) -> None:
    client, fake = label_client
    fake.set_structured_response(_result("0xabc", "not_a_real_domain"))
    resp = client.post("/label", json=VALID_REQUEST)
    assert resp.status_code == 400
    assert resp.json()["error"] == "UNKNOWN_SUBDOMAIN"


def test_label_coverage_mismatch_wrong_tx_hash(label_client) -> None:
    client, fake = label_client
    fake.set_structured_response(_result("0xwrong", "databases"))
    resp = client.post("/label", json=VALID_REQUEST)
    assert resp.status_code == 400
    assert resp.json()["error"] == "COVERAGE_MISMATCH"


def test_label_confidence_out_of_range(label_client) -> None:
    client, fake = label_client
    fake.set_structured_response(LabelResult(
        tx_hash="0xabc",
        labels=[LabelEntry(subdomain="databases", confidence=1.5)],
    ))
    resp = client.post("/label", json=VALID_REQUEST)
    assert resp.status_code == 400
    assert resp.json()["error"] == "INVALID_MODEL_OUTPUT"


def test_label_prompt_version_mismatch(label_client) -> None:
    client, fake = label_client
    fake.set_structured_response(_result("0xabc", "databases"))
    resp = client.post("/label", json={**VALID_REQUEST, "prompt_version": "labeler_v99"})
    assert resp.status_code == 400
    assert resp.json()["error"] == "PROMPT_VERSION_MISMATCH"


def test_label_output_order_matches_input_order(label_client) -> None:
    client, fake = label_client

    async def delayed_per_tx(system_prompt, user_payload, response_schema, timeout_seconds):
        tx_hash = user_payload["tx_hash"]
        delays = {"0x000": 0.05, "0x001": 0.02, "0x002": 0.0}
        await asyncio.sleep(delays[tx_hash])
        return _result(tx_hash, "databases")

    fake.structured_chat = delayed_per_tx

    resp = client.post("/label", json={
        "prompt_version": "labeler_v1",
        "allowed_subdomains": ["databases"],
        "transactions": [
            {"tx_hash": "0x000", "prompt": "Prompt 0"},
            {"tx_hash": "0x001", "prompt": "Prompt 1"},
            {"tx_hash": "0x002", "prompt": "Prompt 2"},
        ],
    })
    assert resp.status_code == 200
    assert [r["tx_hash"] for r in resp.json()["results"]] == ["0x000", "0x001", "0x002"]


def test_label_one_failed_call_fails_whole_endpoint(label_client) -> None:
    client, fake = label_client
    fake.set_error(AgentServiceError(ErrorCode.PROVIDER_ERROR, "Ollama unreachable", 500))
    resp = client.post("/label", json=VALID_REQUEST)
    assert resp.status_code == 500
    assert resp.json()["error"] == "PROVIDER_ERROR"
