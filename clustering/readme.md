````markdown
# Answer Clustering Component

## Purpose

This component is responsible for clustering validator answers in Mini-Round 2.

The goal is not to find identical answers and not to produce the final user-facing response. Instead, the component detects whether there is a dominant group of semantically compatible answers that can be passed to Mini-Round 3.

Mini-Round 3 will use the dominant cluster as input for a leader/aggregator agent, following the Mixture-of-Agents idea: multiple compatible answers are synthesized into a better final answer.

## Problem Statement

For each transaction, Mini-Round 2 receives a set of signed answers from validators.

Input:

```text
txHash -> [signedAnswer_1, signedAnswer_2, ..., signedAnswer_n]
````

The clustering component must decide:

```text
Does there exist a dominant semantic cluster with at least 2f + 1 unique validator answers?
```

If yes, only that dominant cluster is forwarded to Mini-Round 3.

If no dominant cluster exists before the execution window expires, the transaction is marked as:

```text
NotAnswered
```

## Important Conceptual Rule

This component performs semantic agreement detection, not exact answer matching.

Two answers may belong to the same cluster even if they are phrased differently, as long as they express a compatible solution direction.

For example, these answers may belong to the same cluster:

```text
Use short-lived JWT access tokens and refresh tokens.
Create login and refresh endpoints, validate token expiration and signature.
Use middleware to validate JWTs and rotate refresh tokens securely.
```

They are not identical, but they are semantically compatible and useful as input for Mini-Round 3.

## Recommended Algorithm

The recommended first implementation is:

```text
deterministic embeddings
+ cosine distance
+ complete-linkage hierarchical clustering
+ fixed semantic distance threshold
```

The algorithm must not require a predefined number of clusters.

The component should not use `k = 2`, k-means with fixed `k`, or any method that forces answers into a fixed number of groups.

## Dominant Cluster Rule

For a transaction `txHash`, a dominant cluster exists if:

```text
exists cluster C such that:
    size(C) >= quorum
```

where:

```text
quorum = 2f + 1
```

In Approach 1 of Mini-Round 2, all validators in the selected consensus group execute all transactions, so the quorum is computed from the Mini-Round 2 consensus group size:

```text
f = floor((consensusGroupSize - 1) / 3)
quorum = 2f + 1
```

## Component API

The implementation should expose a function similar to:

```text
ClusterAnswers(request) -> ClusterResult
```

Suggested request structure:

```text
ClusterRequest:
    roundId
    miniRoundId
    blockHash
    txHash
    consensusGroupSize
    quorum
    answers[]
    config
```

Suggested answer structure:

```text
SignedAnswer:
    validatorId
    txHash
    answerText
    answerHash
    signature
```

Suggested config structure:

```text
ClusteringConfig:
    embeddingModelId
    embeddingModelHash
    preprocessingVersion
    distanceMetric
    clusteringAlgorithm
    distanceThreshold
    numericPrecision
    tieBreakingRule
```

Suggested result structure:

```text
ClusterResult:
    txHash
    status: Answered | NotAnswered
    dominantCluster
    allClusters
    rejectedAnswers
    metadata
```

Where:

```text
dominantCluster:
    clusterId
    members[]
    size
    averageDistance
    maxPairwiseDistance
```

## Determinism Requirements

The clustering result must be reproducible by every validator.

The following must be fixed and versioned:

```text
embedding model
model version/hash
tokenizer version
preprocessing rules
embedding precision or quantization
distance metric
clustering algorithm
distance threshold
answer ordering
tie-breaking rules
```

All input answers must be ordered deterministically before clustering, for example by:

```text
validatorId ascending
```

or:

```text
answerHash ascending
```

The chosen rule must be part of the protocol configuration.

## Preprocessing

Before embedding, each answer should pass through a deterministic preprocessing step.

The first version may include:

```text
normalize line endings
trim leading/trailing whitespace
normalize repeated blank lines
preserve code blocks
preserve markdown structure
```

Do not aggressively rewrite the answer. The goal is to remove formatting noise, not to change the semantic content.

## Clustering Procedure

For each transaction:

```text
1. Receive all valid signed answers available for txHash.
2. Remove duplicate answers from the same validator.
3. Sort answers using the canonical ordering rule.
4. Preprocess each answer deterministically.
5. Compute one embedding per answer.
6. Compute the pairwise cosine distance matrix.
7. Start with each answer as its own cluster.
8. Repeatedly merge the two closest clusters only if the complete-linkage distance is <= threshold.
9. Stop when no valid merge remains.
10. Check whether any cluster has at least quorum unique validators.
11. If yes, return Answered and the dominant cluster.
12. If no and the execution window expired, return NotAnswered.
```

Complete-linkage distance between two clusters is defined as:

```text
distance(clusterA, clusterB) =
    max distance(answer_i, answer_j)
    for answer_i in clusterA
    for answer_j in clusterB
```

This is intentionally strict. It prevents chain effects where answer A is close to B, B is close to C, but A is not close to C.

## Tie-Breaking

If multiple clusters satisfy the quorum condition, select the dominant cluster deterministically.

Recommended order:

```text
1. largest cluster size
2. lowest max pairwise distance
3. lowest average pairwise distance
4. lexicographically smallest sorted list of validatorIds
```

The same tie-breaking rule must be used by all nodes.

## Answered vs NotAnswered

A transaction can be marked as `Answered` as soon as a valid dominant cluster exists.

A transaction can be marked as `NotAnswered` only after:

```text
all expected answers were received
```

or:

```text
the execution window expired
```

The absence of an early cluster is not enough to mark the transaction as `NotAnswered`, because later answers may still form a dominant cluster.

## Validation Expectations

The clustering component may assume that signature verification is performed before clustering, but each answer must still contain enough metadata to bind it to:

```text
roundId
miniRoundId
blockHash
txHash
validatorId
answerHash
```

The final Mini-Round 2 artifact should include enough information for validators to recompute and verify the clustering result.

## Testing Requirements

The implementation should include tests for:

```text
single dominant cluster exists
no dominant cluster exists
multiple clusters but none reaches quorum
multiple valid clusters with deterministic tie-breaking
duplicate answer from same validator
same answers in different input order
threshold too strict
threshold too permissive
cluster created only after additional answers arrive
NotAnswered only after timeout/all answers
```

The most important determinism test:

```text
Given the same answers in different input orders,
the component must return exactly the same clusters and dominant cluster.
```

## Out of Scope for First Version

The first implementation should not include:

```text
exact match clustering as the main method
LLM-as-a-judge in the consensus-critical path
test-based code execution
AST-based code comparison
language-specific static analysis
spectral clustering as the canonical algorithm
```

These may be explored later, but the first version should focus on deterministic semantic clustering using embeddings and complete-linkage hierarchical clustering.

## Limitations

This component does not prove that an answer is objectively correct.

It only proves that at least `2f + 1` validators produced semantically compatible answers.

For coding-related prompts, future versions may improve correctness detection using:

```text
code-aware embeddings
test execution
static analysis
structured answer formats
domain-specific validation rules
```

## Expected Outcome

For each transaction, the component must return either:

```text
Answered:
    dominant cluster with at least 2f + 1 semantically compatible signed answers
```

or:

```text
NotAnswered:
    no dominant cluster was found before the execution window ended
```

Only `Answered` transactions are forwarded to Mini-Round 3.

```
```
