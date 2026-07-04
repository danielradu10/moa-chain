#!/usr/bin/env python3

import json
import random
import unittest
from collections import Counter
from pathlib import Path

import numpy
import torch

if __package__:
    from .cluster import cluster_transactions
else:
    from cluster import cluster_transactions


TEST_DATA_PATH = (
    Path(__file__).parent.parent / "testdata" / "obvious_dominant_cluster.json"
)
TOPICAL_WRONG_ANSWERS_PATH = (
    Path(__file__).parent.parent / "testdata" / "topical_but_wrong_answers.json"
)
TEST_SEED = 0


class ObviousDominantClusterTest(unittest.TestCase):
    def test_eight_related_answers_form_one_cluster_and_two_wrong_answers_do_not(self):
        # Scenario: ten different agents answer the same transaction and prompt.
        # Agents 01-08 give semantically compatible answers about digital-signature
        # verification. Agents 09 and 10 answer unrelated questions about
        # photosynthesis and cooking. The expected result is one dominant cluster
        # of eight answers and two separate one-answer clusters.
        transactions = json.loads(TEST_DATA_PATH.read_text(encoding="utf-8"))

        # The production embedding component is part of this test. CPU inference
        # and fixed random seeds keep the embedding and clustering path repeatable.
        random.seed(TEST_SEED)
        numpy.random.seed(TEST_SEED)
        torch.manual_seed(TEST_SEED)
        torch.use_deterministic_algorithms(True)

        result = cluster_transactions(transactions)

        cluster_sizes = Counter(item["clusterId"] for item in result)
        self.assertEqual(sorted(cluster_sizes.values(), reverse=True), [8, 1, 1])

        dominant_cluster_id = result[0]["clusterId"]
        self.assertTrue(
            all(item["clusterId"] == dominant_cluster_id for item in result[:8])
        )
        self.assertNotEqual(result[8]["clusterId"], dominant_cluster_id)
        self.assertNotEqual(result[9]["clusterId"], dominant_cluster_id)
        self.assertNotEqual(result[8]["clusterId"], result[9]["clusterId"])

    def test_topical_but_wrong_answers_can_join_a_semantic_cluster(self):
        # Scenario: agents 01-08 provide correct answers about signature
        # verification. Agent 09 discusses the same topic but incorrectly claims
        # that a signature encrypts a message. Agent 10 also stays on topic but
        # incorrectly claims that a signature proves factual correctness and
        # prevents blockchain forks.
        #
        # With the real embedding model, single linkage, and the default 0.3
        # cutoff, agent 09 is semantically close enough to join the eight correct
        # answers while agent 10 remains separate. The resulting 9-1 split records
        # an important limitation: semantic clustering does not verify truth.
        transactions = json.loads(
            TOPICAL_WRONG_ANSWERS_PATH.read_text(encoding="utf-8")
        )

        random.seed(TEST_SEED)
        numpy.random.seed(TEST_SEED)
        torch.manual_seed(TEST_SEED)
        torch.use_deterministic_algorithms(True)

        result = cluster_transactions(transactions)

        cluster_sizes = Counter(item["clusterId"] for item in result)
        self.assertEqual(sorted(cluster_sizes.values(), reverse=True), [9, 1])

        dominant_cluster_id = result[0]["clusterId"]
        self.assertTrue(
            all(item["clusterId"] == dominant_cluster_id for item in result[:9])
        )
        self.assertNotEqual(result[9]["clusterId"], dominant_cluster_id)


if __name__ == "__main__":
    unittest.main()
