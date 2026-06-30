# Answer Clustering

## Purpose

This component groups semantically similar validator answers for the same
transaction. It is intended to support dominant-cluster detection before a later
aggregation stage.

The current implementation detects semantic similarity. It does not determine
whether an answer is factually correct.

## Components

### Embedding

[`embed.py`](embed.py) exposes:

```python
embed_answers(answers: list[str]) -> list[list[float]]
```

Embeddings remain in memory and are never written to an intermediate file.

The embedding configuration is:

```text
model: sentence-transformers/all-MiniLM-L6-v2
revision: 1110a243fdf4706b3f48f1d95db1a4f5529b4d41
device: CPU
normalization: L2-normalized embeddings
```

The exact model revision and CPU execution are fixed to reduce differences
between executions. Tests also seed Python, NumPy, and PyTorch and enable
deterministic PyTorch algorithms.

### Clustering

[`cluster.py`](cluster.py) performs hierarchical clustering independently for
each `txHash`.

The current defaults are:

```text
distance metric: cosine distance
linkage: single
distance threshold: 0.3
```

SciPy provides the implementation:

```python
scipy.spatial.distance.pdist(..., metric="cosine")
scipy.cluster.hierarchy.linkage(...)
scipy.cluster.hierarchy.fcluster(..., criterion="distance")
```

Supported linkage methods are:

```text
single
complete
average
weighted
```

The linkage method and threshold can be changed without changing source code:

```bash
.venv/bin/python clustering/cluster.py answers.json clusters.json \
  --linkage complete \
  --distance-threshold 0.3
```

The input is a JSON array. Additional fields are preserved:

```json
[
  {
    "agentId": "agent-01",
    "txHash": "tx-001",
    "prompt": "Why should validators verify signatures?",
    "answer": "Signatures establish authenticity and integrity."
  }
]
```

The output adds `clusterId` but does not expose embeddings:

```json
[
  {
    "agentId": "agent-01",
    "txHash": "tx-001",
    "prompt": "Why should validators verify signatures?",
    "answer": "Signatures establish authenticity and integrity.",
    "clusterId": 0
  }
]
```

Cluster IDs have no semantic meaning. Only membership matters.

## Linkage behavior

Single linkage uses the closest pair across two clusters. It can join a new
answer when that answer is close to only one member of a larger cluster. This
allows semantic chains.

Complete linkage uses the most distant pair across two clusters. Every member
of a merged cluster must remain within the selected linkage cutoff. It resists
chains but can fragment valid paraphrases.

Average and weighted linkage are available for later experiments but have not
yet been evaluated by the current test analysis.

## Tests

All tests use the real pinned embedding model. There are no mocked embeddings.
Fixtures are stored in [`testdata`](testdata).

Run the suite with a cached model:

```bash
HF_HUB_OFFLINE=1 .venv/bin/python -m unittest \
  clustering.test_cluster \
  clustering.test_research_scenarios \
  -v
```

### 1. Obvious dominant cluster

Fixture: [`obvious_dominant_cluster.json`](testdata/obvious_dominant_cluster.json)

Eight agents give compatible answers about signature authenticity and message
integrity. Two agents answer unrelated questions about photosynthesis and
cooking.

Expected and observed with single linkage:

```text
[8, 1, 1]
```

This is the baseline case and passes.

### 2. Topical but factually wrong answers

Fixture: [`topical_but_wrong_answers.json`](testdata/topical_but_wrong_answers.json)

Eight answers are correct. Agent 09 incorrectly claims that signatures encrypt
the full message. Agent 10 incorrectly claims that a signature proves factual
correctness and prevents blockchain forks.

Observed with single linkage:

```text
[9, 1]
```

Agent 09 joins the correct cluster because it is semantically close to the
signature topic. Agent 10 remains separate. This passing test records the fact
that semantic proximity does not imply factual correctness.

### 3. Direct contradiction

Fixture: [`direct_contradiction.json`](testdata/direct_contradiction.json)

Agents 01-08 support signature verification. Agents 09-10 use similar vocabulary
but recommend accepting unsigned or invalidly signed messages.

Logical expectation:

```text
[8, 2]
```

Observed with single linkage:

```text
[9, 1]
```

The embedding model does not reliably represent negation or reversed intent.
This research test is marked as an expected failure.

### 4. Competing authentication approaches

Fixture:
[`competing_authentication_approaches.json`](testdata/competing_authentication_approaches.json)

Five agents recommend stateless JWT authentication. Five recommend stateful
server-side sessions. Both groups are topically related but propose different
architectures.

Logical expectation:

```text
[5, 5]
```

Observed with single linkage:

```text
[3, 3, 1, 1, 1, 1]
```

The `0.3` threshold fragments both coherent approaches. This research test is an
expected failure.

### 5. Semantic bridge

Fixture: [`semantic_bridge.json`](testdata/semantic_bridge.json)

Four agents explicitly select PostgreSQL, four explicitly select MongoDB, and
two give generic database-selection advice. The generic answers should not chain
the incompatible concrete choices together.

Logical expectation:

```text
[4, 4, 1, 1]
```

Observed with single linkage:

```text
[7, 1, 1, 1]
```

Single linkage creates a mixed cluster containing PostgreSQL and MongoDB
recommendations. This illustrates the chaining problem and is an expected
failure.

### 6. Subtle key-role reversal

Fixture:
[`subtle_key_role_reversal.json`](testdata/subtle_key_role_reversal.json)

Seven agents correctly say that private keys sign and public keys verify. Three
agents reverse the roles while retaining almost identical terminology.

Logical expectation:

```text
[7, 3]
```

Observed with single linkage:

```text
[8, 1, 1]
```

Some reversed answers join correct answers while some correct paraphrases are
excluded. This is an expected failure caused by using semantic similarity as a
proxy for logical compatibility.

### 7. Presentation styles

Fixture: [`presentation_styles.json`](testdata/presentation_styles.json)

Eight answers describe payment idempotency using short prose, detailed prose,
numbered steps, and pseudocode. Two answers are unrelated.

Logical expectation:

```text
[8, 1, 1]
```

Observed with single linkage:

```text
[1, 1, 1, 1, 1, 1, 1, 1, 1, 1]
```

The current threshold is too strict for these stylistically diverse answers.
This is an expected failure.

### 8. Unrelated-word injection

Fixture:
[`unrelated_word_injection.json`](testdata/unrelated_word_injection.json)

Agents 01-07 give compatible correct answers. Agent 08 gives a complete correct
answer and appends:

```text
recipe bowls sushi
```

The appended words do not negate or modify the answer, so agent 08 should remain
in the correct cluster. Agents 09-10 are entirely unrelated.

Logical expectation:

```text
[8, 1, 1]
```

Observed with single linkage:

```text
[7, 1, 1, 1]
```

The injected words move the embedding far enough that agent 08 becomes a
singleton. This demonstrates sensitivity to irrelevant appended content and is
an expected failure.

## Single-linkage and complete-linkage analysis

[`diagnose.py`](diagnose.py) calculates pairwise distances, distribution
statistics, cluster memberships, and merge distances:

```bash
HF_HUB_OFFLINE=1 .venv/bin/python -m clustering.diagnose \
  clustering/testdata/obvious_dominant_cluster.json \
  clustering/testdata/topical_but_wrong_answers.json
```

For the eight correct signature-verification answers, the 28 pairwise cosine
distances are summarized as follows:

| Minimum | Maximum | Mean | Median |
|---:|---:|---:|---:|
| 0.077052 | 0.353224 | 0.239964 | 0.257870 |

The maximum correct-to-correct distance exceeds `0.3`. Consequently, complete
linkage cannot put all eight correct answers into one cluster at the current
threshold.

### Unrelated-answer scenario

| Distribution | Minimum | Maximum | Mean | Median |
|---|---:|---:|---:|---:|
| Agent 09 to correct answers | 0.956454 | 1.029765 | 0.983047 | 0.976134 |
| Agent 10 to correct answers | 0.978198 | 1.081036 | 1.015422 | 1.003931 |

```text
single:   [8, 1, 1]
complete: [4, 3, 1, 1, 1]
```

Both incorrect answers remain outside under both methods. Complete linkage
fragments the correct answers.

### Topical-but-wrong scenario

| Distribution | Minimum | Maximum | Mean | Median |
|---|---:|---:|---:|---:|
| Agent 09 to correct answers | 0.249751 | 0.407046 | 0.300768 | 0.292422 |
| Agent 10 to correct answers | 0.393594 | 0.566268 | 0.468641 | 0.472392 |

```text
single:   [9, 1]
complete: [4, 4, 1, 1]
```

Under single linkage, agent 09 joins through agent 01 at distance `0.249751`.
Only one close cross-cluster pair is needed, so agent 09 becomes connected to the
entire correct cluster.

Under complete linkage, agent 09 joins only agents 01, 03, and 06 at merge
distance `0.280837`. It cannot merge further at threshold `0.3` because its
distances to agents 04, 05, 07, and 08 are respectively:

```text
0.407046, 0.324765, 0.304008, 0.316106
```

Complete linkage prevents agent 09 from joining every correct answer, but it
also fails to preserve the eight correct answers as one cluster.

## Conclusions

1. The obvious unrelated-outlier scenario works with single linkage at `0.3`.
2. Single linkage is vulnerable to semantic chaining and can absorb incorrect or
   incompatible answers through one nearby answer.
3. Complete linkage reduces chaining, but `0.3` is too strict for the current
   correct paraphrases.
4. The embedding model captures topic and wording more reliably than negation,
   factual correctness, or logical compatibility.
5. A single global threshold does not currently generalize across signature,
   authentication, database, and payment examples.
6. Irrelevant appended words can materially move an otherwise correct answer's
   embedding.
7. Neither linkage method should be selected as the production protocol solely
   from the current fixtures. Average and weighted linkage, threshold selection,
   preprocessing, and a larger labeled evaluation set still need analysis.

The default remains single linkage with threshold `0.3` until those experiments
are completed.

## Limitations and missing protocol behavior

The current implementation has the following limitations:

- Semantic similarity does not prove factual correctness.
- Contradictions and negations may receive similar embeddings.
- Single linkage can create chain effects.
- Complete linkage can fragment valid paraphrases.
- The threshold is manually selected and not calibrated on a representative
  dataset.
- Formatting, verbosity, code, and irrelevant suffixes can change distances.
- CPU execution and a pinned model revision improve repeatability, but exact
  floating-point equality across all hardware and dependency versions is not
  guaranteed.
- Model and dependency artifacts are not yet verified against protocol-level
  cryptographic hashes.
- The model is loaded for every call rather than held by a long-lived service.
- Input validation does not yet enforce one answer per validator.
- Signature verification and answer-hash verification are outside this Python
  component.
- Quorum calculation, dominant-cluster selection, tie-breaking between equally
  sized clusters, and `Answered`/`NotAnswered` status are not implemented.
- The tests are small, manually constructed research cases and do not represent
  the full distribution of production answers.

## Current test status

The two baseline tests pass. The six research tests express the logically desired
membership and are marked as expected failures because the current configuration
does not satisfy them:

```text
Ran 8 tests
OK (expected failures=6)
```
