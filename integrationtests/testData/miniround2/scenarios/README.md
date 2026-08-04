# Mini-round-two integration scenarios

This directory contains deterministic fixtures for the complete MR1-to-MR2
flow. Each scenario supplies controlled labels, answers, judge behavior, network
parameters, and expected protocol outcomes. The suite uses fake judges so that
consensus behavior can be reproduced without an LLM service.

## Files

Every scenario directory contains a tracked `scenario.json`. The test may write
an ignored `result.json` for local inspection; results are not test inputs and
must not be committed.

Fixtures are loaded with strict JSON decoding. Unknown fields, inconsistent
network parameters, missing transaction expectations, or invalid role counts
fail before the round begins. See `miniroundtwo_fixture_test.go` for the schema
and validation rules.

## Scenarios

- `unanimous_correct`: all candidate answers are accepted.
- `four_category_disagreement`: votes cover all classification categories.
- `insufficient_correct_answers`: the round finalizes without enough correct
  answers to advance.
- `prompt_injection_answer`: adversarial instructions are classified as answer
  content, not executed as protocol instructions.
- `malformed_judge_response`: invalid judge output is rejected.
- `judge_execution_error`: judge failures do not create a valid vote.
- `classification_quorum_timeout`: insufficient votes prevent MR2 finalization.
- `invalid_answer_evidence`: malformed producer evidence is rejected.
- `byzantine_signed_classification_vote`: a signed but invalid vote is rejected.
- `leader_omits_certificate_vote`: incomplete certificates are rejected.
- `leader_reorders_certificate_votes`: canonical verification is independent of
  arrival order.
- `shuffled_valid_message_arrival`: valid asynchronous delivery still converges.

## Running the suite

```sh
go test -tags integration ./integrationtests \
  -run TestMiniRoundOneToMiniRoundTwoScenarios
```

## Adding a scenario

1. Copy the closest existing `scenario.json` into a directory whose name
   matches its `name` field.
2. Change only the network behavior or adversarial condition under test.
3. Define explicit expected finalization, producer, voter, status, and category
   counts.
4. Run the scenario suite and inspect assertion failures before updating any
   expectation.

Keep performance experiments and live-model observations out of these fixtures;
their permanent summaries belong under `testresults/`.
