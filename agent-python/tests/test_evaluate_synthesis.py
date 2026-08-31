import json

import pytest

from experiment.recorder import CallRecorder
from providers.fake_provider import FakeProvider
from schemas import EvaluateSynthesisLLMResult


@pytest.mark.parametrize(
    ("validator_id", "validator_name", "endpoint"),
    [
        ("validator-4", "mocked-agent-v4", "http://127.0.0.1:8103"),
        ("validator-7", "mocked-agent-v7", "http://127.0.0.1:8106"),
        ("validator-8", "mocked-agent-v8", "http://127.0.0.1:8107"),
    ],
)
def test_three_byzantine_mocks_reject_synthesis_without_provider(
    client, tmp_path, validator_id, validator_name, endpoint
):
    fake = FakeProvider()
    fake.set_error(AssertionError("provider must not be called"))
    cfg = client.app.state.config
    old_provider = cfg.llm_provider
    old_recorder = client.app.state.recorder
    cfg.llm_provider = "mock"
    client.app.state.provider = fake
    client.app.state.recorder = CallRecorder(
        validator_id=validator_id, validator_name=validator_name,
        provider="mock", model="mocked-agent",
        agent_endpoint=endpoint, experiment_dir=str(tmp_path),
    )
    try:
        response = client.post("/evaluate-synthesis", json={
            "prompt_version": "synthesis_evaluator_v1",
            "transactions": [{
                "tx_hash": "abc123",
                "prompt": "Why is a mutex needed?",
                "correct_answers": ["A mutex prevents races."],
                "proposed_synthesis": "A mutex prevents races.",
            }],
        })
        assert response.status_code == 200
        assert response.json()["evaluations"] == [{"tx_hash": "abc123", "approved": False}]
        records = [json.loads(line) for line in (
            tmp_path / "agents" / f"{validator_name}.jsonl"
        ).read_text().splitlines()]
        assert len(records) == 1
        assert records[0]["mocked"] is True
        assert records[0]["provider_called"] is False
        assert records[0]["input_tokens"] == 0
        assert records[0]["output_tokens"] == 0
        assert records[0]["total_tokens"] == 0
        assert records[0]["success"] is True
    finally:
        cfg.llm_provider = old_provider
        client.app.state.recorder = old_recorder


def test_byzantine_synthesis_setting_does_not_change_evaluator_prompt(client):
    fake = FakeProvider()
    captured = {}

    async def structured_chat(system_prompt, user_payload, response_schema, timeout_seconds, operation=""):
        captured.update(system_prompt=system_prompt, user_payload=user_payload, operation=operation)
        return EvaluateSynthesisLLMResult(tx_hash=user_payload["tx_hash"], approved=False)

    fake.structured_chat = structured_chat
    cfg = client.app.state.config
    old_provider = cfg.llm_provider
    old_byzantine = cfg.byzantine_mr3_synthesis
    client.app.state.provider = fake
    cfg.llm_provider = "deepseek"
    cfg.byzantine_mr3_synthesis = True
    try:
        response = client.post("/evaluate-synthesis", json={
            "prompt_version": "synthesis_evaluator_v1",
            "transactions": [{
                "tx_hash": "abc123",
                "prompt": "Why is a mutex needed?",
                "correct_answers": ["A mutex prevents races."],
                "proposed_synthesis": "A mutex prevents races but has one subtle error.",
            }],
        })
        assert response.status_code == 200
        assert captured["system_prompt"] == client.app.state.prompts["synthesis_evaluator_v1"].content
        assert "adversarial synthesis proposer" not in captured["system_prompt"]
        assert captured["operation"] == "evaluate_synthesis"
    finally:
        cfg.llm_provider = old_provider
        cfg.byzantine_mr3_synthesis = old_byzantine
