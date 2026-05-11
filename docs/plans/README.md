# Plans

Architectural and product plans captured in this directory are higher-level
explorations than the design plans under `docs/helix/02-design/`. Each plan
captures a specific question, the discussion / multi-model review around it,
and the decision (or open status). Filenames follow `plan-YYYY-MM-DD-name.md`.

These are review artifacts, not specs. When a plan converges to a concrete
deliverable, it gets translated into a FEAT-* spec under
`docs/helix/01-frame/features/` (if the work is feature-scoped) or a
`docs/helix/02-design/plan-*` design doc (if it's an implementation plan
for an existing feature).

## Index

| Date | Plan | Status |
|------|------|--------|
| 2026-05-10 | [Pip distribution + auto-generated Python API](plan-2026-05-10-pip-distribution-and-python-api.md) | Drafted; held pending Databricks-deployment decision |
| 2026-05-10 | [DDx in Databricks: deployment shape & constraints](plan-2026-05-10-databricks-deployment.md) | Exploratory; user has rejected Databricks-first; deciding between Option 2 and Option 3 |
| 2026-05-10 | [Storage abstractions: taxonomy + BlobStore-first sequencing](plan-2026-05-10-storage-abstractions.md) | Drafted as FEAT-028; ready for bead breakdown pending user approval |
| 2026-05-10 | [Axon as sole backend / GraphQL collapse](plan-2026-05-10-axon-only-architecture.md) | **Rejected** — keep ddx-server GraphQL + BlobStore separate |
| 2026-05-10 | [MCP architecture: DDx-server-MCP into Databricks Assistant](plan-2026-05-10-mcp-architecture.md) | Verdict reached: ddx-server-MCP only; gap list captured |
| 2026-05-10 | [Read-only deployment: minimum storage surface](plan-2026-05-10-read-only-deployment.md) | Sequencing identified: BlobStore → Axon EntityStore → `attempts` collection → ConfigStore |
| 2026-05-11 | [Website evolution: design system, auto-gen, Hugo renderer, Pages](plan-2026-05-11-website-evolution.md) | Reviewed — 4 decisions locked; bead breakdown via companion plan |
| 2026-05-11 | [Website auto-generation + templating architecture](plan-2026-05-11-website-autogen.md) | Draft v2 — restructured around unified generator + artifact graph after self+opus review |
| 2026-05-11 | [FEAT-005 amendment: `ddx.visibility`](plan-2026-05-11-artifact-visibility.md) | Draft — prerequisite for website autogen Phase C |
| 2026-05-11 | [`ddx __introspect` primitive](plan-2026-05-11-ddx-introspect.md) | Draft — prerequisite for website CLI generator AND pip Python codegen |
| 2026-05-11 | ~~Bead backend interface refinement~~ → folded into TD-027 | **Superseded** by TD-027 (`docs/helix/02-design/technical-designs/TD-027-bead-collection-abstraction.md`). The plan doc retains a redirect note pointing at TD-027 sections. Gating bead `ddx-c6317784` references TD-027 as canonical AC. |
| 2026-05-11 | ~~Bead interface refactor integration plan~~ → folded into TD-027 | **Superseded** by TD-027 §21 (Module Boundary). |
