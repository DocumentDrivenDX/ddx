---
name: ddx
description: Operates the DDx toolkit for document-driven development. Covers beads (work items), the queue, executions, agents, harnesses, personas, reviews, spec-id. Use when the user says "do work", "drain the queue", "run the next bead", "execute a bead", "review this", "check against spec", "what's on the queue", "what's ready", "create a bead", "file this as work", "run an agent", "dispatch", "use a persona", "how am I doing", "ddx doctor", or mentions any ddx CLI command.
---

# DDx

DDx (Document-Driven Development eXperience) is a CLI platform for
document-driven development. It ships a bead tracker (portable work
items with acceptance criteria), an agent service (harness dispatch
with profile-first routing), a persona system (bindable AI
personalities), a library registry (plugins with prompts, templates,
personas), and git-aware synchronization. This skill makes any
skills-compatible coding agent (Claude Code, OpenAI Codex, Gemini
CLI, etc.) understand and operate the DDx surface correctly.

## How this skill works

The skill body you're reading is an **overview** plus an **intent
router**. The real domain guidance lives in `reference/*.md` files.

**Directive: before responding to any DDx-related request, read the
matching reference file from the router table below. The router is
not optional — your answer must be grounded in the reference file's
guidance, not this overview alone.**

## Vocabulary

Single source of truth for DDx terms. Every reference file uses these
exact definitions.

- **Bead** — a portable work item (task, bug, epic, chore) with
  metadata, dependencies, and acceptance criteria. `ddx bead create`.
- **Queue** — the set of open beads. The *ready queue* is the subset
  with all dependencies closed. `ddx bead ready`, `ddx bead blocked`.
- **Ready** — a bead whose dependencies are all closed and which is
  eligible to be picked up next. `ddx bead ready`.
- **Blocked** — a bead with at least one unclosed dependency.
  `ddx bead blocked`.
- **Claim** — mark a bead as in-progress by an agent (concurrent-write
  protection). `ddx bead update <id> --claim`.
- **Close** — mark a bead as done, with evidence (session, commit
  SHA). `ddx bead close <id>`. Beads only close on execution outcomes
  `success` or `already_satisfied`.
- **Work** — drain the bead queue. `ddx work` (alias for
  `ddx agent execute-loop`); see `reference/work.md`.
- **Execute-bead** — the primitive: run one agent on one bead in an
  isolated worktree. `ddx agent execute-bead <id>`. The loop wraps
  this.
- **Execute-loop** — the queue-drain orchestrator; picks ready beads
  one at a time, calls execute-bead, closes on success, unclaims on
  failure. `ddx agent execute-loop` (or `ddx work`).
- **Execution** — a generic DDx execution run (FEAT-010); broader
  than execute-bead. Includes execution definitions, execution
  records, and execution evidence under `.ddx/executions/<id>/`.
- **Agent** — an AI coding agent (Claude, Codex, Gemini, etc.)
  invoked via a harness. Not a subagent (harness-specific — see
  below).
- **Harness** — the abstraction DDx uses to dispatch an agent
  (subprocess for claude/codex/gemini; embedded for the built-in
  `agent` harness). `ddx agent list`, `ddx agent capabilities`.
- **Persona** — a Markdown file (YAML frontmatter + body) that
  defines an AI personality. DDx injects the body as a system-prompt
  addendum to `ddx agent run`. `ddx persona list/show/bind`.
- **Role** — an abstract function (e.g., `code-reviewer`,
  `test-engineer`) a workflow can reference. Projects bind roles to
  personas.
- **Binding** — a project-specific `role: persona` map in
  `.ddx/config.yaml` under `persona_bindings`.
- **Profile** — intent-based routing policy for `ddx agent run`:
  `cheap` (low-cost), `fast` (low-latency), `smart` (balanced
  default). `--profile cheap|fast|smart`.
- **Plugin** — a self-contained extension installed to
  `.ddx/plugins/<name>/`. The default `ddx` plugin (personas,
  prompts, patterns, templates) is auto-installed by `ddx init`.
  `ddx install <name>`.
- **Skill** — an agentskills.io-standard directory (SKILL.md +
  optional `reference/`, `evals/`, `scripts/`). This `ddx` skill is
  the one DDx ships. Plugins can ship additional skills.
- **Subagent** — a harness-local concept for running a prompt in an
  isolated context (Claude Code's `.claude/agents/` + `context:
  fork`; Codex's `agents/`; others differ). DDx's `ddx agent run`
  (especially with `--quorum`) is the portable primitive; how a
  harness wraps that in a subagent is harness business. This skill
  does not take a position on subagent orchestration.
- **Update** — refresh plugin/toolkit content to a newer version.
  `ddx update [<plugin>]`.
- **Upgrade** — replace the DDx binary with a newer release.
  `ddx upgrade`.
- **Review** — two distinct concepts. **Bead review**
  (`ddx bead review <id>`) grades a completed bead against its
  acceptance criteria. **Quorum review** (`ddx agent run
  --quorum=<policy> --harnesses=<list>`) dispatches a review prompt
  across multiple harnesses and aggregates. See `reference/review.md`.
- **Governing artifact** — the document that authorizes a bead's
  work: a FEAT-\*, SD-\*, TD-\*, or ADR-\* under `docs/`. Referenced
  via `spec-id`.
- **Spec-id** — the `spec-id: <ID>` custom field on a bead pointing
  at its governing artifact.

## Intent router

Before responding, read the matching file.

| User says / asks about | Read this file |
|---|---|
| write/plan work, "create a bead", "file this as work", bead metadata, acceptance criteria, dependencies | `reference/beads.md` |
| "do work", "drain the queue", "run the next bead", "execute a bead", "run work", verify-and-close | `reference/work.md` |
| "review this", "check against spec", bead review, quorum review, code review, adversarial check | `reference/review.md` |
| "run an agent", "dispatch", harnesses, profiles, models, effort, "use a persona", role bindings | `reference/agents.md` |
| "what's on the queue", "what's ready", "how am I doing", health check, "ddx doctor", sync status | `reference/status.md` |

If the intent spans multiple files (e.g., "create a bead and then
run it"), read beads.md first, then work.md. If no match, ask the
user which concept they mean rather than guessing.

## Top-level policy reminders

These apply across all DDx operations. Do not restate them in every
reference file; do not violate them.

- **Never edit `.ddx/beads.jsonl` directly.** All tracker changes go
  through `ddx bead create/update/close/dep`. Direct edits corrupt
  bead history and cannot be audited.
- **Tracker changes are commit-worthy.** After `ddx bead create`,
  `update`, `dep add/remove`, or `close`, commit the resulting
  `.ddx/beads.jsonl` change — either as a tracker-only commit or
  folded into the same commit as related implementation changes.
- **Preserve execute-bead commit history.** Branches containing
  `ddx agent execute-bead` commits carry an audit trail. **Never
  squash, rebase, filter, or amend** these commits. Use only
  `git merge --ff-only` or `git merge --no-ff` when merging.
  `gh pr merge --squash` and `--rebase` are forbidden on these
  branches.
- **Work in worktrees for parallel agents.** Use `wt switch -c
  <branch>` (worktrunk) or equivalent to give each concurrent agent
  its own isolated checkout. Execute-bead does this automatically;
  manual parallel work should too.
- **Profile-first agent dispatch.** Default to
  `ddx agent run --profile smart` (or `cheap`/`fast` for specific
  intents). Only override with `--harness`/`--model`/`--effort`
  when pinning for a controlled test or known provider bug.

## Links out

- Full CLI reference: `ddx --help`, `ddx <subcommand> --help`.
- Governing feature specs: see `FEAT-*` documents under your
  project's `docs/` tree — especially the CLI, beads, agent-service,
  executions, and skills features.
- Personas README: shipped by the default `ddx` plugin at
  `.ddx/plugins/ddx/personas/README.md`.
- Open standard this skill conforms to:
  [agentskills.io](https://agentskills.io).
