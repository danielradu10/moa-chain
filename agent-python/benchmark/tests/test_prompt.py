"""Tests for benchmark/prompt.py — strict response validation (Finding 9)"""
import pytest

from benchmark.prompt import (
    ANSWER_JUDGE_PROMPT_HASH,
    ANSWER_JUDGE_PROMPT_VERSION,
    RESPONSE_JSON_SCHEMA,
    build_user_prompt,
    parse_judge_response,
)

VALID_RESPONSE = '{"classifications":[{"candidateId":"candidate-1","category":"CORRECT"}]}'


class TestBuildUserPrompt:
    def test_contains_hex_encoded_tx_id(self):
        tx_id = "hello"
        result = build_user_prompt(tx_id, "prompt?", "answer.", "candidate-1")
        import json
        data = json.loads(result)
        assert data["transactionHash"] == tx_id.encode().hex()

    def test_single_candidate_array(self):
        import json
        result = build_user_prompt("tx", "q", "a", "candidate-1")
        data = json.loads(result)
        assert len(data["candidates"]) == 1
        assert data["candidates"][0]["candidateId"] == "candidate-1"
        assert data["candidates"][0]["answer"] == "a"


class TestParseJudgeResponseSuccess:
    def test_correct_category(self):
        assert parse_judge_response(VALID_RESPONSE) == "CORRECT"

    def test_all_valid_categories(self):
        for cat in ["CORRECT", "WRONG", "HALLUCINATION", "MALICIOUS"]:
            resp = f'{{"classifications":[{{"candidateId":"candidate-1","category":"{cat}"}}]}}'
            assert parse_judge_response(resp) == cat

    def test_whitespace_around_json_is_allowed(self):
        resp = "  \n" + VALID_RESPONSE + "\n  "
        assert parse_judge_response(resp) == "CORRECT"


class TestParseJudgeResponseRejectsTrailingContent:
    """Finding 9: reject trailing JSON values."""

    def test_rejects_trailing_text(self):
        resp = VALID_RESPONSE + " extra"
        with pytest.raises(ValueError, match="trailing content"):
            parse_judge_response(resp)

    def test_rejects_trailing_newline_with_content(self):
        resp = VALID_RESPONSE + "\n{}"
        with pytest.raises(ValueError, match="trailing content"):
            parse_judge_response(resp)

    def test_rejects_trailing_json_object(self):
        resp = VALID_RESPONSE + '{"extra": true}'
        with pytest.raises(ValueError, match="trailing content"):
            parse_judge_response(resp)

    def test_whitespace_only_trailing_is_ok(self):
        resp = VALID_RESPONSE + "   \n"
        assert parse_judge_response(resp) == "CORRECT"


class TestParseJudgeResponseRejectsUnknownFields:
    """Finding 9: reject unknown fields."""

    def test_rejects_unknown_outer_field(self):
        resp = '{"classifications":[{"candidateId":"candidate-1","category":"CORRECT"}],"extra":1}'
        with pytest.raises(ValueError, match="unknown fields in response"):
            parse_judge_response(resp)

    def test_rejects_unknown_classification_field(self):
        resp = '{"classifications":[{"candidateId":"candidate-1","category":"CORRECT","note":"ok"}]}'
        with pytest.raises(ValueError, match="unknown fields in classification"):
            parse_judge_response(resp)

    def test_rejects_extra_nested_key(self):
        resp = '{"classifications":[{"candidateId":"candidate-1","category":"WRONG","confidence":0.9}]}'
        with pytest.raises(ValueError, match="unknown fields in classification"):
            parse_judge_response(resp)


class TestParseJudgeResponseRejectsMalformedStructure:
    def test_rejects_empty_string(self):
        with pytest.raises(ValueError, match="empty"):
            parse_judge_response("")

    def test_rejects_not_object(self):
        with pytest.raises(ValueError, match="must be a JSON object"):
            parse_judge_response('["candidate-1", "CORRECT"]')

    def test_rejects_missing_classifications_key(self):
        with pytest.raises(ValueError, match="unknown fields|classifications"):
            parse_judge_response('{"result": "CORRECT"}')

    def test_rejects_classifications_not_array(self):
        with pytest.raises(ValueError, match='"classifications" must be an array'):
            parse_judge_response('{"classifications": {"candidateId":"candidate-1","category":"CORRECT"}}')

    def test_rejects_empty_classifications_array(self):
        with pytest.raises(ValueError, match="empty"):
            parse_judge_response('{"classifications":[]}')

    def test_rejects_multiple_classifications(self):
        resp = (
            '{"classifications":['
            '{"candidateId":"candidate-1","category":"CORRECT"},'
            '{"candidateId":"candidate-2","category":"WRONG"}'
            ']}'
        )
        with pytest.raises(ValueError, match="exactly 1"):
            parse_judge_response(resp)

    def test_rejects_wrong_candidate_id(self):
        resp = '{"classifications":[{"candidateId":"candidate-2","category":"CORRECT"}]}'
        with pytest.raises(ValueError, match="candidateId mismatch"):
            parse_judge_response(resp)

    def test_rejects_unknown_category(self):
        resp = '{"classifications":[{"candidateId":"candidate-1","category":"MAYBE"}]}'
        with pytest.raises(ValueError, match="unknown category"):
            parse_judge_response(resp)

    def test_rejects_non_string_category(self):
        resp = '{"classifications":[{"candidateId":"candidate-1","category":1}]}'
        with pytest.raises(ValueError, match='"category" must be a string'):
            parse_judge_response(resp)

    def test_rejects_missing_category_field(self):
        resp = '{"classifications":[{"candidateId":"candidate-1"}]}'
        with pytest.raises(ValueError, match='"category"'):
            parse_judge_response(resp)

    def test_rejects_invalid_json(self):
        with pytest.raises(ValueError, match="not valid JSON"):
            parse_judge_response("{not json}")

    def test_rejects_markdown_wrapped_json(self):
        resp = "```json\n" + VALID_RESPONSE + "\n```"
        with pytest.raises(ValueError, match="not valid JSON|trailing content"):
            parse_judge_response(resp)


class TestPromptMetadata:
    def test_version_string(self):
        assert ANSWER_JUDGE_PROMPT_VERSION == "answer-judge-v4"

    def test_prompt_hash_is_sha256(self):
        assert len(ANSWER_JUDGE_PROMPT_HASH) == 64
        assert all(c in "0123456789abcdef" for c in ANSWER_JUDGE_PROMPT_HASH)

    def test_response_schema_is_dict(self):
        assert isinstance(RESPONSE_JSON_SCHEMA, dict)
        assert RESPONSE_JSON_SCHEMA["type"] == "object"
        assert "classifications" in RESPONSE_JSON_SCHEMA["properties"]
        assert RESPONSE_JSON_SCHEMA.get("additionalProperties") is False
