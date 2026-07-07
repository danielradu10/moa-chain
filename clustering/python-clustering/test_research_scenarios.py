#!/usr/bin/env python3

import json
import random
import unittest
from pathlib import Path

import numpy
import torch

if __package__:
    from .cluster import cluster_transactions
else:
    from cluster import cluster_transactions


TEST_DATA_DIRECTORY = Path(__file__).parent.parent / "testdata"
TEST_SEED = 0


def cluster_fixture(fixture_name: str) -> dict[str, int]:
    transactions = json.loads(
        (TEST_DATA_DIRECTORY / fixture_name).read_text(encoding="utf-8")
    )

    random.seed(TEST_SEED)
    numpy.random.seed(TEST_SEED)
    torch.manual_seed(TEST_SEED)
    torch.use_deterministic_algorithms(True)

    result = cluster_transactions(transactions)
    return {item["agentId"]: item["clusterId"] for item in result}


class ResearchClusteringScenariosTest(unittest.TestCase):
    def assert_same_cluster(self, labels: dict[str, int], agents: list[str]) -> None:
        self.assertEqual({labels[agent] for agent in agents}, {labels[agents[0]]})

    def assert_different_clusters(
        self,
        labels: dict[str, int],
        left_agent: str,
        right_agent: str,
    ) -> None:
        self.assertNotEqual(labels[left_agent], labels[right_agent])

    @unittest.expectedFailure
    def test_direct_contradictions_are_separated_from_correct_answers(self):
        # Agents 01-08 correctly say signatures establish authenticity and
        # integrity, so they should form one cluster. Agents 09-10 use much of the
        # same vocabulary but explicitly recommend accepting unsigned or invalid
        # messages. They should form a separate contradictory cluster because
        # negating the security requirement reverses the meaning of the answer.
        labels = cluster_fixture("direct_contradiction.json")

        self.assert_same_cluster(labels, [f"agent-{index:02d}" for index in range(1, 9)])
        self.assert_same_cluster(labels, ["agent-09", "agent-10"])
        self.assert_different_clusters(labels, "agent-01", "agent-09")

    @unittest.expectedFailure
    def test_competing_authentication_approaches_form_two_clusters(self):
        # Agents 01-05 recommend stateless JWT authentication. Agents 06-10
        # recommend stateful server-side sessions. Each approach is internally
        # compatible, but their state-management designs are mutually exclusive,
        # so topical similarity alone should not collapse them into one cluster.
        labels = cluster_fixture("competing_authentication_approaches.json")

        self.assert_same_cluster(labels, [f"agent-{index:02d}" for index in range(1, 6)])
        self.assert_same_cluster(labels, [f"agent-{index:02d}" for index in range(6, 11)])
        self.assert_different_clusters(labels, "agent-01", "agent-06")

    @unittest.expectedFailure
    def test_generic_bridge_answers_do_not_chain_incompatible_database_choices(self):
        # Agents 01-04 explicitly select PostgreSQL; agents 07-10 explicitly
        # select MongoDB. These choices should remain separate. Agents 05-06 only
        # give generic database-selection criteria and should not act as semantic
        # bridges that merge two incompatible concrete recommendations.
        labels = cluster_fixture("semantic_bridge.json")

        self.assert_same_cluster(labels, [f"agent-{index:02d}" for index in range(1, 5)])
        self.assert_same_cluster(labels, [f"agent-{index:02d}" for index in range(7, 11)])
        self.assert_different_clusters(labels, "agent-01", "agent-07")
        for bridge_agent in ["agent-05", "agent-06"]:
            self.assert_different_clusters(labels, bridge_agent, "agent-01")
            self.assert_different_clusters(labels, bridge_agent, "agent-07")

    @unittest.expectedFailure
    def test_reversed_key_roles_are_separated_from_correct_key_roles(self):
        # Agents 01-07 correctly state that private keys sign and public keys
        # verify. Agents 08-10 reverse those roles. Because the reversal makes the
        # answer factually wrong, those agents should not join the correct cluster
        # even though almost all of their cryptographic vocabulary is identical.
        labels = cluster_fixture("subtle_key_role_reversal.json")

        self.assert_same_cluster(labels, [f"agent-{index:02d}" for index in range(1, 8)])
        self.assert_same_cluster(labels, [f"agent-{index:02d}" for index in range(8, 11)])
        self.assert_different_clusters(labels, "agent-01", "agent-08")

    @unittest.expectedFailure
    def test_presentation_style_does_not_fragment_equivalent_answers(self):
        # Agents 01-08 all prescribe idempotency keys, but express the solution as
        # prose, steps, pseudocode, and short or detailed explanations. Formatting
        # should not split semantically equivalent answers. Agents 09-10 discuss
        # gardening and music, so neither belongs in the dominant cluster or with
        # the other unrelated answer.
        labels = cluster_fixture("presentation_styles.json")

        self.assert_same_cluster(labels, [f"agent-{index:02d}" for index in range(1, 9)])
        self.assert_different_clusters(labels, "agent-01", "agent-09")
        self.assert_different_clusters(labels, "agent-01", "agent-10")
        self.assert_different_clusters(labels, "agent-09", "agent-10")

    @unittest.expectedFailure
    def test_unrelated_word_injection_does_not_change_a_correct_answer_cluster(self):
        # Agents 01-07 provide compatible signature-verification answers. Agent 08
        # gives a complete correct answer and then appends the unrelated words
        # "recipe bowls sushi". Those words do not negate or modify the answer, so
        # agent 08 should remain in the correct cluster. Agents 09-10 are entirely
        # unrelated and should remain separate from it and from each other.
        labels = cluster_fixture("unrelated_word_injection.json")

        self.assert_same_cluster(labels, [f"agent-{index:02d}" for index in range(1, 9)])
        self.assert_different_clusters(labels, "agent-01", "agent-09")
        self.assert_different_clusters(labels, "agent-01", "agent-10")
        self.assert_different_clusters(labels, "agent-09", "agent-10")


if __name__ == "__main__":
    unittest.main()
