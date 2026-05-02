---
ddx:
  id: RSCH-005
  status: draft
  depends_on:
    - REF-022
    - REF-023
    - REF-024
id: RSCH-005
title: "Work Is a DAG"
kind: research-synthesis
summary: "Project schedules, build systems, and parallel computation independently converged on the directed acyclic graph as the right substrate for managing dependent work."
tags: [dag, scheduling, ddx-principle]
---

# Work Is a DAG

## Principle

Units of work and their dependencies form a directed acyclic graph. Treating
that graph as a first-class object — explicit, queryable, schedulable —
unlocks parallelism, incremental execution, and reasoning about the critical
path. Treating it as implicit prose produces serial bottlenecks and lost
parallelism.

## Synthesis

CPM and PERT (REF-022) introduced the pattern in 1957–1958: model a project
as nodes (tasks) and edges (dependencies), compute the longest path through
the graph, and identify which tasks are on the critical path. Seventy years
of project management practice has not produced a better abstraction. The
DuPont Polaris missile program and chemical-plant maintenance scheduling
both used the same machinery — task nodes, dependency edges, durations,
critical path — that modern issue trackers reinvent piecemeal.

Bazel (REF-023) carried the same structure into software builds. Every
build action is a node with declared inputs and outputs; the dependency
graph is hermetic and content-addressed. The payoff is operational:
incremental rebuilds touch only what changed, remote caching shares
results across the team, and the build is reproducible because the graph
fully describes the work. The structural discipline — *every dependency
declared, no implicit ones* — is what makes the rest possible.

Dask (REF-024) generalized the idea to runtime computation: arbitrary
Python work is represented as a task graph and scheduled across cores or
clusters. Independent branches run in parallel; dependent ones wait. The
scheduler does not need to understand the semantics of any task — only
the graph topology — and that is sufficient to extract parallelism that
the user did not have to design for explicitly.

The convergence is the point. Three independent fields — project
management, build systems, parallel computation — arrived at the same
abstraction because the abstraction is correct. Anything that manages
dependent units of work eventually grows into a DAG; the choice is whether
that DAG is explicit and tooled or implicit and brittle.

## DDx Implication

The DDx bead tracker is the explicit DAG for AI-assisted work. `ddx bead
dep add` declares edges; `ddx bead ready` walks the graph to find beads
whose dependencies are satisfied; `ddx bead blocked` and `ddx bead dep
tree` make the topology queryable. The execute-loop drains the ready set
and can fan out parallel agents on independent branches, the same way
Dask or Bazel do — because the graph is explicit, the scheduler does not
have to guess. Beads carry critical-path information by virtue of their
edges; humans see what is blocking what without having to reconstruct it
from issue-tracker prose.
