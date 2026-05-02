---
title: Least Privilege for Agents
weight: 9
---

# Least privilege for agents

> An agent is treated as an untrusted process that happens to be useful,
> not as a trusted teammate.

## The principle

Agents are powerful, fallible, and fast — a combination that punishes
permissive defaults. An agent with shell access, network access, and
write access to the entire repo can do enormous damage in seconds, and
the damage may not be obvious until much later. Worse, the agent itself
does not need to be malicious for the damage to occur — *excessive
agency* is exploitable through prompt injection in attacker-controlled
input long before the agent's own judgment becomes the issue.

Least privilege treats agent permissions the way a security-conscious
system treats user permissions: grant the minimum needed for the task,
scope it to the artifact at hand, and audit what was actually used.
For DDx, this resolves into two concrete surfaces: **file scope** and
**tool permissions**.

**File scope.** Each execute-bead run takes place in an isolated git
worktree, named after the bead and timestamp, branched from a known
base revision. The agent edits files inside that worktree only — it
cannot reach across worktrees, mutate the bare repo directly, or touch
unrelated branches. Merge to base is gated, with `--no-ff` history
preserving which agent did which work. The blast radius of any single
attempt is the diff in one worktree, recoverable by deleting the
branch.

**Tool permissions.** Harnesses declare which tools the agent may
call; allowlists in `settings.json` gate Bash and MCP invocations;
destructive or shared-state actions (push, branch deletion, force
operations, network calls outside the harness's declared surface)
sit behind explicit human-in-the-loop confirmation. Personas encode
role-bounded behavior so that the *intent* of the agent's tool use
is also scoped, not just the syntactic surface.

The goal is not to make agents weak. It is to make their blast radius
proportional to the trust we have actually earned with them, and to
require a designed elevation point — not a slide into full autonomy —
when more authority is needed.

## Evidence

- **REF-026 — Saltzer & Schroeder (1975).** Foundational principles:
  least privilege, fail-safe defaults, complete mediation, separation
  of privilege. The agent case is not different in kind from the
  multi-user OS case; what changed is that the "process" can now be
  persuaded in natural language.
- **REF-011 — OWASP LLM Top 10 (2025).** Catalogs the threat model:
  prompt injection, insecure output handling, supply-chain risk, and
  *excessive agency* — what happens when an agent has more tools or
  permissions than the current task requires.
- **REF-012 — EchoLeak (CVE-2025-32711).** Real-world demonstration:
  a zero-click prompt injection in Microsoft 365 Copilot exfiltrated
  user data via attacker-crafted email content. The agent did not need
  to be malicious; the over-broad permission combination was the
  vulnerability.
- **REF-010 — Sheridan & Verplank, levels of automation (1978).**
  Ten levels enumerated precisely so designers must *choose* a
  human-confirmation boundary rather than slide unconsciously into
  full autonomy.
- **RSCH-009.** REF-026 supplies the principle, REF-011 the modern
  threat catalog, REF-012 the worked example, REF-010 the
  human-confirmation discipline.

See `docs/helix/00-discover/research/RSCH-009-least-privilege-for-agents.md`.

## DDx response

- **Isolated git worktrees** for `execute-bead` runs (under
  `.execute-bead-wt-*`) scope every file edit to the bead's branch.
  The agent cannot mutate the bare repo or other worktrees.
- **Per-harness tool declarations** restrict which tools each agent
  can call; the executing process inherits only the credentials the
  harness declared.
- **`settings.json` allowlists** gate Bash and MCP invocations at the
  Claude Code harness layer — the harness, not the agent, decides
  what is auto-approved.
- **Persona definitions** encode role-bounded behavior so the agent's
  intent is also scoped, not just its syntactic capabilities.
- **Merge gates and human-in-the-loop confirmation** for destructive
  or shared-state actions (push, force operations, branch deletion),
  in line with REF-010's levels-of-automation discipline.
