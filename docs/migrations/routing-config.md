# Routing Config Migration

DDx no longer stores durable provider or route catalogs. Fizeau is the agent
harness and routing authority; DDx is the work tracker and executor that sends
one execution request at a time.

## Provider configuration removed from DDx

DDx hard-rejects these fields in both project and user DDx config, including
when their value is empty or `null`:

- `agent.model`
- `agent.models`
- `agent.reasoning_levels`
- `agent.endpoints`

Move durable providers, model catalogs, endpoints, and credentials to Fizeau.
Fizeau loads project configuration from `.fizeau/config.yaml` and global
configuration from `~/.config/fizeau/config.yaml`; its precedence is built-in
defaults, global config, project config, environment variables, then request or
CLI flags. For example:

```yaml
# .fizeau/config.yaml
providers:
  studio:
    type: lmstudio
    include_by_default: true
    endpoints:
      - name: vidar
        base_url: http://vidar:1234/v1
```

Keep secrets in Fizeau's global config or environment variables rather than a
committed project file.

DDx may still send explicit, one-request constraints such as harness, provider,
model, policy, power bounds, reasoning, permissions, timeouts, role, working
directory, and correlation metadata. Those values are opaque request inputs:
DDx does not turn them into durable routing defaults and does not choose which
harness Fizeau should use. During retries DDx may raise only the abstract
`MinPower` floor. It preserves an operator's `MaxPower` and every other request
fact byte-for-byte; Fizeau interprets the complete request and chooses the
concrete route.

## Retired DDx routing defaults

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

See ADR-024 for retry and review policy: DDx may raise only `MinPower`; it
preserves operator `MaxPower` and all other request facts byte-identically.
Fizeau chooses the concrete route. DDx never normalizes, substitutes, widens,
or fuzzy-matches explicit passthrough values.

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
