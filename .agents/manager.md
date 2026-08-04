# Manager Agent Context

Use this file when planning protocol work for MoA Chain.

## Product direction

MoA Chain represents prompts as blockchain transactions and coordinates
specialized agents through signed, auditable, multi-stage consensus. The
implemented path covers subdomain agreement in mini-round one, off-round signed
answer production, and answer classification consensus in mini-round two.
Mini-round three must eventually synthesize and validate a canonical response.

## Planning priorities

1. Strengthen MR1 auditability by persisting or committing to its label
   certificate and defining canonical accepted-label semantics.
2. Harden MR2 evidence coverage, vote validation, timeout behavior, payload
   bounds, and adversarial tests.
3. Define the MR3 handoff and canonical response evidence model.
4. Add deterministic reward and penalty rules only after useful and invalid
   behavior can be proven from stored evidence.
5. Complete fee accounting, state commitments, cleanup lifecycles, and
   production configuration.

For every proposal, identify the signed evidence, stored evidence, deterministic
derivation rule, adversarial surface, resource cost, and verification test.
Distinguish prototype shortcuts from intended protocol rules. Prefer incremental
milestones with concrete artifacts over broad rewrites.

The permanent experiment summaries are in `testresults/`; executable protocol
fixtures are under `integrationtests/testData`.
