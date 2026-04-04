---
ddx:
  id: FEAT-013
  depends_on:
    - helix.prd
    - FEAT-002
    - FEAT-004
    - FEAT-006
    - FEAT-012
---
# Feature: Multi-Agent and Multi-Machine Coordination

**ID:** FEAT-013
**Status:** Framing
**Priority:** P1
**Owner:** DDx Team

## Overview

DDx needs to be safe and useful in a world where multiple agents — possibly
on different machines, using different models, working in parallel worktrees —
interact with the same project. This is not cluster orchestration. DDx stays
local-first and git-native. But it must design for coordination at the edges:
safe concurrent writes, observable state, and composable dispatch patterns.

## Problem Statement

**Current situation:**
- A human supervisor (the "side channel") runs alongside implementation
  agents, reviewing specs, evaluating test results, and steering beads. This
  works today by running `helix worker` as a background process locally.
- Sub-agents can be spawned locally in worktrees for parallel implementation,
  but merging their results is manual.
- There is no way to spread work across machines or agent accounts (for token
  budget distribution, compile/test parallelism, or model diversity).
- Concurrent bead updates from multiple agents can race on the JSONL store.

**Desired outcome:** DDx provides the primitives that make multi-agent
work safe and observable, without becoming an orchestration framework.
Orchestration policy stays in HELIX and other workflow tools.

## User Scenarios

### S1: Supervisory Side Channel (Human-in-the-Loop)

**Today:** A developer runs `helix worker` in the background while working in
a separate terminal (or Claude Code session). They review execution results,
update specs, evaluate bead completion, and steer the queue. Both the worker
and the supervisor operate on the same repo.

**Near-term need:** The supervisor should be able to operate from a different
machine or session without conflicting with the worker. This means:
- Bead state must be safe for concurrent read/write (already mostly true
  with append-only JSONL + atomic writes)
- Spec document edits from the supervisor must auto-commit (FEAT-012) so the
  worker sees them
- The supervisor needs read access to agent session logs and execution results
  without being co-located

**Is this an MCP server use case?** Yes. `ddx server` (FEAT-002) already
exposes beads, documents, agent sessions, and execution results over MCP and
HTTP. A supervisor on another machine can connect to the DDx MCP server to:
- Read bead state and queue status
- Read execution results and test output
- Read/write spec documents (FEAT-012)
- Inspect agent session logs

The missing pieces are:
- MCP write tools for beads (today beads are CLI-only writes)
- MCP notifications when state changes (bead created, execution completed)
- Authentication for non-localhost access

### S2: Federated Building in Parallel Worktrees

**Today:** A planner agent spawns sub-agents in git worktrees for orthogonal
implementation tasks. Each worktree is a full copy of the repo. When the
sub-agent finishes, the worktree is merged back (or abandoned if the work
failed).

**Near-term need:** This pattern should work across machines:
1. A coordinator creates a worktree branch for a task
2. Pushes the branch to the remote
3. A worker (on another machine, possibly another agent account) clones/fetches
   that branch, does the work, pushes results
4. The coordinator merges the results

**What DDx provides:**
- `ddx bead claim` already supports agent coordination (mark who's working
  on what)
- Bead state tracks which worktree/branch a bead is being worked in
- `ddx checkpoint` (FEAT-012) marks known-good states before/after merges
- `ddx agent run` can target a specific worktree via `--workdir`

**What DDx does NOT provide:**
- Machine provisioning or remote execution
- Branch merge strategy (that's git + workflow policy)
- Token budget management across accounts
- Process supervision (that's the OS/scheduler)

### S3: Observability Across Agents

Multiple agents working in parallel need a shared view of:
- Which beads are claimed, by whom, in which worktree
- Execution results from all agents' runs
- Spec document changes made by any agent

This is already mostly covered by the bead tracker and execution store being
file-backed and git-synced. The gaps are:
- Real-time notifications (agent A finishes a bead → agent B sees it
  immediately)
- Cross-worktree bead visibility (worktree A's bead updates aren't visible
  in worktree B until merged)

## Design Principles

### 1. Git is the coordination bus

All durable state lives in git-tracked files. Agents on different machines
coordinate by pushing/pulling branches. DDx does not introduce a separate
coordination service, message queue, or database.

### 2. Beads are the work distribution primitive

Beads with claim semantics are how work is allocated to agents. The
`claim/unclaim` pattern with PID tracking already prevents double-execution.
Extending this to include machine identity and branch tracking makes it
multi-machine safe.

### 3. MCP server is the remote observation surface

`ddx server` provides read (and limited write) access to project state for
remote supervisors and tools. This is the "side channel" that supervisors
use. It does not replace the CLI for heavy operations.

### 4. Worktrees are the isolation boundary

Each agent works in its own worktree. Merging is explicit. This avoids
concurrent file writes entirely — the only shared mutable state is git refs.

### 5. DDx provides primitives, not orchestration

DDx does not decide which agent works on which bead, when to merge, or how
to handle conflicts. That's workflow-level policy (HELIX). DDx provides:
- Safe concurrent bead operations
- Observable state over MCP/HTTP
- Worktree-aware agent dispatch
- Git-backed coordination primitives

## Requirements

### Functional (Near-term)

**Bead concurrency safety**
1. Bead claims include machine/session identity, not just PID
2. MCP write tools for beads: `ddx_bead_create`, `ddx_bead_update`,
   `ddx_bead_claim`
3. Bead operations safe under concurrent append to JSONL
   (already implemented via atomic writes — verify and document)

**MCP supervisor surface**
4. MCP tools for execution result inspection
5. MCP write tools for documents (FEAT-012)
6. MCP write tools for beads (new)
7. Optional authentication for non-localhost MCP access

**Worktree-aware dispatch**
8. `ddx agent run --worktree <name>` creates/reuses a worktree for the run
9. Bead claim records the worktree branch
10. `ddx bead show` displays the active worktree/branch for claimed beads

### Functional (Future — framing only)

**Remote worker pattern**
11. A coordinator can publish a "work package" (bead + branch + prompt) that
    a remote worker picks up
12. Workers report results by pushing to the branch and updating the bead
13. The coordinator merges completed branches

**Notification**
14. MCP server emits notifications on state changes (bead transitions,
    execution completions)
15. Subscribers can filter by label, status, or artifact ID

### Non-Functional

- **No new infrastructure:** Coordination uses git + existing MCP server.
  No message queues, databases, or orchestration services.
- **Degrade gracefully:** Single-machine, single-agent use is the default.
  Multi-agent features add capability without adding complexity for solo users.
- **Token budget awareness:** DDx should track token usage per agent session
  (already in agent logs) but does not manage budgets or accounts.

## Reference Systems

| System | Relevance |
|--------|-----------|
| [Gastown](https://github.com/anthropics/gastown) | Multi-agent coordination patterns, work distribution |
| [MCP Agent Mailbox](https://github.com/anthropics/agent-mailbox) | Async agent-to-agent messaging over MCP |
| Git worktrees | Isolation boundary for parallel work |
| Git push/pull | Coordination bus for multi-machine work |

## Affected Existing Specs

| Spec | Change |
|------|--------|
| FEAT-002 (Server) | Add bead write MCP tools, optional auth |
| FEAT-004 (Beads) | Add machine/session identity to claims |
| FEAT-006 (Agent) | Worktree-aware dispatch flag |

## Out of Scope

- Machine provisioning or remote execution
- Process supervision (use OS tools, systemd, etc.)
- Token budget management across accounts
- Automatic merge conflict resolution
- Real-time collaborative editing (use git)
- Building a distributed system — DDx stays local-first with git as the
  coordination bus
