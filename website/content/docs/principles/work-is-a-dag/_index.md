---
title: Work Is a DAG
weight: 5
---

# Work is a DAG

> Work is not a list. It is a directed acyclic graph.

## The principle

Tasks have prerequisites, blockers, and downstream consequences, and
pretending otherwise produces either false serial ordering (slow) or
chaotic parallelism (broken). A directed acyclic graph is the honest
data structure: each unit of work names what must exist before it
starts and what becomes possible once it finishes.

In a factory where multiple agents run in parallel, the DAG is also the
scheduler — the queue dispatches what is ready and holds what is
blocked, without anyone having to remember the order. Modeling work as
a DAG forces dependencies into the open, where they can be inspected,
broken, or parallelized; modeling work as a list hides them, where they
re-emerge as merge conflicts and rework.

The structural discipline — *every dependency declared, no implicit
ones* — is what makes the rest possible. The scheduler does not need to
understand the semantics of any task; only the topology. That is
sufficient to extract parallelism the user did not have to design for
explicitly, to identify the critical path, and to surface what is
actually blocking what without asking anyone to reconstruct it from
issue-tracker prose.

## Evidence

- **REF-022 — CPM/PERT (1957–1958).** Project work modeled as nodes
  (tasks) and edges (dependencies); the longest path through the graph
  is the critical path. Seventy years of project management practice
  has not produced a better abstraction.
- **REF-023 — Bazel.** Every build action is a node with declared
  inputs and outputs; the dependency graph is hermetic and
  content-addressed. Incremental rebuilds, remote caching, and
  reproducibility all fall out of the graph being explicit.
- **REF-024 — Dask.** Generalizes the pattern to runtime computation:
  arbitrary Python work as a task graph, scheduled across cores or
  clusters. Independent branches run in parallel; dependent ones wait.
- **RSCH-005.** Three independent fields converged on the same
  abstraction because the abstraction is correct.

See `docs/helix/00-discover/research/RSCH-005-work-is-a-dag.md`.

## DDx response

- **Beads have explicit `depends_on` edges.** `ddx bead dep add` and
  `ddx bead dep remove` manage them; the graph is queryable via
  `ddx bead dep tree`.
- **The queue partitions ready and blocked.** `ddx bead ready` walks
  the graph to surface beads whose dependencies are satisfied;
  `ddx bead blocked` shows the rest with the edges that hold them.
- **Execute-loop respects topological order.** It drains the ready set
  and can fan out parallel agents on independent branches, the same
  way Dask and Bazel schedulers do, because the graph is explicit and
  the scheduler does not have to guess.
- **Critical-path information is implicit in the edges.** Humans see
  what is blocking what without having to reconstruct the topology
  from prose.
