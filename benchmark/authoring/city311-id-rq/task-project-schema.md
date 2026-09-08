# Task Project Schema

Every released City 311 task is a self-contained project directory:

```text
TASK-ID/
  B/                         # supplied customer system and inherited constraints
    customer-system/          # runnable seed source/image, with the increment absent
    customer-context.md       # organisation, roles, data, owned systems
    constraints.md            # inherited hard and scored constraints
    fixtures/                 # synthetic data and customer-service fixtures
  R/                          # agent-facing delivery request
    requirement.md
  ref-answer/                 # author-only, never supplied to an agent
    customer-system/          # complete runnable implementation of B + R
    provenance.yaml
  evaluation/                 # author-only validators and score policy
    scoring.yaml
    validator-dag.yaml
  task.yaml                   # immutable IDs, hashes, and release status
```

`B/customer-system` and `ref-answer/customer-system` are different source
trees. The latter is not a patch description: it must start from B, implement
R, and pass the full evaluation against the same fixtures. A task is
releaseable only when both source-tree hashes and the fixture image digests are
recorded in `task.yaml`.

The old `*-reference.tar.gz` files are reference-source candidates only. They
do not make a task releaseable until a task-specific B tree and evaluation
bundle exist.
