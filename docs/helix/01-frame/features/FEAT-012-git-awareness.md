---
ddx:
  id: FEAT-012
  depends_on:
    - helix.prd
    - FEAT-001
    - FEAT-002
    - FEAT-007
---
# Feature: Git Awareness and Revision Control Integration

**ID:** FEAT-012
**Status:** In Progress
**Priority:** P1
**Owner:** DDx Team

## Overview

DDx operates on files that live in git repositories. Several DDx capabilities
require git awareness beyond "git is the user's problem":

- Protecting spec documents from loss during rapid agent editing
- Tracking document changes for the staleness model
- Enabling MCP/UI clients to commit edits they make to documents
- Providing document history to agents inspecting how specs evolved

DDx is not replacing git. It is adding a thin, deliberate integration layer
that makes git operations available where DDx operations need them — commit
checkpointing, history queries, and safe write-then-commit workflows.

## Problem Statement

**Current situation:**
- Agents edit spec documents rapidly, sometimes overwriting each other's work
  or losing changes when context windows reset
- `ddx doc stamp` records content hashes but has no awareness of when a
  document was last committed vs merely saved
- The MCP server is read-only; a client that wants to edit a document via MCP
  has no way to commit the change
- The web UI (FEAT-008) can't offer "save" functionality without a commit path
- Document history requires manual `git log` — no DDx surface for it

**Desired outcome:** DDx operations that modify tracked files can auto-commit
to protect work. MCP and UI clients can write and commit documents through
DDx. Agents can query document revision history to understand how specs
evolved.

## User Scenarios

### S1: Agent Rapidly Editing Spec Documents (early commit)

An agent is evolving multiple spec documents in a `helix evolve` session. It
updates the PRD, two feature specs, and a solution design. Between each edit,
DDx should checkpoint the change to git so that:

- If the agent crashes or the session ends, no edits are lost
- Each document's change is individually attributable in history
- Concurrent agents on other branches don't conflict with uncommitted state

**Trigger:** Any DDx operation that writes to a tracked document (e.g.,
`ddx doc stamp`, future `ddx doc edit`, MCP write tools) should offer or
default to auto-commit.

**Design constraint:** Early commits should be lightweight (no hooks, fast).
The user or workflow can squash/rebase later. The important thing is that
changes survive.

### S2: Document Graph Tracking Changes Over Time

`ddx doc stale` currently computes staleness from content hashes recorded in
frontmatter stamps. But the interesting question is often "when did this
document last change relative to its dependencies?" — which requires git
history.

**Scenarios:**
- "Show me documents that changed in the last 5 commits" — useful for
  detecting what's in play
- "When was this document last modified?" — git log, not file mtime
- "What changed between the current stamp and the current content?" — diff
  from stamped hash to working copy
- "Which documents changed since tag v0.2.0?" — release-scoped staleness

**Design constraint:** These are read queries against git history. DDx wraps
them with document-graph semantics (artifact IDs, dependency relationships)
rather than raw file paths.

### S3: MCP Client Edits a Document

An agent connected via MCP wants to update a feature spec. Today the server
is read-only. With git awareness:

1. Agent calls `ddx_doc_write` with artifact ID and new content
2. DDx writes the file to disk
3. DDx commits the change with a structured message (e.g., "docs: update
   FEAT-001 via MCP [agent: claude]")
4. The commit is immediately visible to other tools reading the repo

**Design constraint:** Write-then-commit must be atomic from the client's
perspective. If the commit fails, the write should be rolled back (or at
minimum, clearly reported as uncommitted).

### S4: Web UI Saves a Document Edit

A developer editing a document in the FEAT-008 web UI clicks "Save". This
is the same flow as S3 but via HTTP instead of MCP:

1. UI sends `PUT /api/docs/:id` with new content
2. Server writes the file
3. Server commits with attribution
4. UI shows "Saved and committed" or an error

### S5: Agent Inspects Document Evolution

An agent working on a feature wants to understand how the governing spec
evolved — what requirements were added, what changed, why. This is more
than "is it stale?" — it's "what happened?"

**Scenarios:**
- "Show me the last 5 changes to FEAT-001" — git log filtered by file path
- "What changed in FEAT-001 between v0.1.0 and now?" — diff between tags
- "Who last edited this document?" — git blame
- "Show me all documents that changed in this branch" — branch diff

### S6: Workflow Checkpoints Before and After Phases

HELIX (or any workflow) wants to tag a known-good state before entering a
risky phase (e.g., before `helix build` modifies code). DDx can provide:

- `ddx checkpoint "pre-build"` — creates a lightweight git tag
- `ddx checkpoint --restore "pre-build"` — restores to that state

This is a thin wrapper over git tags, but named in DDx terms.

### S7: Bead Tracker Changes Are Committed

Agents create, update, and close beads throughout a work session. The bead
tracker file (`.ddx/beads.jsonl`) is project state that must be committed to
git — it records what work exists, who claimed it, and what's done.

**Current problem:** `.ddx/beads.jsonl` is not gitignored, but nothing in DDx
ensures it gets committed. Agents that don't know about this convention leave
bead changes uncommitted, leading to lost tracker state when sessions end or
branches switch.

**Required behavior:**
- DDx bead-mutating commands (`ddx bead create`, `update`, `close`, etc.)
  should auto-commit `beads.jsonl` after successful mutations (governed by the
  same `git.auto_commit` setting as document changes)
- `ddx install` should auto-commit plugin-related changes (skill symlinks,
  bead tracker updates)
- `ddx init` should generate agent guidance (in CLAUDE.md or AGENTS.md) that
  instructs agents to commit `beads.jsonl` alongside code changes

**Design constraint:** Bead auto-commits should be lightweight. A single bead
mutation produces one commit. Batch operations (e.g., `ddx bead import`)
produce one commit for the whole batch.

### S8: Agent Guidance Generated on Init

`ddx init` should produce agent-facing guidance that ensures agents commit
DDx-managed state files. Without this, agents treat `.ddx/beads.jsonl` and
skill symlinks as untracked noise.

**Minimum guidance:**
- `.ddx/beads.jsonl` should be committed after bead mutations
- `.agents/skills/` and `.claude/skills/` should be committed after
  `ddx install` (already implemented)
- `.ddx/config.yaml` and `.ddx/versions.yaml` should be committed on init
  (already implemented)

The guidance can be injected into CLAUDE.md (extending the existing
metaprompt injection) or generated as a standalone AGENTS.md.

## Requirements

### Functional

**Early commit (S1, S3, S4, S7)**
1. DDx operations that write tracked files can auto-commit the change
2. Auto-commit is configurable: `always` (default for MCP/UI writes),
   `prompt` (ask user), `never` (opt out)
3. Commit messages follow a structured format:
   `docs(<artifact-id>): <description> [ddx: <operation>]`
4. MCP write tools (`ddx_doc_write`) commit by default
5. HTTP write endpoints (`PUT /api/docs/:id`) commit by default

**History queries (S2, S5)**
6. `ddx doc history <id> [--since <ref>]` — show commit log for an artifact
7. `ddx doc diff <id> [<ref1>] [<ref2>]` — show content diff between refs
8. `ddx doc changed [--since <ref>]` — list artifacts changed since a ref
9. MCP tools: `ddx_doc_history`, `ddx_doc_diff`, `ddx_doc_changed`
10. HTTP endpoints: `GET /api/docs/:id/history`, `GET /api/docs/:id/diff`

**Checkpoints (S6)**
11. `ddx checkpoint <name>` — create lightweight git tag with DDx prefix
12. `ddx checkpoint --list` — list DDx checkpoints
13. `ddx checkpoint --restore <name>` — restore working tree to checkpoint

**Bead tracker auto-commit (S7)**
14. `ddx bead` mutating commands auto-commit `.ddx/beads.jsonl` after
    successful operations (governed by `git.auto_commit`)
15. `ddx install` auto-commits plugin artifacts (skill symlinks, tracker
    changes) with `chore: install <name> <version>` message
16. Batch bead operations produce one commit, not one per record

**Agent guidance generation (S8)**
17. `ddx init` injects agent guidance into CLAUDE.md (or generates AGENTS.md)
    that instructs agents to commit DDx-managed state files:
    `.ddx/beads.jsonl`, `.ddx/config.yaml`, `.agents/skills/`, `.claude/skills/`
18. The guidance is part of the metaprompt injection and is updated on
    `ddx init --force`

**Configuration**
19. `git.auto_commit` in `.ddx/config.yaml`: `always`, `prompt`, `never`
20. `git.commit_prefix` for customizing commit message format
21. `git.checkpoint_prefix` for tag naming (default: `ddx/`)

### Non-Functional

- **Safety:** Never force-push, rebase, or delete branches. DDx only creates
  commits, tags, and reads history.
- **Performance:** History queries should use `--follow` for renamed files.
  Commit operations <500ms.
- **Compatibility:** Works with any git repo. No special git configuration
  required.
- **Graceful degradation:** If not in a git repo, history/commit features
  are disabled with clear messages. Core DDx functionality still works.

## Affected Existing Specs

| Spec | Change Needed |
|------|--------------|
| FEAT-002 (Server) | Remove "read-only for v1" constraint. Add write+commit endpoints for documents. |
| FEAT-007 (Doc Graph) | Remove "use git directly" from Out of Scope. Add history/diff/changed commands. |
| PRD | Add git awareness as a primary capability. |

## User Stories

### US-120: Agent Edits Are Protected by Auto-Commit
**As an** AI agent editing spec documents
**I want** DDx to auto-commit each document change
**So that** my edits survive session boundaries and crashes

**Acceptance Criteria:**
- Given auto-commit is enabled, when I write a document via MCP, then a git
  commit is created with the artifact ID in the message
- Given two agents edit different documents, then each edit is a separate
  commit with clear attribution

### US-121: Developer Views Document History
**As a** developer reviewing a spec
**I want** to see how it evolved over recent commits
**So that** I understand what changed and why

**Acceptance Criteria:**
- Given I run `ddx doc history FEAT-001`, then I see a log of commits that
  touched the FEAT-001 file with dates and messages
- Given I run `ddx doc diff FEAT-001 v0.1.0`, then I see the content diff
  since that tag

### US-122: MCP Client Writes and Commits a Document
**As an** MCP-connected agent
**I want** to update a document and have it committed atomically
**So that** my changes are durable and visible to other tools

**Acceptance Criteria:**
- Given I call `ddx_doc_write` with an artifact ID and content, then the file
  is written and committed
- Given the commit fails (e.g., merge conflict), then I receive a clear error
  and the working tree is not left in a dirty state

### US-123: Workflow Creates Checkpoints
**As a** workflow tool (HELIX)
**I want** to tag a known-good state before risky operations
**So that** I can restore if something goes wrong

**Acceptance Criteria:**
- Given I run `ddx checkpoint pre-build`, then a git tag `ddx/pre-build`
  is created
- Given I run `ddx checkpoint --restore pre-build`, then the working tree
  matches the tagged state

### US-124: Bead Changes Are Auto-Committed
**As an** AI agent managing work items
**I want** bead mutations to be committed automatically
**So that** tracker state survives session boundaries and is visible to other
agents and developers

**Acceptance Criteria:**
- Given auto-commit is enabled, when I run `ddx bead create "Fix login"`,
  then `.ddx/beads.jsonl` is committed with a structured message
- Given I run `ddx bead close hx-001`, then the bead state change is committed
- Given I run `ddx bead import`, then one commit is created for the entire
  batch, not one per record
- Given auto-commit is `never`, then bead commands do not create commits

### US-125: Init Generates Agent Guidance for Committing DDx State
**As a** developer setting up a project
**I want** `ddx init` to instruct agents about which files to commit
**So that** agents don't leave DDx state files uncommitted

**Acceptance Criteria:**
- Given I run `ddx init`, then CLAUDE.md contains guidance listing
  `.ddx/beads.jsonl`, `.ddx/config.yaml`, `.agents/skills/`, and
  `.claude/skills/` as files that should be committed
- Given I run `ddx init --force`, then the guidance is refreshed
- Given an agent reads the generated guidance, then it knows to `git add`
  and commit `.ddx/beads.jsonl` after bead operations

## Dependencies

- `internal/git` package (existing — basic git operations)
- FEAT-004 (beads — bead mutation operations)
- FEAT-007 (document graph — artifact ID to file path mapping)
- FEAT-002 (server — write endpoints)
- FEAT-009 (plugin registry — `ddx init` and `ddx install`)

## Out of Scope

- Branch management (creating, switching, merging branches)
- Remote operations (push, pull, fetch)
- Merge conflict resolution (report conflicts, don't resolve them)
- Git configuration management
- Submodule or subtree operations (handled by existing `ddx update/contribute`)
