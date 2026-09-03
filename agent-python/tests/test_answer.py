import asyncio

import pytest
from fastapi.testclient import TestClient

from errors import AgentServiceError, ErrorCode
from providers.fake_provider import FakeProvider
from schemas import AnswerResult


@pytest.fixture
def answer_client(client: TestClient):
    fake = FakeProvider()
    client.app.state.provider = fake
    return client, fake


def _result(tx_hash: str, answer: str = "Here is the solution.") -> AnswerResult:
    return AnswerResult(tx_hash=tx_hash, answer=answer)


VALID_REQUEST = {
    "prompt_version": "answerer_v1",
    "transactions": [
        {
            "tx_hash": "0xabc",
            "prompt": "Design a rate-limited API.",
            "subdomains": ["back_end_with_apis"],
        }
    ],
}


def test_answer_valid_single_transaction(answer_client) -> None:
    client, fake = answer_client
    fake.set_structured_response(_result("0xabc"))
    resp = client.post("/answer", json=VALID_REQUEST)
    assert resp.status_code == 200
    data = resp.json()
    assert data["prompt_version"] == "answerer_v1"
    assert len(data["prompt_hash"]) == 64
    assert len(data["results"]) == 1
    assert data["results"][0]["tx_hash"] == "0xabc"
    assert data["results"][0]["answer"] == "Here is the solution."


def test_mocked_answer_bypasses_provider(answer_client) -> None:
    client, fake = answer_client
    fake.set_error(AssertionError("provider must not be called"))
    wrong = "Deterministic wrong answer."
    client.app.state.config.mock_preprocessing_answer = wrong
    try:
        resp = client.post("/answer", json=VALID_REQUEST)
        assert resp.status_code == 200
        assert resp.json()["results"][0]["answer"] == wrong
    finally:
        client.app.state.config.mock_preprocessing_answer = ""


def test_answer_valid_multiple_transactions(answer_client) -> None:
    client, fake = answer_client

    async def per_tx(system_prompt, user_payload, response_schema, timeout_seconds, operation=""):
        return _result(user_payload["tx_hash"], f"Answer for {user_payload['tx_hash']}")

    fake.structured_chat = per_tx

    resp = client.post("/answer", json={
        "prompt_version": "answerer_v1",
        "transactions": [
            {"tx_hash": "0xaaa", "prompt": "Prompt A", "subdomains": ["databases"]},
            {"tx_hash": "0xbbb", "prompt": "Prompt B", "subdomains": ["security"]},
        ],
    })
    assert resp.status_code == 200
    results = resp.json()["results"]
    assert len(results) == 2
    assert results[0]["tx_hash"] == "0xaaa"
    assert results[1]["tx_hash"] == "0xbbb"


def test_answer_empty_answer_returns_error(answer_client) -> None:
    client, fake = answer_client
    fake.set_structured_response(_result("0xabc", answer="   "))
    resp = client.post("/answer", json=VALID_REQUEST)
    assert resp.status_code == 400
    assert resp.json()["error"] == "EMPTY_ANSWER"


def test_answer_coverage_mismatch_wrong_tx_hash(answer_client) -> None:
    client, fake = answer_client
    fake.set_structured_response(_result("0xwrong"))
    resp = client.post("/answer", json=VALID_REQUEST)
    assert resp.status_code == 400
    assert resp.json()["error"] == "COVERAGE_MISMATCH"


def test_answer_prompt_version_mismatch(answer_client) -> None:
    client, fake = answer_client
    fake.set_structured_response(_result("0xabc"))
    resp = client.post("/answer", json={**VALID_REQUEST, "prompt_version": "answerer_v99"})
    assert resp.status_code == 400
    assert resp.json()["error"] == "PROMPT_VERSION_MISMATCH"


def test_answer_output_order_matches_input_order(answer_client) -> None:
    client, fake = answer_client

    async def delayed_per_tx(system_prompt, user_payload, response_schema, timeout_seconds, operation=""):
        tx_hash = user_payload["tx_hash"]
        delays = {"0x000": 0.05, "0x001": 0.02, "0x002": 0.0}
        await asyncio.sleep(delays[tx_hash])
        return _result(tx_hash)

    fake.structured_chat = delayed_per_tx

    resp = client.post("/answer", json={
        "prompt_version": "answerer_v1",
        "transactions": [
            {"tx_hash": "0x000", "prompt": "Prompt 0", "subdomains": []},
            {"tx_hash": "0x001", "prompt": "Prompt 1", "subdomains": []},
            {"tx_hash": "0x002", "prompt": "Prompt 2", "subdomains": []},
        ],
    })
    assert resp.status_code == 200
    assert [r["tx_hash"] for r in resp.json()["results"]] == ["0x000", "0x001", "0x002"]


def test_answer_one_failed_call_fails_whole_endpoint(answer_client) -> None:
    client, fake = answer_client
    fake.set_error(AgentServiceError(ErrorCode.PROVIDER_ERROR, "Ollama unreachable", 500))
    resp = client.post("/answer", json=VALID_REQUEST)
    assert resp.status_code == 500
    assert resp.json()["error"] == "PROVIDER_ERROR"
