---
ddx:
  id: SD-013
  depends_on:
    - FEAT-013
    - FEAT-002
    - FEAT-004
    - FEAT-006
    - SD-004
---
# Solution Design: Multi-Agent Coordination

## Purpose

This design specifies how DDx makes multi-agent and multi-machine work safe
and observable without becoming an orchestration framework. Coordination
policy stays in HELIX and other workflow tools; DDx provides the primitives.

## Scope

- Machine/session identity in bead claims
- MCP write tools for beads (remote supervisor surface)
- Worktree-aware agent dispatch
- Git as the coordination bus
- Division of responsibility between DDx and workflow tools

Out of scope: machine provisioning, process supervision, token budget
management, automatic merge conflict resolution.

## Bead Concurrency: Machine/Session Identity in Claims

Today claims record `claimed-pid` and `assignee`. This is insufficient for
multi-machine use because PIDs collide across hosts and `assignee` is advisory.

**Changes to the claim record:**

- Add `claimed-machine` (hostname or `$DDX_MACHINE_ID` env override)
- Add `claimed-session` (agent session ID if invoked from `ddx agent run`,
  empty otherwise)
- Add `claimed-worktree` (worktree branch name when `--worktree` is set)

These three fields augment rather than replace the existing claim fields. The
claim algorithm in SD-004 is unchanged; the new fields are populated at step 4
alongside `claimed-at` and `claimed-pid`.

Claims remain advisory. No hard reservation lock is introduced. The three new
fields allow a coordinator to answer: "which machine, session, and branch holds
this bead?" without polling external state.

**Concurrency safety:** The existing atomic temp-file swap and directory lock
(SD-004) already serialize concurrent writers. No changes to the write path are
required. Multiple agents racing to claim the same bead will serialize; the
last writer wins, which is the current behavior.

## MCP Supervisor Surface

`ddx server` (FEAT-002) today exposes read-only bead and document endpoints.
A remote supervisor needs write access to steer work without running the CLI
locally.

**New MCP tools (server-side):**

| Tool | Description |
|------|-------------|
| `ddx_bead_create` | Create a bead in the active-work collection |
| `ddx_bead_update` | Update status, title, or labels on an existing bead |
| `ddx_bead_claim` | Claim or unclaim a bead with caller identity |

These tools are thin wrappers over the existing bead store methods. They use
the same atomic write path as the CLI. No new storage logic is introduced.

**Existing read tools remain unchanged.** The supervisor can already read bead
state, execution results, and agent session logs via existing MCP resources.

**Authentication:** Non-localhost access is authenticated via ts-net (Tailscale),
per ADR-006. The server binds to localhost by default; enabling `server.tsnet`
in `.ddx/config.yaml` adds a Tailscale listener with identity-based auth at
the transport layer. No API keys or custom auth middleware are required.

**MCP notifications (future):** Real-time notifications on state change (bead
transitions, execution completions) are deferred. The supervisor can poll bead
state via existing list/show endpoints in the near term.

## Worktree-Aware Dispatch

`ddx agent run` today accepts `--workdir` to set the working directory of the
agent invocation.

**New flag:** `--worktree <name>`

Behavior:
1. If the named worktree does not exist, create it with
   `git worktree add .worktrees/<name> -b <name>`.
2. Set the agent working directory to the worktree path.
3. Populate `claimed-worktree` on any bead claim made during the run.
4. `ddx bead show` renders `claimed-worktree` alongside other claim fields.

The `--workdir` flag continues to work unchanged. `--worktree` is additive.

## Git as Coordination Bus

DDx does not introduce a coordination service. All durable state lives in
git-tracked files. The coordination pattern for multi-machine work:

1. Coordinator creates a worktree branch and pushes it to the remote.
2. Worker (another machine or account) fetches the branch and runs
   `ddx agent run --worktree <branch>`.
3. Worker pushes results (code, updated beads, agent session logs) to the branch.
4. Coordinator fetches, reviews, and merges.

**What DDx provides in this flow:**
- `ddx bead claim` records which branch/machine holds the work
- `ddx agent run --worktree` creates the isolation boundary
- Bead JSONL and session collections are file-backed and merge-friendly
  (append-only JSONL rows are non-conflicting if agents write to separate
  collections or the merge strategy takes both sides)

**What DDx does not provide:**
- Push/pull scheduling or automation
- Branch merge strategy
- Conflict resolution policy

## What DDx Provides vs. Workflow Tools

| Concern | Owner |
|---------|-------|
| Safe concurrent bead writes | DDx (atomic swap + lock) |
| Machine/session identity in claims | DDx (claim fields) |
| Worktree creation and dispatch | DDx (`--worktree` flag) |
| MCP read surface for remote supervisors | DDx (existing server) |
| MCP write tools for beads | DDx (new tools, this design) |
| Which agent works on which bead | HELIX / workflow policy |
| When to merge a worktree branch | HELIX / workflow policy |
| Remote machine provisioning | Out of scope |
| Token budget management | Out of scope |

## Reference Systems

**Gastown** (anthropics/gastown): Demonstrates multi-agent work distribution
using claim semantics and a shared queue. The `claimed-machine` / `claimed-session`
pattern here mirrors Gastown's worker identity approach.

**MCP Agent Mailbox** (anthropics/agent-mailbox): Demonstrates async agent-to-
agent messaging over MCP. The MCP write tools for beads (`ddx_bead_claim`,
`ddx_bead_update`) provide a similar coordination surface without introducing
a separate mailbox service — beads are the messages.

## Validation

- `TestBeadClaimRecordsMachineAndSession` — claim fields populated correctly
- `TestBeadClaimWorktreeBranch` — worktree name stored in claim record
- `TestMCPBeadCreateTool` — MCP create tool writes via store
- `TestMCPBeadClaimTool` — MCP claim tool respects lock and atomic write
- `TestAgentRunWorktreeFlag` — creates worktree, sets workdir, records branch
- `TestAgentRunWorktreeFlagReusesExisting` — second run reuses existing worktree
