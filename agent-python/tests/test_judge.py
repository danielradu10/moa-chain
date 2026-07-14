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


def _valid_response(*candidate_ids: str) -> str:
    categories = ["CORRECT", "HALLUCINATION", "MALICIOUS", "WRONG"]
    return json.dumps({
        "classifications": [
            {"candidateId": cid, "category": categories[i % len(categories)]}
            for i, cid in enumerate(candidate_ids)
        ]
    })


VALID_REQUEST = {
    "system_prompt": "You are a judge.",
    "user_prompt": "Here are the candidate answers.",
}


def test_judge_valid_response_passes_through(judge_client) -> None:
    client, fake = judge_client
    raw = _valid_response("candidate-1", "candidate-2")
    fake.set_raw_response(raw)
    resp = client.post("/judge", json=VALID_REQUEST)
    assert resp.status_code == 200
    assert resp.json()["response"] == raw


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
    fake.set_raw_response(_valid_response("candidate-1"))
    resp = client.post("/judge", json={**VALID_REQUEST, "system_prompt": "   "})
    assert resp.status_code == 400
    assert resp.json()["error"] == "INVALID_REQUEST"


def test_judge_empty_user_prompt(judge_client) -> None:
    client, fake = judge_client
    fake.set_raw_response(_valid_response("candidate-1"))
    resp = client.post("/judge", json={**VALID_REQUEST, "user_prompt": ""})
    assert resp.status_code == 400
    assert resp.json()["error"] == "INVALID_REQUEST"


def test_judge_all_valid_categories_accepted(judge_client) -> None:
    client, fake = judge_client
    raw = _valid_response("candidate-1", "candidate-2", "candidate-3", "candidate-4")
    fake.set_raw_response(raw)
    resp = client.post("/judge", json=VALID_REQUEST)
    assert resp.status_code == 200


def test_judge_provider_error_propagates(judge_client) -> None:
    client, fake = judge_client
    fake.set_error(AgentServiceError(ErrorCode.PROVIDER_ERROR, "Ollama unreachable", 500))
    resp = client.post("/judge", json=VALID_REQUEST)
    assert resp.status_code == 500
    assert resp.json()["error"] == "PROVIDER_ERROR"
