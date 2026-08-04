# Mini-round-two test data

Mini-round two reaches consensus over answer classifications. It does not
generate the final response: answer producers first create and sign candidate
answers, committee judges classify those candidates, and validators derive the
same canonical category groups from a verified classification certificate.

## Round boundary

Answer generation is off-round. Model execution has variable latency, so
producers prepare answers before the timed MR2 classification flow. During MR2,
validators verify signed answer evidence, judge a canonical candidate set,
exchange signed classification votes, aggregate them deterministically, and
finalize transaction statuses.

This boundary keeps consensus liveness independent of the slowest answer model
while binding every off-round answer to its transaction through signed evidence
and hashes. The complete protocol rules are documented in
[`consensus/miniround2/README.md`](../../../consensus/miniround2/README.md).

## Deterministic scenarios

The `scenarios/` directory contains the executable MR1-to-MR2 fixtures. Each
tracked `scenario.json` controls labels, producer answers, judge behavior,
network roles, faults, and expected finalization. These tests use fake judges so
source-code regressions can be reproduced without Ollama.

The scenarios cover successful classification, disagreement across categories,
insufficient correct answers, prompt injection, malformed judge output, judge
errors, quorum timeouts, invalid evidence, Byzantine signed votes, tampered
leader certificates, and reordered message arrival.

See [`scenarios/README.md`](scenarios/README.md) for the scenario catalog and
instructions for adding fixtures.

## Live and distributed coverage

Real-agent diverse tests keep answer inputs controlled while exercising live
judging. Distributed tests use separately configured agent services and cover
multi-node convergence and configurable adversarial behavior. Permanent reports
are stored under `testresults/`:

- `experiment-local-shared-agent.md` preserves the retired single-service
  baselines and their interpretation.
- `experiment-distributed-mr2.md` contains the distributed experiment results.

Raw JSONL measurements and generated scenario `result.json` files are local
outputs and are intentionally ignored by Git.

## Running deterministic MR2 tests

```sh
go test -tags integration ./integrationtests \
  -run TestMiniRoundOneToMiniRoundTwoScenarios
```
