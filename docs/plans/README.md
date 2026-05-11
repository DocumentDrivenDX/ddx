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
| 2026-05-11 | [Website evolution: design system, auto-gen, Hugo renderer, Pages](plan-2026-05-11-website-evolution.md) | Draft — user review required before bead breakdown |
