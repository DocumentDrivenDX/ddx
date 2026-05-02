---
ddx:
  id: ADR-021
  depends_on:
    - FEAT-006
    - FEAT-008
    - FEAT-004
    - ADR-006
    - ADR-007
---
# ADR-021: Operator-Prompt Beads as the Web Write Path

**Status:** Accepted
**Date:** 2026-05-02
**Authors:** Story 15 planning (`/tmp/story-15-final.md`)

## Context

FEAT-008 (web UI) and FEAT-006 (agent service) need a write path that lets a
trusted operator type a free-form natural-language prompt on the project home
page and have the system *do work* — mutate the bead queue, write artifacts,
or run a multi-step change — without dropping to the CLI.

Today the server's existing write surface is structured:

- GraphQL mutations: `beadCreate/Update/Claim/Unclaim/Reopen/Close`,
  `documentWrite`, `workerDispatch`, `startWorker/stopWorker`,
  `pluginDispatch`, `comparisonDispatch`
  (`cli/internal/server/graphql/schema.graphql`).
- REST: `/api/agent/run` (inline prompt → harness, FEAT-022 D2),
  worker exec-loop, plugin dispatch, doc write, bead create/update.
- Auth: `requireTrusted` wraps every mutation — localhost OR a ts-net
  `WhoIs`-authenticated peer (`cli/internal/server/server.go`).
- Inline-prompt ingress already hard-fails oversize at
  `serverPromptCapBytes` (Stage D2); no oversize-bypass option exists.

A free-form prompt does not fit any of those structured mutations. The
question is *how* it gets executed and *what trust contract* applies — not
whether the feature ships.

This ADR exists because adding a free-form natural-language ingress to a
write surface is a security-boundary decision. Future contributors must not
regress it (e.g. by adding a "fast path" that runs an operator prompt
synchronously inside the GraphQL resolver, bypassing the worktree-and-land
safeguards every other write goes through).

## Decision

**Every operator prompt submitted from the web UI becomes a bead with
`issueType: operator-prompt`, drained by the existing execute-loop.**

There is no synchronous in-process mutation path for prompts. The
`operatorPromptSubmit` GraphQL mutation persists a bead and signals the
project's execute-loop coordinator; the loop claims the bead, runs the
harness in a fresh worktree, and lands changes through the standard
land-coordinator pipeline.

### Why enqueue-as-bead

- **Reuses every safeguard already shipped.** Worktree isolation,
  land-coordinator, post-land build gate, large-deletion rejection,
  preserve-on-failure, evidence ledger, structural AC checks (where
  applicable), cost cap. None of these need to be reinvented for a
  prompt-driven path.
- **Audit trail is automatic.** The bead *is* the operator's instruction;
  attempts, evidence, and events are already tracked. No new audit table.
- **Multi-node story (Story 14, ADR-007) gets the right answer for free.**
  The prompt routes to whichever node owns the project's queue, identical
  to any other bead. No new authorization plane.
- **Failure modes are familiar.** Operators see preserved branches and
  review labels — the same failure surface they already understand —
  instead of a bespoke prompt-error UI.
- **Approval flow is just bead status.** `proposed → ready` is a deliberate
  transition gated to trusted requests; no parallel approval state machine.

### Rejected alternative: synchronous in-process `service.Execute`

The agent service is already reachable in-process (`service.Execute`), so a
GraphQL resolver could in principle invoke a harness directly and return the
result inline. This was rejected:

- It bypasses worktree/land safety. Any mutation a prompt produces would
  apply directly to the working tree without the per-attempt isolation that
  catches partial commits, large deletions, and failed build gates.
- It doubles the execution path. Bead-driven and prompt-driven mutations
  would diverge in subtle ways (event shape, evidence layout, cost
  attribution), and every invariant we add later would have to be enforced
  twice.
- It requires a parallel audit table. Beads already record who, when, what,
  and why. A synchronous path would need bespoke audit logging that
  operators cannot inspect with the tools they already have.

Latency is the only legitimate complaint against enqueue-as-bead, and it is
mitigated below.

## Trust requirement: `requireTrusted` plus prompt-specific controls

`operatorPromptSubmit` and any companion mutations (`operatorPromptApprove`,
`operatorPromptCancel`) MUST go through `requireTrusted` —
localhost or ts-net `WhoIs`-authenticated peer per ADR-006.

`requireTrusted` authenticates the *network peer*, not user *intent*.
Prompt-driven mutation endpoints add the following on top of
`requireTrusted`:

- **CSRF / Origin defense.** Strict `Origin`/`Host` validation. Reject
  browser cross-origin POSTs even from localhost. Require a per-session
  CSRF token issued by the served HTML.
- **Idempotency key.** `operatorPromptSubmit` accepts a client-generated
  UUID; the server dedupes. Prevents duplicate destructive work on retries.
- **Per-project authorization.** Tailnet identity is not authorization. Add
  `web.operator_prompt.allow_identities`, a project-scoped ts-net identity
  allowlist. Default empty → localhost-only writes; ts-net peers see
  read-only UI until added.
- **Identity-bound audit.** Submitter identity is recorded as a structured
  field on the immutable first bead event (not just labels), including:
  peer identity (loopback or ts-net WhoIs), origin node ID, build SHA,
  config-approval-mode at submit time, request ID, and prompt SHA-256.
- **UI escaping.** Operator prompts and assistant outputs render through
  strict escaping; XSS coverage in the Playwright suite. Pasted-content
  prompt-injection is mitigated by a visible "this is what we will send"
  preview before approval.
- **Oversize.** Prompts above `serverPromptCapBytes` return 400 with the
  existing cap-error envelope. No bypass.

## Audit-as-bead

The bead IS the audit record. There is no parallel audit table for
operator-prompt submissions.

- **Title** is the first line of the prompt.
- **Body** is the full prompt verbatim.
- **Default labels** include `kind:operator-prompt`, `source:web-ui`.
- **Default tier** is the project's policy tier.
- **Acceptance section** is auto-generated from a template (e.g.
  "agent must produce a diff or evidence rationale; no AC = AC is the
  prompt verbatim"). The structural AC check is skipped for this
  `issueType` — the structural check assumes pre-authored AC.
- **First event** records the identity-bound audit fields enumerated above.
- **Backlinks.** Every bead created/changed and every artifact mutated by
  an operator-prompt execution gets an `origin_operator_prompt_id` event
  so provenance survives later edits, rebases, and history rewrites.

## Multi-node delegation policy

Per ADR-007 (federation topology), prompts run on the client node that owns
the project's queue, not on the coordinator. The submission flow:

1. The operator's browser submits to whichever node it is connected to.
2. If that node owns the project's queue, it persists the bead locally and
   wakes the local execute-loop.
3. If the receiving node is a coordinator that does not own the queue, it
   forwards the submission to the owning client node, **carrying the
   originating trust attestation unchanged** (peer identity, request ID,
   prompt SHA-256, CSRF result).
4. The owning node verifies the originating identity against its own
   `web.operator_prompt.allow_identities` allowlist before persisting the
   bead.

The trust model is uniform across read and write: "localhost OR ts-net
`WhoIs` identity." Any future coordinator-relay path MUST NOT lose the
originating attestation. Coordinator-relay without attestation forwarding
is a regression of this ADR.

## Prompt-injection threat model

Prompts arriving via `operatorPromptSubmit` are untrusted *content* from a
trusted *peer*. The threats and mitigations:

- **Hostile prompt content (injection, jailbreak, prompt smuggling).**
  Mitigation: `requireTrusted` plus the per-project allowlist limit the
  attack surface to peers we already trust to run beads. No additional
  content sanitation is performed — the prompt becomes a bead description
  like any other, and the harness, worktree isolation, and land-coordinator
  safeguards apply unchanged.
- **Pasted hostile content masquerading as instructions.** Mitigation: the
  UI shows a "this is what we will send" preview before approval, so the
  operator has the chance to notice unexpected content before queuing work.
- **Prompts that try to exfiltrate or escalate by mutating the queue.**
  Mitigation: see *Allowed-mutation scope* below — operator-prompt beads
  cannot mutate other operator-prompt beads, and all mutations go through
  the same tooling a regular bead has.
- **Prompts that try to mass-delete or corrupt artifacts.** Mitigation: the
  land-coordinator's existing large-deletion rejection and post-land build
  gate apply unchanged.
- **XSS via rendered prompt or assistant output.** Mitigation: strict
  escaping in the SvelteKit components rendering prompt text and assistant
  evidence; explicit Playwright XSS test in the Story-15 suite.
- **CSRF from a malicious page on the same machine.** Mitigation: strict
  `Origin`/`Host` validation and per-session CSRF token issued by the
  served HTML.
- **DNS rebinding (attacker page resolves to 127.0.0.1 to reach the
  loopback write surface).** Mitigation: the same strict `Host` allowlist
  rejects requests whose `Host` header is not an exact match for the
  configured server bind name (e.g. `localhost`, `127.0.0.1`, or the
  ts-net hostname). Combined with the CSRF token requirement (which the
  attacker page cannot read across origins) and the per-session
  same-origin Origin check, a rebound DNS name cannot turn a read-only
  visit into a write. Documented explicitly so future contributors do not
  relax `Host` validation thinking CSRF alone is sufficient.
- **Replay / duplicate destructive work.** Mitigation: idempotency key
  dedupe.

## Allowed-mutation scope

An operator-prompt bead's execution may do **anything a normal bead's
execution can do** — the full agent toolkit (bead CRUD, document write,
worker dispatch within tier policy, etc.). There is no privilege
escalation: the agent runs with the same harness, profile, tier, and tool
permissions that it would for any bead the project owns.

One hard exclusion:

- **Operator-prompts cannot mutate operator-prompts.** An `operator-prompt`
  bead's execution may not create, edit, claim, close, or otherwise alter
  another `operator-prompt` bead. This prevents queue-mutation loops
  ("close all operator prompts", "approve every pending operator prompt")
  and the obvious self-reinforcing-injection failure mode. The bead store
  enforces this at write time, not at planning time, so the agent cannot
  argue its way around it.

This scope is deliberately narrow:

- The agent already has rich capabilities for a reason — restricting them
  for prompt-driven beads would make the feature less useful than the CLI
  (`ddx try`, `ddx work`).
- The trust boundary is the *peer identity at submission time*, not the
  *capability set at execution time*. Tightening capabilities below what
  the same peer could do via CLI would be theatre.
- The bead-vs-CLI symmetry keeps reasoning simple: "if you can run it as
  a bead from the terminal, you can run it as a prompt from the web."

## Approval flow

Default ON for ts-net peers, OFF (auto-approve) for localhost.

- Submitted beads land in `proposed` status.
- The UI shows them with an "Approve & queue" button that transitions to
  `ready`.
- `web.operator_prompt.auto_approve` flips to skip-for-trusted-localhost.
- Auto-approve fires only for configured localhost identities, NEVER for
  ts-net peers by default. A ts-net peer must be explicitly listed in both
  the per-project allowlist and an auto-approve allowlist to skip review.

## Latency mitigation

Operators may wait seconds-to-minutes between submit and the loop claiming
the bead. Mitigations:

- The `operatorPromptSubmit` resolver immediately wakes the local
  execute-loop coordinator after persisting the bead.
- The chat pane streams the resulting attempt's events (same
  `workers.recentEvents` shape used elsewhere in FEAT-008).
- The UI shows a tail of recent operator-prompt beads with live status,
  so operators see progress without polling.

## Consequences

- **One write path, not two.** No synchronous in-process resolver path will
  be added later "for speed." If latency is unacceptable, the fix is to
  speed up the loop, not to bypass it.
- **All bead invariants apply.** Cost caps, evidence ledger, preserve-on-
  failure, large-deletion rejection — every safeguard the queue has,
  prompts inherit. Adding a new safeguard to beads automatically protects
  prompts.
- **No bespoke audit table.** Audit goes through the bead event log; tools
  that read bead events (CLI, MCP, web UI) automatically see operator-
  prompt history.
- **Multi-node coupling.** Story-14 implementations MUST forward the
  originating trust attestation across coordinator relay. This ADR makes
  that a hard requirement, not a nice-to-have.
- **Weaker AC than authored beads.** Operator-prompt beads skip the
  structural AC check because the AC template is auto-generated. Review
  tier is expected to catch garbage. This is documented as a known
  tradeoff, not hidden.
- **Ts-net peers are read-only by default.** They must be added to a
  per-project allowlist before any prompt they submit will be persisted.
  Localhost remains the path of least resistance, matching ADR-006.

## References

- `/tmp/story-15-final.md` — Story 15 plan (diagnosis, alternatives,
  acceptance criteria, codex-review security controls)
- ADR-006 — ts-net authentication (the underlying trust model)
- ADR-007 — federation topology (multi-node delegation context)
- ADR-004 — bead-backed runtime storage (the audit substrate)
- FEAT-006 — agent service (harness invocation path)
- FEAT-008 — web UI (the surface that submits prompts)
- FEAT-004 — beads (the queue that drains operator-prompt beads)
- Closed epic `ddx-88e91fdb` — chat interface to project home page
  (origin of the broader feature exploration)
