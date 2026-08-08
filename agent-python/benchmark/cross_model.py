"""
Cross-model analysis: pairwise agreement and shared error detection.

Finding 11:
  - "Failed by all" requires complete predictions from every declared model,
    not just models that happened to evaluate that fixture.
  - Missing and invalid predictions are reported separately from wrong predictions.
  - "Failed by majority" (> 50% of declared models wrong) is reported.
  - Model-specific failure analysis is included.

All functions are pure (no I/O). Inputs are lists of prediction dicts
keyed by model name.
"""
from __future__ import annotations

from dataclasses import dataclass, field
from typing import Any


# A fixture key uniquely identifies one evaluation unit (independent of model/trial).
FixtureKey = tuple[str, str, str, str, int]  # (group, tx_id, perspective, candidate_id, trial)


@dataclass
class PairwiseResult:
    model_a: str
    model_b: str
    total_shared: int
    agreement_count: int
    agreement_rate: float
    shared_false_acceptances: int
    shared_false_rejections: int


@dataclass
class SharedErrorAnalysis:
    pairwise: list[PairwiseResult]

    # Fixture keys where ALL declared models produced a wrong prediction
    # (requires complete predictions from the full declared model set)
    failed_by_all: list[dict[str, Any]]

    # Fixture keys where >50% of declared models were wrong (including missing as wrong)
    failed_by_majority: list[dict[str, Any]]

    # Fixture keys where exactly one declared model was wrong
    model_specific_failures: dict[str, list[dict[str, Any]]]

    # Fixture keys where a declared model produced no prediction (timeout/error)
    # These are distinct from wrong predictions
    missing_predictions: dict[str, list[dict[str, Any]]]

    # Coverage summary
    declared_models: list[str]
    coverage_gaps: dict[str, int]   # model -> number of fixture keys with no prediction


def _record_key(r: dict[str, Any]) -> FixtureKey:
    return (
        r.get("group", ""),
        r.get("tx_id", ""),
        r.get("perspective", ""),
        r.get("candidate_id", "candidate-1"),
        r.get("trial", 1),
    )


def compute_pairwise_agreement(
    records_by_model: dict[str, list[dict[str, Any]]],
) -> list[PairwiseResult]:
    """Compute pairwise agreement between every pair of models."""
    models = sorted(records_by_model.keys())
    if len(models) < 2:
        return []

    # Index: model -> fixture_key -> record (valid predictions only for agreement)
    indexed: dict[str, dict[FixtureKey, dict[str, Any]]] = {}
    for model, recs in records_by_model.items():
        indexed[model] = {_record_key(r): r for r in recs if r.get("predicted") is not None}

    results: list[PairwiseResult] = []
    for i, ma in enumerate(models):
        for mb in models[i + 1:]:
            shared_keys = set(indexed[ma]) & set(indexed[mb])
            total = len(shared_keys)
            if total == 0:
                results.append(PairwiseResult(
                    model_a=ma, model_b=mb, total_shared=0,
                    agreement_count=0, agreement_rate=0.0,
                    shared_false_acceptances=0, shared_false_rejections=0,
                ))
                continue

            agree = 0
            shared_fa = 0
            shared_fr = 0
            for key in shared_keys:
                ra, rb = indexed[ma][key], indexed[mb][key]
                if ra["predicted"] == rb["predicted"]:
                    agree += 1
                if (ra.get("is_adversarial") and ra["predicted"] == "CORRECT"
                        and rb.get("is_adversarial") and rb["predicted"] == "CORRECT"):
                    shared_fa += 1
                if (not ra.get("is_adversarial") and ra["predicted"] != "CORRECT"
                        and not rb.get("is_adversarial") and rb["predicted"] != "CORRECT"):
                    shared_fr += 1

            results.append(PairwiseResult(
                model_a=ma, model_b=mb,
                total_shared=total,
                agreement_count=agree,
                agreement_rate=agree / total,
                shared_false_acceptances=shared_fa,
                shared_false_rejections=shared_fr,
            ))

    return results


def find_shared_errors(
    records_by_model: dict[str, list[dict[str, Any]]],
    declared_models: list[str] | None = None,
) -> SharedErrorAnalysis:
    """Identify shared and model-specific errors across the declared model set.

    Parameters
    ----------
    records_by_model:
        Dict mapping model name → list of prediction record dicts.
    declared_models:
        The full list of models that were supposed to run. Used to detect
        missing predictions. If None, defaults to sorted(records_by_model.keys()).

    Notes
    -----
    "Failed by all" requires a wrong semantic prediction from every declared
    model. Missing/invalid outputs are reported separately and never converted
    into semantic errors.
    """
    if declared_models is None:
        declared_models = sorted(records_by_model.keys())
    declared_set = set(declared_models)

    if len(declared_models) < 2:
        return SharedErrorAnalysis(
            pairwise=[],
            failed_by_all=[],
            failed_by_majority=[],
            model_specific_failures={m: [] for m in declared_models},
            missing_predictions={m: [] for m in declared_models},
            declared_models=declared_models,
            coverage_gaps={m: 0 for m in declared_models},
        )

    pairwise = compute_pairwise_agreement(records_by_model)

    # Index all records by fixture key for each declared model
    # Separate: valid predictions vs no prediction
    valid_index: dict[str, dict[FixtureKey, dict[str, Any]]] = {m: {} for m in declared_models}
    all_index: dict[str, dict[FixtureKey, dict[str, Any]]] = {m: {} for m in declared_models}

    for model in declared_models:
        for r in records_by_model.get(model, []):
            key = _record_key(r)
            all_index[model][key] = r
            if r.get("predicted") is not None:
                valid_index[model][key] = r

    # Union of all fixture keys across all declared models
    all_keys: set[FixtureKey] = set()
    for m in declared_models:
        all_keys.update(all_index[m].keys())

    failed_by_all: list[dict[str, Any]] = []
    failed_by_majority: list[dict[str, Any]] = []
    model_specific: dict[str, list[dict[str, Any]]] = {m: [] for m in declared_models}
    missing: dict[str, list[dict[str, Any]]] = {m: [] for m in declared_models}

    for key in all_keys:
        wrong_models: list[str] = []
        missing_models: list[str] = []
        predictions: dict[str, str | None] = {}

        for m in declared_models:
            r = all_index[m].get(key)
            if r is None:
                # Model was supposed to run this but has no record at all
                missing_models.append(m)
                predictions[m] = "MISSING"
            elif r.get("predicted") is None:
                # Model ran but produced no valid output
                missing_models.append(m)
                predictions[m] = "NO_OUTPUT"
            elif r.get("predicted") != r.get("expected"):
                wrong_models.append(m)
                predictions[m] = r["predicted"]
            else:
                predictions[m] = r["predicted"]

        # Representative record for metadata
        rep = None
        for m in declared_models:
            rep = all_index[m].get(key)
            if rep is not None:
                break
        if rep is None:
            continue

        total_declared = len(declared_models)

        entry = {
            "group": rep.get("group"),
            "tx_id": rep.get("tx_id"),
            "perspective": rep.get("perspective"),
            "expected": rep.get("expected"),
            "predictions": predictions,
            "wrong_models": wrong_models,
            "missing_models": missing_models,
        }

        # Failed by ALL requires complete declared-model coverage and all wrong.
        if not missing_models and len(wrong_models) == total_declared:
            failed_by_all.append(entry)

        # Majority denominator is the declared model set; missing stays separate.
        elif len(wrong_models) > total_declared / 2:
            failed_by_majority.append(entry)

        # Model-specific wrong prediction (exactly one model wrong, rest correct)
        elif len(wrong_models) == 1 and len(missing_models) == 0:
            m = wrong_models[0]
            r = all_index[m].get(key)
            if r:
                model_specific[m].append({
                    "group": r.get("group"),
                    "tx_id": r.get("tx_id"),
                    "perspective": r.get("perspective"),
                    "expected": r.get("expected"),
                    "predicted": r.get("predicted"),
                })

    # Missing predictions per model
    for m in declared_models:
        for key in all_keys:
            r = all_index[m].get(key)
            if r is None or r.get("predicted") is None:
                # Reconstruct a summary from any available record for this key
                ref = None
                for other_m in declared_models:
                    ref = all_index[other_m].get(key)
                    if ref is not None:
                        break
                if ref:
                    missing[m].append({
                        "group": ref.get("group"),
                        "tx_id": ref.get("tx_id"),
                        "perspective": ref.get("perspective"),
                        "expected": ref.get("expected"),
                        "outcome": "NO_OUTPUT" if r is not None else "MISSING_RECORD",
                    })

    coverage_gaps = {m: len(missing[m]) for m in declared_models}

    return SharedErrorAnalysis(
        pairwise=pairwise,
        failed_by_all=failed_by_all,
        failed_by_majority=failed_by_majority,
        model_specific_failures=model_specific,
        missing_predictions=missing,
        declared_models=declared_models,
        coverage_gaps=coverage_gaps,
    )
