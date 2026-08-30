import json

import pytest
from fastapi.testclient import TestClient

from errors import AgentServiceError, ErrorCode
from experiment.recorder import CallRecorder
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


def test_byzantine_mock_approves_self_and_rejects_others_without_provider(
    judge_client, tmp_path
) -> None:
    client, fake = judge_client
    fake.set_error(AssertionError("provider must not be called"))
    wrong_answer = "configured Byzantine answer"
    cfg = client.app.state.config
    old_provider = cfg.llm_provider
    old_recorder = client.app.state.recorder
    cfg.llm_provider = "mock"
    cfg.mock_preprocessing_answer = wrong_answer
    client.app.state.recorder = CallRecorder(
        validator_id="validator-7", validator_name="mocked-agent",
        provider="mock", model="mocked-agent",
        agent_endpoint="http://127.0.0.1:8106",
        experiment_dir=str(tmp_path),
    )
    try:
        prompt = json.dumps({
            "transactionHash": "abc123",
            "prompt": "Why is a mutex needed?",
            "candidates": [
                {"candidateId": "candidate-1", "answer": "honest answer one"},
                {"candidateId": "candidate-2", "answer": wrong_answer},
                {"candidateId": "candidate-3", "answer": "honest answer two"},
            ],
        })
        resp = client.post("/judge", json={
            "system_prompt": "protocol prompt",
            "user_prompt": prompt,
        })
        assert resp.status_code == 200
        assert json.loads(resp.json()["response"])["classifications"] == [
            {"candidateId": "candidate-1", "category": "WRONG"},
            {"candidateId": "candidate-2", "category": "CORRECT"},
            {"candidateId": "candidate-3", "category": "WRONG"},
        ]
        records = [json.loads(line) for line in (
            tmp_path / "agents" / "mocked-agent.jsonl"
        ).read_text().splitlines()]
        assert len(records) == 3
        assert all(record["mocked"] is True for record in records)
        assert all(record["provider_called"] is False for record in records)
        assert all(record["total_tokens"] == 0 for record in records)
    finally:
        cfg.llm_provider = old_provider
        cfg.mock_preprocessing_answer = ""
        client.app.state.recorder = old_recorder


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
