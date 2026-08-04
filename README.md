# MoA Chain

MoA Chain is a proof-of-concept blockchain protocol for decentralized
Mixture-of-Agents inference. Prompts are submitted as transactions, validators
agree on their technical domains, specialized agents produce answers, and a
second validator committee classifies those answers from signed evidence.

## Implemented flow

1. **Mini-round one** selects a validator committee, proposes and validates a
   prompt batch, collects signed subdomain labels, and finalizes aggregated
   subdomain frequencies.
2. **Answer collection** runs outside the timed consensus round. Selected
   producers create answers and sign the resulting evidence.
3. **Mini-round two** verifies the answer evidence, asks committee members to
   classify every candidate, aggregates signed classification votes, and
   finalizes deterministic answer groups and transaction statuses.

The Go node contains the protocol and consensus implementation. `agent-python`
provides the HTTP service used for labeling, answering, and judging through
Ollama-compatible models.

## Repository map

- `cmd/node`: node entry point and configuration
- `consensus/miniround1`: prompt and subdomain consensus
- `consensus/miniround2`: answer-evidence and classification consensus
- `agent/httpclient`: Go client for the Python agent service
- `agent-python`: FastAPI agent service, prompts, providers, and tests
- `integrationtests`: deterministic, real-agent, and distributed protocol tests
- `testresults`: retained distributed experiment reports
- `scripts`: multi-host cluster lifecycle helpers

## Verify the repository

```sh
make test
make -C agent-python test
```

Tests that use a real model require Ollama and are exposed as explicit Make
targets. For example:

```sh
make test-realagent-mr1-group-a
make test-realagent-mr2-diverse
```

Distributed targets use `configs/cluster.json` and the scripts under `scripts/`.

## Protocol documentation

- [Mini-round one](consensus/miniround1/README.md)
- [Mini-round two](consensus/miniround2/README.md)
- [MR2 integration scenarios](integrationtests/testData/miniround2/scenarios/README.md)
- [Distributed MR1 results](testresults/experiment-distributed-mr1.md)
- [Distributed MR2 results](testresults/experiment-distributed-mr2.md)
- [Python agent service](agent-python/README.md)

## Current limitations

- MR1 finalizes the first valid quorum certificate, so frequency maps can vary
  between runs when different valid label votes arrive first.
- MR1 does not yet persist the complete label certificate needed for historical
  auditing.
- Mini-round three, reward and penalty accounting, and final canonical response
  generation are not implemented.
- The fee model, state commitments, and production lifecycle remain prototype
  quality.

Detailed determinism and evidence rules belong in the mini-round documentation
linked above.
