#!/usr/bin/env python3

import argparse
import json
from pathlib import Path

from scipy.cluster.hierarchy import fcluster, linkage
from scipy.spatial.distance import pdist

if __package__:
    from .embed import embed_answers
else:
    from embed import embed_answers


DEFAULT_LINKAGE = "single"
DEFAULT_DISTANCE_THRESHOLD = 0.3
LINKAGE_METHODS = ("single", "complete", "average", "weighted")


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser(
        description="Cluster answer embeddings independently for each transaction."
    )

    parser.add_argument("input", type=Path, help="input answers JSON file")
    parser.add_argument("output", type=Path, help="output clusters JSON file")
    parser.add_argument(
        "--linkage",
        choices=LINKAGE_METHODS,
        default=DEFAULT_LINKAGE,
        help=f"hierarchical linkage method (default: {DEFAULT_LINKAGE})",
    )

    parser.add_argument(
        "--distance-threshold",
        type=float,
        default=DEFAULT_DISTANCE_THRESHOLD,
        help=f"cosine-distance linkage cutoff (default: {DEFAULT_DISTANCE_THRESHOLD})",
    )

    return parser.parse_args()


def cluster_embeddings(
    embeddings: list[list[float]],
    linkage_method: str = DEFAULT_LINKAGE,
    distance_threshold: float = DEFAULT_DISTANCE_THRESHOLD,
) -> list[int]:
    if linkage_method not in LINKAGE_METHODS:
        raise ValueError(
            f"unsupported linkage method {linkage_method!r}; "
            f"expected one of {', '.join(LINKAGE_METHODS)}"
        )

    if distance_threshold < 0:
        raise ValueError("distance threshold must be non-negative")
    if len(embeddings) < 2:
        return [0] * len(embeddings)

    distances = pdist(embeddings, metric="cosine")
    linkage_matrix = linkage(distances, method=linkage_method)
    raw_labels = fcluster(
        linkage_matrix,
        t=distance_threshold,
        criterion="distance",
    )

    members_by_label: dict[int, list[int]] = {}
    for index, label in enumerate(raw_labels):
        members_by_label.setdefault(int(label), []).append(index)

    canonical_labels = {
        label: cluster_id
        for cluster_id, (label, _) in enumerate(
            sorted(members_by_label.items(), key=lambda item: item[1])
        )
    }

    return [canonical_labels[int(label)] for label in raw_labels]


def load_answers(input_path: Path) -> list[dict]:
    with input_path.open(encoding="utf-8") as input_file:
        data = json.load(input_file)

    if not isinstance(data, list):
        raise ValueError("input must be a JSON array of transactions")

    for index, transaction in enumerate(data):
        if not isinstance(transaction, dict):
            raise ValueError(f"transaction at index {index} must be an object")

        tx_hash = transaction.get("txHash")
        answer = transaction.get("answer")
        if not isinstance(tx_hash, str) or not tx_hash:
            raise ValueError(f"txHash at index {index} must be a non-empty string")
        if not isinstance(answer, str) or not answer:
            raise ValueError(f"answer at index {index} must be a non-empty string")

    return data


def cluster_transactions(
    data: list[dict],
    linkage_method: str = DEFAULT_LINKAGE,
    distance_threshold: float = DEFAULT_DISTANCE_THRESHOLD,
) -> list[dict]:
    embeddings = embed_answers([transaction["answer"] for transaction in data])
    cluster_ids = [-1] * len(data)

    indices_by_tx_hash: dict[str, list[int]] = {}
    for index, item in enumerate(data):
        indices_by_tx_hash.setdefault(item["txHash"], []).append(index)

    for indices in indices_by_tx_hash.values():
        canonical_indices = sorted(
            indices,
            key=lambda index: tuple(embeddings[index]),
        )
        transaction_embeddings = [embeddings[index] for index in canonical_indices]
        transaction_cluster_ids = cluster_embeddings(
            transaction_embeddings,
            linkage_method=linkage_method,
            distance_threshold=distance_threshold,
        )
        for index, cluster_id in zip(
            canonical_indices, transaction_cluster_ids, strict=True
        ):
            cluster_ids[index] = cluster_id

    return [
        {**item, "clusterId": cluster_id}
        for item, cluster_id in zip(data, cluster_ids, strict=True)
    ]


def main() -> None:
    args = parse_args()
    data = load_answers(args.input)
    result = cluster_transactions(
        data,
        linkage_method=args.linkage,
        distance_threshold=args.distance_threshold,
    )

    with args.output.open("w", encoding="utf-8") as output_file:
        json.dump(result, output_file, indent=2)
        output_file.write("\n")


if __name__ == "__main__":
    main()
