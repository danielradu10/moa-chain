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
    "prompt_version": "labeler_v3",
    "allowed_subdomains": ["databases", "security", "non_related"],
    "transactions": [{"tx_hash": "0xabc", "prompt": "Design a rate-limited API."}],
}


def test_label_valid_single_transaction(label_client) -> None:
    client, fake = label_client
    fake.set_structured_response(_result("0xabc", "databases"))
    resp = client.post("/label", json=VALID_REQUEST)
    assert resp.status_code == 200
    data = resp.json()
    assert data["prompt_version"] == "labeler_v3"
    assert len(data["prompt_hash"]) == 64
    assert len(data["results"]) == 1
    assert data["results"][0]["tx_hash"] == "0xabc"
    assert data["results"][0]["labels"][0]["subdomain"] == "databases"
    assert data["results"][0]["labels"][0]["confidence"] == 0.9


def test_mocked_label_bypasses_provider(label_client) -> None:
    client, fake = label_client
    fake.set_error(AssertionError("provider must not be called"))
    client.app.state.config.mock_preprocessing_label = "systems_programming"
    try:
        request = {
            **VALID_REQUEST,
            "allowed_subdomains": ["systems_programming", "non_related"],
        }
        resp = client.post("/label", json=request)
        assert resp.status_code == 200
        assert resp.json()["results"][0]["labels"] == [
            {"subdomain": "systems_programming", "confidence": 1.0}
        ]
    finally:
        client.app.state.config.mock_preprocessing_label = ""


def test_label_valid_multiple_transactions(label_client) -> None:
    client, fake = label_client

    async def per_tx(system_prompt, user_payload, response_schema, timeout_seconds, operation=""):
        return _result(user_payload["tx_hash"], "databases")

    fake.structured_chat = per_tx

    resp = client.post("/label", json={
        "prompt_version": "labeler_v3",
        "allowed_subdomains": ["databases", "non_related"],
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
    resp = client.post("/label", json={**VALID_REQUEST, "prompt_version": "labeler_v999"})
    assert resp.status_code == 400
    assert resp.json()["error"] == "PROMPT_VERSION_MISMATCH"


def test_label_output_order_matches_input_order(label_client) -> None:
    client, fake = label_client

    async def delayed_per_tx(system_prompt, user_payload, response_schema, timeout_seconds, operation=""):
        tx_hash = user_payload["tx_hash"]
        delays = {"0x000": 0.05, "0x001": 0.02, "0x002": 0.0}
        await asyncio.sleep(delays[tx_hash])
        return _result(tx_hash, "databases")

    fake.structured_chat = delayed_per_tx

    resp = client.post("/label", json={
        "prompt_version": "labeler_v3",
        "allowed_subdomains": ["databases", "non_related"],
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


def test_label_non_related_pure_response_is_accepted(label_client) -> None:
    client, fake = label_client
    fake.set_structured_response(LabelResult(
        tx_hash="0xabc",
        labels=[LabelEntry(subdomain="non_related", confidence=0.95)],
    ))
    resp = client.post("/label", json=VALID_REQUEST)
    assert resp.status_code == 200
    result = resp.json()["results"][0]
    assert result["labels"][0]["subdomain"] == "non_related"


def test_label_real_label_dominates_non_related_by_confidence(label_client) -> None:
    client, fake = label_client
    # Real label has higher confidence than non_related → keep real label only.
    fake.set_structured_response(LabelResult(
        tx_hash="0xabc",
        labels=[
            LabelEntry(subdomain="non_related", confidence=0.5),
            LabelEntry(subdomain="databases", confidence=0.8),
        ],
    ))
    resp = client.post("/label", json=VALID_REQUEST)
    assert resp.status_code == 200
    result = resp.json()["results"][0]
    returned_subdomains = [e["subdomain"] for e in result["labels"]]
    assert returned_subdomains == ["databases"]


def test_label_non_related_dominates_real_label_by_confidence(label_client) -> None:
    client, fake = label_client
    # non_related has higher confidence than any real label → keep non_related only.
    fake.set_structured_response(LabelResult(
        tx_hash="0xabc",
        labels=[
            LabelEntry(subdomain="non_related", confidence=0.9),
            LabelEntry(subdomain="databases", confidence=0.3),
        ],
    ))
    resp = client.post("/label", json=VALID_REQUEST)
    assert resp.status_code == 200
    result = resp.json()["results"][0]
    returned_subdomains = [e["subdomain"] for e in result["labels"]]
    assert returned_subdomains == ["non_related"]


def test_label_more_than_three_subdomains_is_truncated_to_top_3_by_confidence(label_client) -> None:
    client, fake = label_client
    allowed = ["databases", "security", "systems_programming", "dev_ops", "non_related"]
    # Model returns 4 labels; service must keep the 3 with highest confidence.
    fake.set_structured_response(LabelResult(
        tx_hash="0xabc",
        labels=[
            LabelEntry(subdomain="databases", confidence=0.9),
            LabelEntry(subdomain="security", confidence=0.8),
            LabelEntry(subdomain="systems_programming", confidence=0.7),
            LabelEntry(subdomain="dev_ops", confidence=0.6),
        ],
    ))
    resp = client.post("/label", json={
        "prompt_version": "labeler_v3",
        "allowed_subdomains": allowed,
        "transactions": [{"tx_hash": "0xabc", "prompt": "Build a full-stack system."}],
    })
    assert resp.status_code == 200
    result = resp.json()["results"][0]
    returned_subdomains = [e["subdomain"] for e in result["labels"]]
    assert len(returned_subdomains) == 3
    assert "databases" in returned_subdomains
    assert "security" in returned_subdomains
    assert "systems_programming" in returned_subdomains
    assert "dev_ops" not in returned_subdomains
