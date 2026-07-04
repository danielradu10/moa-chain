#!/usr/bin/env python3

import argparse
import json
import statistics
from collections import defaultdict
from pathlib import Path

from scipy.cluster.hierarchy import linkage
from scipy.spatial.distance import pdist, squareform

if __package__:
    from .cluster import DEFAULT_DISTANCE_THRESHOLD, cluster_embeddings
    from .embed import embed_answers
else:
    from cluster import DEFAULT_DISTANCE_THRESHOLD, cluster_embeddings
    from embed import embed_answers


def summarize(values: list[float]) -> dict[str, float]:
    return {
        "minimum": min(values),
        "maximum": max(values),
        "mean": statistics.mean(values),
        "median": statistics.median(values),
    }


def first_merge_with_correct_answer(
    linkage_matrix,
    target_index: int,
    correct_count: int,
    answer_count: int,
) -> dict:
    members = {index: {index} for index in range(answer_count)}
    for row_index, row in enumerate(linkage_matrix):
        left, right = int(row[0]), int(row[1])
        merged = members[left] | members[right]
        members[answer_count + row_index] = merged
        if target_index in merged and any(index < correct_count for index in merged):
            return {
                "distance": float(row[2]),
                "memberIndices": sorted(merged),
            }
    raise RuntimeError("target answer never merged with a correct answer")


def analyze_scenario(
    transactions: list[dict],
    correct_count: int = 8,
    distance_threshold: float = DEFAULT_DISTANCE_THRESHOLD,
    linkage_methods: tuple[str, ...] = ("single", "complete"),
) -> dict:
    if not 0 < correct_count < len(transactions):
        raise ValueError("correct count must split correct and incorrect answers")

    agent_ids = [transaction["agentId"] for transaction in transactions]
    embeddings = embed_answers(
        [transaction["answer"] for transaction in transactions]
    )
    distances = squareform(pdist(embeddings, metric="cosine"))

    correct_pairs = [
        {
            "left": agent_ids[left],
            "right": agent_ids[right],
            "distance": float(distances[left, right]),
        }
        for left in range(correct_count)
        for right in range(left + 1, correct_count)
    ]
    incorrect_to_correct = {}
    for incorrect_index in range(correct_count, len(transactions)):
        pairs = [
            {
                "correctAgent": agent_ids[correct_index],
                "distance": float(distances[incorrect_index, correct_index]),
            }
            for correct_index in range(correct_count)
        ]
        incorrect_to_correct[agent_ids[incorrect_index]] = {
            "pairs": pairs,
            "summary": summarize([pair["distance"] for pair in pairs]),
        }

    clusters = {}
    condensed_distances = pdist(embeddings, metric="cosine")
    for method in linkage_methods:
        labels = cluster_embeddings(
            embeddings,
            linkage_method=method,
            distance_threshold=distance_threshold,
        )
        members_by_label = defaultdict(list)
        for agent_id, label in zip(agent_ids, labels, strict=True):
            members_by_label[label].append(agent_id)

        linkage_matrix = linkage(condensed_distances, method=method)
        clusters[method] = {
            "members": sorted(
                members_by_label.values(),
                key=lambda members: (-len(members), members),
            ),
            "incorrectFirstCorrectMerges": {
                agent_ids[index]: first_merge_with_correct_answer(
                    linkage_matrix,
                    target_index=index,
                    correct_count=correct_count,
                    answer_count=len(transactions),
                )
                for index in range(correct_count, len(transactions))
            },
        }

    return {
        "txHash": transactions[0]["txHash"],
        "distanceThreshold": distance_threshold,
        "correctPairDistances": {
            "pairs": correct_pairs,
            "summary": summarize([pair["distance"] for pair in correct_pairs]),
        },
        "incorrectToCorrectDistances": incorrect_to_correct,
        "clusters": clusters,
    }


def main() -> None:
    parser = argparse.ArgumentParser(
        description="Measure answer distances and compare hierarchical linkages."
    )
    parser.add_argument("inputs", nargs="+", type=Path)
    parser.add_argument("--correct-count", type=int, default=8)
    parser.add_argument(
        "--distance-threshold",
        type=float,
        default=DEFAULT_DISTANCE_THRESHOLD,
    )
    args = parser.parse_args()

    analyses = []
    for input_path in args.inputs:
        with input_path.open(encoding="utf-8") as input_file:
            transactions = json.load(input_file)
        analyses.append(
            {
                "scenario": input_path.name,
                **analyze_scenario(
                    transactions,
                    correct_count=args.correct_count,
                    distance_threshold=args.distance_threshold,
                ),
            }
        )

    print(json.dumps(analyses, indent=2))


if __name__ == "__main__":
    main()
