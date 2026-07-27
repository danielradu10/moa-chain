import json

import pytest
from fastapi.testclient import TestClient

from errors import AgentServiceError, ErrorCode
from providers.fake_provider import FakeProvider


@pytest.fixture
def judge_client(client: TestClient):
    fake = FakeProvider()
    client.app.state.provider = fake
    return client, fake


def _single_response(candidate_id: str, category: str = "CORRECT") -> str:
    return json.dumps({"classifications": [{"candidateId": candidate_id, "category": category}]})


def _user_prompt(*candidate_ids: str) -> str:
    return json.dumps({
        "transactionHash": "abc123",
        "prompt": "What is the main benefit of unit tests?",
        "candidates": [
            {"candidateId": cid, "answer": f"Answer from {cid}."}
            for cid in candidate_ids
        ],
    })


VALID_REQUEST = {
    "system_prompt": "You are a judge.",
    "user_prompt": _user_prompt("candidate-1"),
}


def test_judge_single_candidate_returns_classification(judge_client) -> None:
    client, fake = judge_client
    fake.set_raw_response(_single_response("candidate-1", "CORRECT"))
    resp = client.post("/judge", json=VALID_REQUEST)
    assert resp.status_code == 200
    data = json.loads(resp.json()["response"])
    assert data["classifications"] == [{"candidateId": "candidate-1", "category": "CORRECT"}]


def test_judge_two_candidates_merges_per_candidate_responses(judge_client) -> None:
    client, fake = judge_client
    # FakeProvider returns the same raw response for every call.
    # With 2 candidates the endpoint makes 2 LLM calls and merges both results.
    fake.set_raw_response(_single_response("candidate-1", "CORRECT"))
    resp = client.post("/judge", json={
        "system_prompt": "You are a judge.",
        "user_prompt": _user_prompt("candidate-1", "candidate-2"),
    })
    assert resp.status_code == 200
    data = json.loads(resp.json()["response"])
    assert len(data["classifications"]) == 2


def test_judge_invalid_json_from_model(judge_client) -> None:
    client, fake = judge_client
    fake.set_raw_response("not valid json")
    resp = client.post("/judge", json=VALID_REQUEST)
    assert resp.status_code == 400
    assert resp.json()["error"] == "INVALID_MODEL_OUTPUT"


def test_judge_array_instead_of_object(judge_client) -> None:
    client, fake = judge_client
    fake.set_raw_response(json.dumps([{"candidateId": "candidate-1", "category": "CORRECT"}]))
    resp = client.post("/judge", json=VALID_REQUEST)
    assert resp.status_code == 400
    assert resp.json()["error"] == "INVALID_MODEL_OUTPUT"


def test_judge_missing_classifications_key(judge_client) -> None:
    client, fake = judge_client
    fake.set_raw_response(json.dumps({"results": []}))
    resp = client.post("/judge", json=VALID_REQUEST)
    assert resp.status_code == 400
    assert resp.json()["error"] == "INVALID_MODEL_OUTPUT"


def test_judge_unknown_category(judge_client) -> None:
    client, fake = judge_client
    fake.set_raw_response(json.dumps({"classifications": [{"candidateId": "candidate-1", "category": "Uncertain"}]}))
    resp = client.post("/judge", json=VALID_REQUEST)
    assert resp.status_code == 400
    assert resp.json()["error"] == "UNKNOWN_CATEGORY"


def test_judge_empty_system_prompt(judge_client) -> None:
    client, fake = judge_client
    fake.set_raw_response(_single_response("candidate-1"))
    resp = client.post("/judge", json={**VALID_REQUEST, "system_prompt": "   "})
    assert resp.status_code == 400
    assert resp.json()["error"] == "INVALID_REQUEST"


def test_judge_empty_user_prompt(judge_client) -> None:
    client, fake = judge_client
    fake.set_raw_response(_single_response("candidate-1"))
    resp = client.post("/judge", json={**VALID_REQUEST, "user_prompt": ""})
    assert resp.status_code == 400
    assert resp.json()["error"] == "INVALID_REQUEST"


def test_judge_user_prompt_not_json(judge_client) -> None:
    client, fake = judge_client
    fake.set_raw_response(_single_response("candidate-1"))
    resp = client.post("/judge", json={**VALID_REQUEST, "user_prompt": "not json"})
    assert resp.status_code == 400
    assert resp.json()["error"] == "INVALID_REQUEST"


def test_judge_user_prompt_missing_candidates(judge_client) -> None:
    client, fake = judge_client
    fake.set_raw_response(_single_response("candidate-1"))
    resp = client.post("/judge", json={
        **VALID_REQUEST,
        "user_prompt": json.dumps({"transactionHash": "abc", "prompt": "q"}),
    })
    assert resp.status_code == 400
    assert resp.json()["error"] == "INVALID_REQUEST"


def test_judge_user_prompt_empty_candidates(judge_client) -> None:
    client, fake = judge_client
    fake.set_raw_response(_single_response("candidate-1"))
    resp = client.post("/judge", json={
        **VALID_REQUEST,
        "user_prompt": json.dumps({"transactionHash": "abc", "prompt": "q", "candidates": []}),
    })
    assert resp.status_code == 400
    assert resp.json()["error"] == "INVALID_REQUEST"


def test_judge_all_valid_categories_accepted(judge_client) -> None:
    for category in ("CORRECT", "HALLUCINATION", "MALICIOUS", "WRONG"):
        client, fake = judge_client
        fake.set_raw_response(_single_response("candidate-1", category))
        resp = client.post("/judge", json=VALID_REQUEST)
        assert resp.status_code == 200


def test_judge_provider_error_propagates(judge_client) -> None:
    client, fake = judge_client
    fake.set_error(AgentServiceError(ErrorCode.PROVIDER_ERROR, "Ollama unreachable", 500))
    resp = client.post("/judge", json=VALID_REQUEST)
    assert resp.status_code == 500
    assert resp.json()["error"] == "PROVIDER_ERROR"
