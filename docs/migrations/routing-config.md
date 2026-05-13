# Routing Config Migration

Removed in `ddx-3bd7396a`.

`profile_ladders`, `model_overrides`, `default_harness`, and the old
`agent.routing.*` defaults are retired DDx routing config keys. DDx hard-rejects
them at config load so old local config cannot silently reintroduce concrete
route selection into DDx.

Delete those keys from project and user config. DDx execution config should
describe queue cadence, retry policy, power bounds, and evidence handling.
Fizeau routing config owns provider/model catalogs, fuzzy matching, fallback
policy, route health, and model alias resolution. DDx execution config owns
`run` / `try` / `work` policy, while Fizeau routing config owns the concrete
route. Explicit `--harness`, `--provider`, and `--model` values are
operator-supplied passthrough controls only; they are not replacement routing
policy and are not filled in from `agent.*` defaults.

See ADR-024 for retry and review policy: DDx may change `MinPower`,
`MaxPower`, and request facts, but Fizeau chooses the concrete route. DDx never
normalizes, substitutes, widens, or fuzzy-matches explicit passthrough values.

## Remaining Search-Hit Audit

The repository still contains historical `legacy agent`, `ddx-agent`,
`Agent Service`, and `cli/internal/agent` text in archived generated command
pages, superseded technical designs, alignment reviews, literal code/package
paths, fixtures, scripts, demos, generated website pages, and project-local
skill mirror snapshots under `.agents/skills/` and `.claude/skills/`. Those
hits are retained for traceability only. Current operator workflow guidance is
`ddx run`, `ddx try`, `ddx work`, and `ddx bead`; current routing-boundary
guidance is that DDx forwards raw passthrough strings unchanged and Fizeau owns
matching, routing, provider/model discovery, and route errors.

Active DDx skill guidance is the shipped `ddx` skill (`skills/ddx/`,
`.agents/skills/ddx/`, and `.claude/skills/ddx/`). Those copies must not contain
legacy workflow or DDx-owned-routing guidance. Obsolete mirror-only docs that had
no root-source counterpart, including old FEAT-010 and TP-020 routing-plan
snapshots, are removed from the project-local skill mirrors by this migration.
