---
ddx:
  id: RSCH-009
  status: draft
  depends_on:
    - REF-026
    - REF-011
    - REF-012
    - REF-010
id: RSCH-009
title: "Least Privilege for Agents"
kind: research-synthesis
summary: "Agents need narrow, explicit, revocable authority — bounded file scope, scoped tool permissions, and human-confirmed elevation — because the failure modes are now known, exploited, and CVE-cataloged."
tags: [security, agents, least-privilege, ddx-principle]
---

# Least Privilege for Agents

## Principle

An agent gets the smallest set of files, tools, and capabilities required
to do the current job, scoped to a specific time and location, with
elevation requiring human confirmation. The agent is treated as an
untrusted process that happens to be useful, not as a trusted teammate.

## Synthesis

Saltzer and Schroeder (REF-026) enumerated the foundational principles
in 1975: least privilege, fail-safe defaults, complete mediation,
separation of privilege. The agent case is not different in kind from
the multi-user OS case — what changed is that the "process" can now be
persuaded in natural language, by attacker-controlled input, to misuse
whatever authority it holds.

OWASP's 2025 LLM Top 10 (REF-011) catalogs the threat model: prompt
injection, insecure output handling, supply-chain risk, and — most
relevantly — *excessive agency*. Excessive agency is what happens when
an agent has more tools, permissions, or write access than the current
task requires; the agent itself does not need to be malicious for the
excess to be exploited. EchoLeak (REF-012, CVE-2025-32711) is the
canonical real-world demonstration: a zero-click prompt injection in
Microsoft 365 Copilot exfiltrated user data via attacker-crafted email
content. The agent had authority to read user mail and to render
arbitrary content; the combination was the vulnerability.

Sheridan and Verplank's levels-of-automation framework (REF-010) is the
human-side counterpart: at every level above "fully manual," there must
be a designed point at which the human confirms or overrides. The 1978
paper enumerated ten levels precisely so designers would have to
*choose* rather than slide unconsciously into full autonomy. For
agentic coding tools the choice is the same: which actions are
auto-approved, which require confirmation, which are forbidden?

The four references compose: REF-026 supplies the principle, REF-011
the modern threat catalog, REF-012 a worked example, REF-010 the
human-confirmation discipline. The conclusion is operational: scope
agent authority by default, audit continuously, require a human at
every elevation boundary.

## DDx Implication

DDx executes agents in isolated git worktrees — `execute-bead` runs in a
named worktree under `.execute-bead-wt-*`, scoping every file edit to
that branch and that bead. The agent cannot reach across worktrees,
mutate the bare repo directly, or touch unrelated branches. Tool
permissions are scoped per harness, with the executing process inheriting
only the credentials the harness declared. Persona definitions encode
role-bounded behavior. Destructive or shared-state actions (push, branch
deletion, force operations) sit behind explicit human-in-the-loop
confirmation, in line with REF-010's levels-of-automation discipline.
The architectural stance is that the agent's blast radius must be
designed, not assumed.
