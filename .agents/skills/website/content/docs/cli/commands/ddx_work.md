---
title: "work"
generated: true
---

## ddx work

Work the bead execution queue

### Synopsis

`ddx work` is the primary operator-facing surface for draining the bead
execution queue. It claims the next execution-ready bead, runs the bead
execution workflow from the project root, records the structured result,
and continues until no unattempted ready work remains.

Use `ddx try` when you want to execute one bead directly. Use `ddx work`
when you want the queue worker to keep advancing ready work.

Planning and document-only beads are valid execution targets — any bead
with unmet acceptance criteria and no blocking deps is eligible.

Close semantics (per execute-bead result status):
  success                      — close bead with session + commit evidence
  already_satisfied            — close bead (after repeated no_changes)
  no_changes                   — unclaim; may cooldown or close after retries
  land_conflict                — unclaim; result preserved under refs/ddx/iterations/
  post_run_check_failed        — unclaim; result preserved
  execution_failed             — unclaim
  structural_validation_failed — unclaim

Only success (and already_satisfied) closes the bead. Every other status
leaves the bead open and unclaimed so a later attempt can try again. Each
attempt is appended to the bead as an execution event (status, detail,
base_rev, result_rev, preserve_ref, retry_after), and the underlying
session log is recorded with the attempt.

`ddx work` runs inline in the current process. Per ADR-022 there is no
separate submit-to-server mode; the legacy --local flag is accepted only as a
deprecated no-op.

Project targeting (multi-project servers):
  --project <path>    target a specific project root (absolute path or name)
  DDX_PROJECT_ROOT    env var fallback; used when --project is not set
  (default)           the git root of the current working directory

When submitting to a multi-project server you must ensure the target project
is registered with the server (run "ddx server" from that directory, or use
"ddx server projects register"). The server rejects unrecognised project paths.


```
ddx work [flags]
```

### Examples

```
  # Drain the current execution-ready queue once and exit
  ddx work

  # Pick one ready bead, execute it, and stop
  ddx work --once

  # Run continuously as a bounded queue worker
  ddx work --poll-interval 30s

  # Forward harness/model as passthrough constraints for a debugging pass
  ddx work --once --harness agent --model minimax/minimax-m2.7

  # Constrain power bounds while leaving concrete routing to Fizeau
  ddx work --once --min-power 40 --max-power 90
```

### Options

```
      --adaptive-min-tier-window int   Trailing window size for adaptive min-tier evaluation (default 50)
      --effort string                  Effort level
      --from string                    Base git revision to start from (default: HEAD)
      --harness string                 Agent harness to use
  -h, --help                           help for work
      --json                           Output loop result as JSON
      --local                          Run inline in current process instead of server worker (default: submit to server)
      --max-cost float                 Stop the loop when accumulated billed cost exceeds USD; 0 = unlimited; subscription and local providers do not count (default 100)
      --max-tier string                Maximum tier for auto-escalation: cheap, standard, or smart (default: smart)
      --min-tier string                Minimum tier for auto-escalation: cheap, standard, or smart (default: cheap)
      --model string                   Model override
      --model-ref string               Model reference passthrough; resolved by Fizeau
      --no-adaptive-min-tier           Disable adaptive min-tier promotion based on trailing cheap-tier success rate
      --no-review                      Skip post-merge review (e.g. for doc-only beads or tight iteration loops)
      --once                           Process at most one ready bead
      --poll-interval duration         Poll interval for continuous scanning; zero drains current ready work and exits
      --profile string                 Routing profile: default, cheap, fast, or smart (default "default")
      --project string                 Target project root path or name (default: CWD git root). Env: DDX_PROJECT_ROOT
      --provider string                Provider name (e.g. vidar, openrouter); selects a named provider from config
      --review-harness string          Harness to use for the post-merge reviewer (default: same as implementation harness)
      --review-model string            Model override for the post-merge reviewer (default: smart tier)
```

### Options inherited from parent commands

```
      --config string              config file (default is $HOME/.ddx.yml)
      --library-base-path string   override path for DDx library location
  -v, --verbose                    verbose output
```

### SEE ALSO

* [ddx](/docs/cli/commands/ddx/)	 - Document-Driven Development eXperience - AI development toolkit

###### Auto generated by spf13/cobra on 21-Apr-2026
