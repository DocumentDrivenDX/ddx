# Changelog

All notable changes to DDx are documented in this file.

## [Unreleased]

### Breaking changes

#### Removed: `agent.routing.default_harness`

The `agent.routing.default_harness` field has been removed from `.ddx/config.yaml`
(bead ddx-87fb72c2). DDx now refuses to load any config that still carries it
and prints a migration pointer instead.

Why: `default_harness` was a silent fallback that competed with the top-level
`agent.harness` setting and the live route-resolution call graph. It produced
confusing routing decisions when the upstream service had no viable provider for
the requested profile. The endpoint-first redesign (epic ddx-fdd3ea36) makes
harness selection an explicit decision per dispatch — a config-level "default"
no longer fits.

Migration: delete the field. The top-level `agent.harness` is still honored as
a tie-break preference on the resolution path (NOT a default override). See
[docs/migrations/routing-config.md](docs/migrations/routing-config.md).

#### Opt-in: `agent.routing.profile_ladders` and `agent.routing.model_overrides`

These fields are no longer consulted on the default execute path. They are now
explicit opt-in:

- `profile_ladders` is consulted only when `--escalate` is passed.
- `model_overrides` is consulted only when the new `--override-model` flag is
  passed.

Configs that still set these fields will load successfully but emit a one-time
process warning at config-load time. To silence the warning, either pass the
opt-in flag on the relevant invocation or remove the field. See the migration
guide for details.

### Fix: `latency_ms` in claude harness traces now reflects per-call duration

Previously every `llm.response` event in `.ddx/agent-logs/agent-claude-*.jsonl`
had `latency_ms == elapsed_ms` because the claude stream parser wrote the
cumulative wall-clock value into both fields. Per-call LLM latency was
therefore unrecoverable from captured traces.

The parser now tracks the time the most recent LLM request was sent (session
start, then reset on each `user`/tool_result event) and emits
`latency_ms = now - lastRequestSentAt` for each assistant turn. `elapsed_ms`
remains cumulative, so for any event past turn 1 the two values diverge.

Legacy traces produced before this commit have `latency_ms == elapsed_ms` and
should be ignored by latency-analysis tooling for runs predating the fix.

Regression coverage: `TestParseClaudeStreamLatencyIsPerCall` in
`cli/internal/agent/claude_stream_test.go` drives two assistant turns through
the parser with real sleeps and asserts (a) `latency_ms != elapsed_ms` for
turn 2, (b) the second-turn latency excludes the prior turn and tool window,
and (c) the sum of per-call latencies does not exceed the final event's
`elapsed_ms`.

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
