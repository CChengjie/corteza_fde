# City311 Whole-System Task Family

`C311-SYS` treats the deployable City311 system and the Corteza-required core
as one customer delivery target. `P1` is the only whole-system greenfield
node: its B bundle contains customer fixtures and a non-product build harness,
but no Corteza or City311 implementation. Every later node is a canonical
brownfield evolution of the preceding answer.

This directory is an authoring registry, not an agent bundle. A node can be
released only after its project has two independently startable source trees:

```text
B/customer-system/source.tar.gz
ref-answer/customer-system/source.tar.gz
```

Both trees must start with the node's supplied runtime fixture contract. The
answer must implement only accumulated requirements through that node; copying
the final system into an earlier answer is invalid.

`system-task-catalog.yaml` is the authoritative construction order and records
the new requirement, inherited boundary obligations, fixtures, and oracle
classes for all 36 nodes.
