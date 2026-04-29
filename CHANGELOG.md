# Changelog

All notable changes to DDx are documented in this file.

## [Unreleased]

### execute-loop: structured `declined_needs_decomposition` outcome

Adds a first-class outcome distinct from `no_changes` and `execution_failed`
for executors that conclude a bead is too large to deliver in one pass and
must first be split into sub-beads. Previously this case was conveyed via
free-form text in `no_changes_rationale`, and the loop kept re-attempting the
bead under a short cooldown — burning tokens to re-derive the same
recommendation across runs.

The new contract:

- `ExecuteBeadStatusDeclinedNeedsDecomposition` (`declined_needs_decomposition`)
  is a new value of the execute-bead status. Executors set it together with
  `DecompositionRecommendation []string` (recommended sub-bead titles) and
  optional `DecompositionRationale string` on `ExecuteBeadReport`.
- The execute-loop responds by appending a structured
  `decomposition-recommendation` event (JSON body carrying the rationale and
  recommended sub-beads) and parking the bead with a 365-day cooldown so
  subsequent loop iterations do not re-attempt it. The bead remains open;
  its status field is unchanged.
- A first-class CLI for the cooldown surface lands as `ddx bead cooldown
  show <id>` and `ddx bead cooldown clear <id>`. Operators should reach for
  these commands instead of editing the magic `execute-loop-retry-after`
  Extra key. The existing `ddx bead update --set/--unset
  execute-loop-retry-after=...` workflow continues to work as a power-user
  override but is deprecated for the decomposition case — clearing a
  cooldown via the new command is the intended path.

Migration: this is an additive contract. Executors that do not emit the new
status see no behaviour change. Existing cooldown workflows (no_changes
short cooldown, push_failed long park) are unchanged.

Regression coverage:
`TestExecuteBeadWorkerDeclinedNeedsDecompositionParksBead` in
`cli/internal/agent/execute_bead_loop_test.go`.

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
