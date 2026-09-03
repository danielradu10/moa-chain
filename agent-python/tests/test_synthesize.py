import json

import pytest
from fastapi.testclient import TestClient

from experiment.recorder import CallRecorder
from providers.fake_provider import FakeProvider
from schemas import SynthesizeLLMResult


@pytest.fixture
def synthesize_client(client: TestClient):
    fake = FakeProvider()
    client.app.state.provider = fake
    return client, fake


REQUEST = {
    "transactions": [{
        "tx_hash": "abc123",
        "prompt": "Why is a mutex needed?",
        "correct_answers": ["A mutex prevents races.", "It protects invariants."],
    }],
}


@pytest.mark.parametrize(
    ("byzantine", "prompt_version", "expected_fragment", "attack_status"),
    [
        (False, "synthesizer_v1", "synthesis expert", "not_applicable"),
        (True, "byzantine_synthesizer_v1", "exactly ONE materially incorrect", "requires_review"),
    ],
)
def test_synthesis_prompt_selection_and_real_provider_path(
    synthesize_client, tmp_path, byzantine, prompt_version, expected_fragment, attack_status
):
    client, fake = synthesize_client
    cfg = client.app.state.config
    old_provider = cfg.llm_provider
    old_byzantine = cfg.byzantine_mr3_synthesis
    old_recorder = client.app.state.recorder
    captured = {}

    async def structured_chat(system_prompt, user_payload, response_schema, timeout_seconds, operation=""):
        captured.update(system_prompt=system_prompt, user_payload=user_payload, operation=operation)
        return SynthesizeLLMResult(
            tx_hash=user_payload["tx_hash"],
            synthesized_answer="Mostly correct synthesis with the generated model claim.",
        )

    fake.structured_chat = structured_chat
    cfg.llm_provider = "deepseek"
    cfg.byzantine_mr3_synthesis = byzantine
    client.app.state.recorder = CallRecorder(
        validator_id="validator-10", validator_name="deepseek-v4-pro",
        provider="deepseek", model="deepseek-v4-pro",
        agent_endpoint="http://127.0.0.1:8109", experiment_dir=str(tmp_path),
    )
    try:
        response = client.post("/synthesize", json={
            "prompt_version": prompt_version,
            **REQUEST,
        })
        assert response.status_code == 200
        assert expected_fragment in captured["system_prompt"]
        assert captured["operation"] == "synthesize"
        assert captured["user_payload"]["correct_answers"] == REQUEST["transactions"][0]["correct_answers"]

        record = json.loads((
            tmp_path / "agents" / "deepseek-v4-pro.jsonl"
        ).read_text().strip())
        assert record["mocked"] is False
        assert record["provider_called"] is True
        assert record["request_payload"]["synthesis_prompt_version"] == prompt_version
        assert record["request_payload"]["synthesis_system_prompt"] == captured["system_prompt"]
        assert record["request_payload"]["attack_generation_status"] == attack_status
        assert record["request_payload"]["correct_answers"] == REQUEST["transactions"][0]["correct_answers"]
        assert record["parsed_response"]["synthesized_answer"] == "Mostly correct synthesis with the generated model claim."
    finally:
        cfg.llm_provider = old_provider
        cfg.byzantine_mr3_synthesis = old_byzantine
        client.app.state.recorder = old_recorder


def test_mock_synthesis_copies_first_correct_answer_without_provider_call(client, tmp_path):
    cfg = client.app.state.config
    old_provider_name = cfg.llm_provider
    old_provider = client.app.state.provider
    old_recorder = client.app.state.recorder

    class ProviderMustNotBeCalled:
        async def structured_chat(self, **_kwargs):
            raise AssertionError("mock synthesis called an external provider")

    cfg.llm_provider = "mock"
    client.app.state.provider = ProviderMustNotBeCalled()
    client.app.state.recorder = CallRecorder(
        validator_id="validator-8", validator_name="gemini-3.6-flash-2",
        provider="mock", model="mocked-agent",
        agent_endpoint="http://127.0.0.1:8107", experiment_dir=str(tmp_path),
    )
    try:
        response = client.post("/synthesize", json={
            "prompt_version": "synthesizer_v1",
            **REQUEST,
        })
        assert response.status_code == 200
        assert response.json()["synthesized_answers"][0]["answer"] == "A mutex prevents races."

        record = json.loads((
            tmp_path / "agents" / "gemini-3.6-flash-2.jsonl"
        ).read_text().strip())
        assert record["mocked"] is True
        assert record["provider_called"] is False
        assert record["total_tokens"] == 0
    finally:
        cfg.llm_provider = old_provider_name
        client.app.state.provider = old_provider
        client.app.state.recorder = old_recorder
