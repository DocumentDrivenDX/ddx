---
ddx:
  id: plan-2026-05-10-databricks-deployment
---
# DDx in Databricks: Deployment Shape & Constraints

Date: 2026-05-10
Status: Drafted; user has confirmed "not Databricks-first" but considering Option 2 (binary-wheel + scope) or Option 3 (server-as-Databricks-App). Foundational work (storage abstractions) is in flight via FEAT-028; deployment-shape decision deferred.

## Question being answered

> Can a document-driven workflow live inside a Databricks workspace, and if so, what does it look like?

Not "ship ddx to Databricks" — the user has been explicit this is exploratory. The honest answer is: **yes, but only the consumption half — and the answer reshapes the Python package, not Databricks.**

## What works in a Databricks workspace

- **Reading library + bead state**: `ddx bead list/show`, `ddx prompts/persona/templates`, `ddx jq`. Stateless reads.
- **Filing beads**: `ddx bead create`, `ddx bead update`. Local writes fine; pushing to a remote needs git auth.
- **Binary distribution**: pip-installed Linux wheel runs fine on a cluster driver; `%pip install ddx` in a notebook works; binary lands on `/local_disk0/...`.

## What doesn't work, or only barely

- **Legacy sync / worktrees**: legacy sync paths are being removed (`ddx-a827d146`); `legacy agent execute-bead` still relies on real git semantics (worktrees, push). Databricks Repos route commits through Databricks' git proxy, not raw git. Worktrees on DBFS/Volumes mounts are broken-to-flaky (FUSE).
- **Agent harnesses**: `claude`, `codex`, `opencode` CLIs need OAuth + token storage + egress to provider APIs. Production workspaces commonly restrict egress; auth flows that pop a browser don't work in notebooks.
- **`work`**: long-running drain doesn't fit the notebook lifecycle. Packaging as a Databricks Job is possible but the job model (retries, parameters, idempotency) is awkward for a continuously-draining queue.
- **Lefthook / pre-commit gates**: irrelevant in a Databricks Repo (commits don't go through local git).
- **Persistence**: workspace files have size/count limits; DBFS/Volumes lack POSIX semantics ddx assumes (locks, atomic rename); `/local_disk0/` is ephemeral. No good place for a long-lived `.ddx/`.

## The architectural fork

Three options on the table:

### Option 1 — Databricks-first
Pivot to building `ddx-server` and making the Python package a thin client. Pip wheel becomes pure-Python (no platform matrix). Local CLI continues for non-Databricks users but is no longer the primary surface. **User has rejected this** — not Databricks-first.

### Option 2 — Databricks-secondary
Ship the binary-bundled wheel as planned, scope the Databricks story to "read + file beads against a `.ddx/` cloned into local_disk0," document the cliff. Users who want execution run ddx on a workstation or in CI.

### Option 3 — Databricks as one supported deployment among several
Build `ddx-server` as a portable container; ship it both as a workstation/CI service AND as a Databricks App. The App deployment uses **Lakebase Postgres** (via Axon) for entity state, **Unity Catalog Volumes** for blob storage, and dispatches execution to **Databricks Jobs**. Notebook Python wrapper is a pure-Python HTTP/MCP client.

## Option 3 deployment shape

```
┌─ Databricks Workspace ─────────────────────────────────────────┐
│                                                                │
│  Notebook ──HTTPS+OAuth──> ddx-server (App container)          │
│                                  │                             │
│                                  ├──> Lakebase: entity state   │
│                                  │     (via Axon)              │
│                                  ├──> UC Volumes: library +    │
│                                  │     attachments + evidence  │
│                                  └──> Databricks Jobs: dispatch│
│                                       execute-bead attempts    │
│                                                                │
│  Job worker ──> agent harness (LLM via Model Serving           │
│                  or direct API) ──> result back to server      │
└────────────────────────────────────────────────────────────────┘

External: source repo (workstation/CI runs ddx against the same server)
```

What Databricks Apps actually offer:
- **Workspace API** for notebook/file objects — API-latency, rate limits.
- **Unity Catalog Volumes** — proper governed blob storage; best fit for ddx library + attachments + evidence.
- **Lakebase Postgres** — UC-governed transactional Postgres; best fit for the entity store. Axon already runs on Postgres.
- **OAuth identity passthrough** — no separate auth model to design.

## Cost / risk if Option 3

Real engineering, not free:
1. New `BackendLakebase`-shaped work (or proving Axon-on-Lakebase end-to-end — see axon backend audit, plan-2026-05-10-axon-only-architecture.md). Audit verdict was **not viable today**.
2. UC Volumes adapter for blob/attachment storage. Cleanly slots into FEAT-028's `BlobStore` interface (the abstraction now exists).
3. ddx-server itself ("planned" in CLAUDE.md, not built).
4. The App packaging (Dockerfile, `app.yaml`, secrets wiring).
5. Worker model decision: Databricks Jobs-native dispatch (~weeks of real work) vs. external pollers (~days, the hybrid). The "execution out-of-process" assumption is currently aspirational — today's server runs `agent.ExecuteBeadWithConfig` in-process at `cli/internal/server/workers.go:748,783` (per axon audit).

## Open questions

- Customer model: are we deploying ddx-server-as-App for *our* workspace, or shipping it for *any customer* to deploy in *their* Databricks? Big difference for whether Apps deployment matters and what prerequisites we can assume.
- Worker model: Jobs-native or external pollers? Jobs-native is the right Databricks answer but a real chunk of work AND blocks on resolving in-process execution.
- Lakebase region availability: not everywhere; check the workspace's region before designing around it.
- FEAT-009 library registry: if library content lives in UC Volumes inside the workspace, the registry-fetch model needs a Volumes-reading backend (not github.com). Another adapter to slot in.

## Recommended direction

User has signaled Option 2 OR Option 3, leaning toward Option 3 framing ("can a document-driven workflow live in Databricks"). Foundational work that's safe under either option:

1. **FEAT-028 storage abstractions** — defines `BlobStore` and the broader 5-abstraction taxonomy. Same abstractions support local-FS (Option 2) AND UC Volumes (Option 3). Already drafted, post-review fixes applied, ready for bead breakdown.
2. **Validate Axon production-readiness** — opus audit returned a "not viable today" verdict (see plan-2026-05-10-axon-only-architecture.md). Multiple prerequisites missing; would need to land before any Postgres/Lakebase deployment.
3. **Defer App-specific work** until storage abstractions land and Axon is production-viable.

The right shape: ddx-server ships with **two storage adapters from day one** (local-FS + sqlite, and Lakebase + UC Volumes), selectable via config. App deployment is one of two supported deployments. Forces the abstractions to actually be abstractions, not Databricks-shaped APIs with a "local mode" bolted on after.
