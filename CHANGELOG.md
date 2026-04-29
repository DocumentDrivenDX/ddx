# Changelog

All notable changes to DDx are documented in this file.

## [Unreleased]

### Routing point release — ddx-agent v0.9.23

DDx now consumes the cumulative routing contract delivered upstream across
`github.com/DocumentDrivenDX/agent` v0.9.10..v0.9.24 (pinned at v0.9.23 in
`cli/go.mod`). Upstream shipped these capabilities incrementally across
multiple tags rather than as a single release, so this entry bundles the
combined contract DDx now relies on.

Consumed upstream capabilities (commit hash · upstream bead):

- **Cost-aware routing tiebreak** — `b85e90e` · [agent-0dafc7f0](~/Projects/agent/.ddx/beads.jsonl) (regression coverage tightened in `9c80eb1`). Adds cost as a routing dimension so equally-capable candidates resolve to the cheaper provider.
- **First-class routing profiles** — `b645280` · [agent-191a74f9](~/Projects/agent/.ddx/beads.jsonl). Ships the `default`, `local`, and `standard` profiles with zero-interaction defaults.
- **Routing reference docs** — `faafff2`, `db26108` · [agent-1f46cf22](~/Projects/agent/.ddx/beads.jsonl). Publishes the routing reference and clarifies the best-provider cost baseline.
- **Public route decision trace** — `0bc2bf4` · [agent-53f38d95](~/Projects/agent/.ddx/beads.jsonl). Exposes the candidate decision trace so callers can inspect why a route was chosen.
- **Supported-models allow-list** — `fd71e00` · [agent-bab52778](~/Projects/agent/.ddx/beads.jsonl). Gates exact pins by the harness allow-list so unsupported models fail fast.
- **Typed explicit-pin errors** — `5992c7b` · [agent-dfabb10b](~/Projects/agent/.ddx/beads.jsonl). Returns typed errors for explicit-pin routing failures instead of opaque strings.

The combined contract DDx consumes:

- **Zero-interaction defaults** via the `default` profile.
- **Profiles** (`default`, `local`, `standard`) selectable without bespoke config.
- **Override semantics** with explicit pins gated by the supported-models allow-list and surfaced through typed errors.
- **Public decision trace** for inspecting candidate selection.
- **Cost dimension** as a tiebreak in routing decisions.

This entry, together with the `cli/go.mod` pin at v0.9.23, satisfies AC #1 of
parent bead `ddx-fdd3ea36` — there is no single upstream release tag that
bundles the routing point release, so traceability lives here.
