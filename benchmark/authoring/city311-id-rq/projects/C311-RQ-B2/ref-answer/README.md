# Hidden Reference Answer A

This directory will hold a separate, runnable `B + R` source tree. It is not
part of the agent-facing package.

The currently sealed `C311-RQ-B2-reference.tar.gz` is deliberately not copied
here: its Compose environment points CivicWorks at an unavailable example host
and does not run a deterministic customer fixture. It cannot prove `C-RQ-B2-01`
and is therefore a candidate source snapshot, not the authoritative answer.

The authoritative answer must include the CivicWorks fixture, fixture call log,
signed callback flow, source-to-destination mapping persistence, and all
validators in `../evaluation` passing.
