"""Manifest compatibility and corruption tests."""
from dataclasses import replace

import pytest

from benchmark.fixtures import ALL_FIXTURES, Fixture
from benchmark.manifest import compute_config_hash, create_manifest, load_manifest


def _hash(**overrides):
    values = dict(
        model_name="m", seed=42, temperature=0.0, num_ctx=4096,
        num_predict=256, think=False, prompt_version="v", prompt_hash="p",
        dataset_version="d", dataset_hash="h", trials=1, groups=None,
        keep_alive="30m", timeout_s=120.0, max_retries=1,
        response_schema_hash="schema",
    )
    values.update(overrides)
    return compute_config_hash(**values)


@pytest.mark.parametrize("field,value", [
    ("seed", 43), ("temperature", 0.1), ("num_ctx", 8192),
    ("num_predict", 128), ("think", True), ("keep_alive", "5m"),
    ("timeout_s", 60.0), ("max_retries", 2), ("trials", 2),
    ("dataset_hash", "changed"), ("prompt_hash", "changed"),
    ("response_schema_hash", "changed"),
])
def test_every_compatibility_parameter_changes_hash(field, value):
    assert _hash(**{field: value}) != _hash()


def test_fixture_hash_includes_metric_metadata():
    fixture = ALL_FIXTURES[0]
    assert replace(fixture, is_adversarial=not fixture.is_adversarial).content_hash() != fixture.content_hash()
    assert replace(fixture, assumption_basis="benchmark-assumption").content_hash() != fixture.content_hash()
    assert replace(fixture, scenario_id="changed").content_hash() != fixture.content_hash()


def test_corrupt_manifest_is_fatal(tmp_path):
    (tmp_path / "manifest.json").write_text("{broken")
    with pytest.raises(RuntimeError, match="Corrupt"):
        load_manifest(tmp_path)


def test_manifest_commits_exact_trial_orders():
    manifest = create_manifest(
        model="m", seed=42, temperature=0.0, num_ctx=4096,
        num_predict=256, think=False, keep_alive="30m", timeout_s=120,
        trials=2, groups=None, fixtures=ALL_FIXTURES,
        ollama_version="0.20.0",
        model_info={"digest": "sha256:x", "details": {}},
    )
    assert set(manifest.trial_orders) == {"1", "2"}
    assert all(len(order) == len(ALL_FIXTURES) for order in manifest.trial_orders.values())
    assert manifest.trial_orders["1"] != manifest.trial_orders["2"]
