# Bead Tracker

The bead tracker stores work items in JSONL format at `.ddx/beads.jsonl`. Each entry
contains a unique ID, title, description, acceptance criteria, labels, and dependency edges.
The tracker enforces a DAG structure on dependencies — circular edges are rejected at creation time.

Dependency resolution uses depth-first traversal. `ddx bead dep tree <id>` prints the full
dependency DAG with cycle detection enabled and exit code 1 on cycles.
