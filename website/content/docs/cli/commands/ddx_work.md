---
title: "work"
generated: true
---

## ddx work

Drain the bead execution queue

### Synopsis

work drains the execution-ready bead queue. It is the FEAT-010 layer-3
queue drain: it iterates ddx try (layer 2) across ready beads until a stop
condition is met and owns retry-power policy between attempts.

ddx work treats --harness, --provider, and --model as opaque passthrough
constraints forwarded to Fizeau unchanged. DDx does not validate these values
or branch on them; Fizeau owns routing within the requested power
bounds.

Review is on by default. --no-review is a break-glass override and
requires --no-review-i-know-what-im-doing. A bead label of review:skip
is only honored when it also carries a sibling review:skip-reason:*
label; otherwise the label is ignored.

Stop conditions (evaluated between attempts):
  drained     — no ready beads remain
  blocked     — every remaining bead has produced a terminal non-success outcome
  deferred    — configured budget exhausted
  no_progress — N consecutive attempts produced no commit (default N=3)
  signal      — SIGINT/SIGTERM received between attempts

work runs inline in the current process; per ADR-022 there is no separate
"submit to server" mode. The legacy --local flag is accepted but ignored
(deprecation warning printed) and will be removed in a future release.


```
ddx work [flags]
```

### Examples

```
  # Drain the current execution-ready queue and exit
  ddx work

  # Pick one ready bead, execute it, and stop
  ddx work --once

  # Watch for newly-ready work after the current queue drains
  ddx work --watch

  # Watch with a shorter idle scan interval
  ddx work --watch --idle-interval 15s

  # Forward harness/model as passthrough constraints (ddx does not validate these)
  ddx work --once --harness agent --model minimax/minimax-m2.7

  # Skip review only with the break-glass acknowledgement flag
  ddx work --once --no-review --no-review-i-know-what-im-doing

  # Constrain power tier (retry-power policy is owned by ddx work)
  ddx work --once --min-power 40 --max-power 90
```

### Options

```
      --effort string                    Effort level
      --from string                      Base git revision to start from (default: HEAD)
      --harness string                   Agent harness constraint (passthrough; ddx work does not validate)
  -h, --help                             help for work
      --idle-interval duration           Sleep duration between empty-queue scans in watch mode (default 30s)
      --json                             Output loop result as JSON
      --max-bead-cost float              Per-bead cost budget in USD; stop escalating when this bead's billed cost exceeds this amount (0 = unlimited); overridden per-bead by a budget:<USD> label (default 5)
      --max-cost float                   Stop when accumulated billed cost exceeds USD; 0 = unlimited (default 100)
      --max-power int                    Maximum model power allowed (0 = unconstrained); passed to agent routing unchanged
      --max-recovery-cost float          Per-bead automated recovery budget in USD for reframe/decompose attempts after repeated ladder exhaustion (default 2)
      --min-power int                    Minimum model power required (0 = unconstrained); passed to agent routing unchanged
      --model string                     Model constraint (passthrough; ddx work does not validate)
      --model-ref string                 Model reference passthrough (e.g. code-medium); resolved by Fizeau
      --no-review                        Skip post-merge review (break-glass: requires --no-review-i-know-what-im-doing)
      --no-review-i-know-what-im-doing   Break-glass acknowledgement required when using --no-review
      --once                             Process at most one ready bead
      --profile string                   Routing profile: default, cheap, fast, or smart (empty = unconstrained; let the agent service choose)
      --project string                   Target project root path or name (default: CWD git root). Env: DDX_PROJECT_ROOT
      --provider string                  Provider constraint (passthrough; ddx work does not validate)
      --rate-limit-max-wait duration     Per-bead total wait budget for HTTP 429 / rate-limit retries (default 5m). 0 keeps the default; negative disables retry. (default 5m0s)
      --request-timeout duration         Per-request provider wall-clock timeout; overrides project config and model-class defaults
      --review-harness string            Harness for the post-merge reviewer (default: same as implementation harness)
      --review-model string              Model override for the post-merge reviewer (default: smart tier)
      --watch                            Keep watching for newly-ready beads after the current queue drains
```

### Options inherited from parent commands

```
      --config string              config file (default is $HOME/.ddx.yml)
      --library-base-path string   override path for DDx library location
  -v, --verbose                    verbose output
```

### SEE ALSO

* [ddx](/docs/cli/commands/ddx/)	 - Document-Driven Development eXperience - AI development toolkit
* [ddx work analyze](/docs/cli/commands/ddx_work_analyze/)	 - Query cross-attempt performance from .ddx/metrics/attempts.jsonl
* [ddx work clear-cooldowns](/docs/cli/commands/ddx_work_clear-cooldowns/)	 - Bulk-clear queue-drain cooldowns
* [ddx work focus](/docs/cli/commands/ddx_work_focus/)	 - Show work requiring operator attention
* [ddx work metrics](/docs/cli/commands/ddx_work_metrics/)	 - Metrics over attempt evidence
* [ddx work plan](/docs/cli/commands/ddx_work_plan/)	 - Preview what 'ddx work' would pick (dry-run)

###### Auto generated by spf13/cobra on 12-May-2026
