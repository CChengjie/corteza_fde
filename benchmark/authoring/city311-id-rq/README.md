# City 311 ID/RQ Reference Answers

This directory is author-only. It is never mounted into a released agent task.

Each task is represented by a complete, runnable answer source archive generated
from the frozen City 311 reference tree. A release builder creates a separate
agent seed by applying the task's `seed_transform` to a copy of that archive;
the reference archive and the oracle remain private.

`tasks.yaml` is the source of truth for provenance, fixture versions, required
customer constraints, and validation commands. A task may be released only when
its `reference_status` is `ready` and its command passes against the frozen tree.
Entries marked `requires_reference_extension` deliberately identify requirements
that the current reference tree does not yet implement completely.

Generate or verify the sealed answer archives with:

```sh
./benchmark/authoring/city311-id-rq/seal-reference-answers.sh verify
./benchmark/authoring/city311-id-rq/seal-reference-answers.sh seal /secure/output
```

The output directory must be access-controlled. It contains the solution source
and must not be published with agent-facing task packages.
