from fastapi.testclient import TestClient

from config import settings


def test_health_returns_200(client: TestClient) -> None:
    response = client.get("/health")
    assert response.status_code == 200


def test_health_response_has_required_fields(client: TestClient) -> None:
    data = client.get("/health").json()
    assert "status" in data
    assert "provider" in data
    assert "model" in data
    assert "reachable" in data


def test_health_fields_match_config(client: TestClient) -> None:
    data = client.get("/health").json()
    assert data["provider"] == settings.llm_provider
    assert data["model"] == settings.ollama_model
    assert data["status"] == "ok"
    assert isinstance(data["reachable"], bool)  # value depends on Ollama availability


def test_unknown_route_returns_structured_error(client: TestClient) -> None:
    response = client.get("/unknown-route")
    assert response.status_code == 404
    data = response.json()
    assert "error" in data
    assert "detail" in data
    assert data["error"] == "INVALID_REQUEST"


def test_unknown_route_does_not_expose_stack_trace(client: TestClient) -> None:
    data = client.get("/unknown-route").json()
    assert "traceback" not in data
    assert "stack" not in data
