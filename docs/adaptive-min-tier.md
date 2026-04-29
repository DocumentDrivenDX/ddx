# Adaptive Min-Tier

The adaptive min-tier feature automatically promotes the minimum routing tier
from `cheap` to `standard` when cheap-tier attempts have been consistently
failing, protecting the execution queue from burning time and budget on attempts
that are unlikely to succeed.

## How it works

Each time `ddx agent execute-loop` (or `ddx work`) claims a bead, it reads the
most recent N execution results from `.ddx/executions/*/result.json` and
computes the cheap-tier trailing success rate:

```
cheap_success_rate = cheap_successes / cheap_attempts  (within trailing window)
```

If all of the following are true, the cheap tier is skipped for this run:
- `cheap_attempts >= 3` (AdaptiveMinTierMinSamples) — enough evidence
- `cheap_success_rate < 0.20` (AdaptiveMinTierThreshold) — rate below threshold

When the cheap tier is skipped, a log line is emitted:

```
adaptive min-tier: skipping cheap tier (trailing success rate 0.00 over 9 attempts; threshold 0.20) — min-tier=standard
```

### Infra failure exclusion

Attempts whose `failure_mode` indicates an infrastructure or configuration
failure that did not reach an actual model invocation are **excluded** from the
success-rate computation:

| `failure_mode`          | Excluded? | Reason                                         |
|-------------------------|-----------|------------------------------------------------|
| `no_viable_provider`    | Yes       | No route could be resolved; model never called |
| `harness_not_installed` | Yes       | Harness binary missing; model never called      |
| All others              | No        | Model was invoked; outcome is real signal       |

This prevents a configuration outage (e.g., a missing harness binary or a
routing misconfiguration) from permanently condemning cheap tier after the root
cause is resolved.

## Metric store

| Location | Format | Purpose |
|----------|--------|---------|
| `.ddx/executions/{attempt-id}/result.json` | JSON object, one per execution | Primary source for adaptive computation |
| `.ddx/agent-logs/routing-outcomes.jsonl` | JSONL, append-only | Harness-level analytics (latency, cost, success rate) — **not** used by adaptive min-tier |
| `.ddx/agent-logs/adaptive-reset.json` | JSON object | Reset marker written by `route-status reset` |

### `result.json` schema (fields read by adaptive min-tier)

```json
{
  "harness":      "claude",
  "model":        "claude-haiku-4-5",
  "outcome":      "task_succeeded | task_failed | task_no_changes",
  "failure_mode": "no_viable_provider | harness_not_installed | ..."
}
```

Executions with an empty `harness` field are already excluded (these represent
attempts where no harness could be resolved at all).

### `adaptive-reset.json` schema

```json
{ "reset_at": "2026-04-29T16:27:02Z" }
```

After a reset, `AdaptiveMinTier` ignores every execution whose directory
timestamp (`YYYYMMDDTHHMMSS-<hex>`) predates `reset_at`. Historical data is
preserved on disk; only the adaptive view of it changes.

## Diagnosing "why is cheap being skipped?"

Run:

```
ddx agent route-status --adaptive
```

Example output:

```
Adaptive Min-Tier State
--------------------------------------------------
window size:      50 attempts
reset at:         (none)
total in window:  12 (all tiers)
cheap attempts:   9 (contributing to success rate)
infra-skipped:    3 (excluded: no_viable_provider / harness_not_installed)
cheap successes:  0
success rate:     0.00  (threshold: 0.20, min-samples: 3)
effective floor:  standard  [cheap tier is SKIPPED — success rate below threshold]

To reset: ddx agent route-status reset --yes
```

For JSON output (useful for scripting):

```
ddx agent route-status --adaptive --json
```

## Resetting the adaptive metric

When the underlying problem has been fixed (e.g., a misconfigured model name
corrected, a missing harness installed, or a quota restored), reset the trailing
window to give cheap tier a fresh start:

```
ddx agent route-status reset --yes
```

This writes `.ddx/agent-logs/adaptive-reset.json` with the current timestamp.
The output names every file touched:

```
Adaptive min-tier state cleared.

Touched: /path/to/project/.ddx/agent-logs/adaptive-reset.json

Cheap-tier will now be evaluated from a clean baseline.
Run 'ddx agent route-status --adaptive' to verify the new state.
```

After the reset, the next `execute-loop` invocation evaluates cheap tier on its
own merits. No prior trailing-window verdict carries over.

Without `--yes`, the command prints what it would do and exits cleanly (no
changes made):

```
ddx agent route-status reset
```

```
This will write a reset marker that causes the adaptive min-tier logic
to ignore all execution history recorded before this moment.
Cheap-tier eligibility will be re-evaluated on its own merits.

Re-run with --yes to confirm:
  ddx agent route-status reset --yes
```

## Bypassing vs. resetting

| Mechanism | Effect | Use case |
|-----------|--------|----------|
| `--no-adaptive-min-tier` | Permanently disables adaptive gating for that run | Testing; known-good environments where you never want gating |
| `route-status reset --yes` | Clears trailing-window state; gating re-activates from scratch | After fixing the root cause of cheap-tier failures |

Prefer reset over permanent bypass so the adaptive guard can re-activate if
failures recur.

## Configuration flags (on `execute-loop`)

| Flag | Default | Description |
|------|---------|-------------|
| `--adaptive-min-tier-window` | 50 | Number of most-recent attempts in the trailing window |
| `--no-adaptive-min-tier` | false | Disable adaptive gating entirely for this run |
| `--min-tier` | (adaptive) | Pin the minimum tier; overrides adaptive evaluation |
