# Migration: routing-config deprecation (bead ddx-87fb72c2)

This guide covers the breaking change to `agent.routing.*` in
`.ddx/config.yaml`. The change ships as part of epic `ddx-fdd3ea36`
(endpoint-first routing).

## Summary

| Field                              | Status         | Action                       |
| ---------------------------------- | -------------- | ---------------------------- |
| `agent.routing.default_harness`    | **Removed**    | Delete the field             |
| `agent.routing.profile_ladders`    | Opt-in only    | Consulted only with `--escalate`        |
| `agent.routing.model_overrides`    | Opt-in only    | Consulted only with `--override-model`  |
| `agent.harness` (top-level)        | Kept           | Tie-break preference, not a default override |
| `agent.routing.profile_priority`   | Already deprecated | Still parsed; warns at load |

## `agent.routing.default_harness` → removed

The field has been deleted from the schema. DDx now rejects configs that
still carry it with a hard error at startup and a pointer back to this
guide. Exit code is non-zero.

### Before

```yaml
agent:
  harness: claude
  routing:
    default_harness: agent
```

### After

```yaml
agent:
  harness: claude
```

The top-level `agent.harness` field is preserved and remains honored as
a tie-break preference during route resolution. It is **not** a silent
default override.

## `agent.routing.profile_ladders` → opt-in via `--escalate`

The default execute path no longer iterates the ladder. A single
`ResolveRoute` call drives each attempt; `profile_ladders` is consulted
only when the operator explicitly passes `--escalate` to enable
tier-ladder escalation semantics.

If your config still sets `profile_ladders`, DDx emits a one-time
process warning at config-load time:

```
warning: agent.routing.profile_ladders is opt-in.
It is consulted only when --escalate is passed; the default execute
path ignores it. See docs/migrations/routing-config.md.
```

To silence the warning either drop the field, or pass `--escalate` on
the runs where you want tier escalation.

## `agent.routing.model_overrides` → opt-in via `--override-model`

The same treatment as `profile_ladders`. The default execute path
ignores `model_overrides`. Pass the new `--override-model` flag to opt
into per-tier override consultation:

```
ddx agent execute-loop --escalate --override-model
```

A one-time process warning fires at config-load time when the field is
set without invocation.

## Test commands

```bash
# Reject a config with the removed field:
echo 'version: "1.0"
library:
  path: ./library
  repository:
    url: https://example.com/repo
    branch: main
agent:
  routing:
    default_harness: claude' > /tmp/bad.yaml
DDX_PROJECT_ROOT=/tmp ddx status   # exits non-zero with migration message
```

## Server worker path alignment (ddx-c7081f89)

The server worker path (`ddx work` without `--local`) now matches the CLI path
semantics described above. Prior to bead `ddx-c7081f89`, the worker started
escalation whenever neither harness nor model was pinned. After that bead,
`WorkerSpec.Escalate` must be `true` for the tier loop to engage — identical to
the `--escalate` flag on the CLI path. The `--escalate` flag is forwarded to the
server worker spec automatically when passed to `ddx work`.

## Background

Prior to this change, three competing surfaces dictated harness
selection: the top-level `agent.harness` field, the routing block's
`default_harness`, and the live route-resolution call graph. The
endpoint-first redesign (epic ddx-fdd3ea36) collapses harness selection
into a single explicit decision per dispatch: route resolution is the
source of truth, and `agent.harness` is the operator's tie-break. The
ladder + overrides knobs remain available for operators who explicitly
want escalation or model substitution, but they are no longer
consulted by default.
