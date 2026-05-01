# Investigation: root cause of 33–142h hung execute-bead workers

**Bead:** ddx-0a651925 (investigates; sibling to ddx-b808df39)
**Attempt:** 20260418T043148-1346e8a3
**Scope:** Source-code + execution-bundle analysis. No live hung worker was available
in this isolated worktree to capture live `ps`/`pprof`, so the incident evidence is
drawn from `.ddx/executions/*` bundles and the worker/runner source path.

---

## 1. Observed evidence (from `.ddx/executions/`)

| Metric                                               | Count / value                                        |
| ---------------------------------------------------- | ---------------------------------------------------- |
| Total attempt bundles                                | 328                                                  |
| Bundles with `result.json`                           | 291 (89%)                                            |
| Bundles with `manifest.json` but **no** `result.json` | 37 (11%) — executions whose worker never finalized  |

Top five longest *finalized* attempts:

| `duration_ms` | Wall time    | `detail`                                                                    | Bundle                          |
| ------------- | ------------ | --------------------------------------------------------------------------- | ------------------------------- |
| 7,720,908     | **2h 8m**    | `agent: compaction stuck: 50 consecutive failed compaction attempts`        | 20260416T230316-efbf91c5        |
| 7,200,098     | **2h 0m**    | `timeout after 2h0m0s` (claude streaming wall-clock fired)                  | 20260415T015231-c2ffa9ba        |
| 6,764,927     | **1h 53m**   | success (agent harness, qwen)                                               | 20260416T230315-f9f46822        |
| 5,649,987     | **1h 34m**   | compaction stuck                                                            | 20260416T230313-e193942b        |
| 4,284,347     | **1h 11m**   | compaction stuck                                                            | 20260415T103422-bb3a45fa        |

**Key observation:** the 33h / 142h hang durations cited in ddx-b808df39 are
~15× and ~65× longer than the longest *finalized* attempt on record. This means
the hung workers never reached the point of writing `result.json` — they were
blocked inside `runner.Run` (or deeper) for days. Every code path that
*should* have terminated them (idle timeout, compaction-stuck, stall detector,
cancel propagation) failed to fire.

The 37 manifest-only bundles are candidates for hung or externally-killed
workers; e.g. `20260416T215953-6b515c05` has a manifest from 2026-04-16T21:59Z
with no result. Those fit the profile of "workers running 33h–142h with no bead
assigned" reported on agent projects.

---

## 2. Root-cause classification (source code evidence)

The investigation checked the four hypotheses from the bead description.

### RC1 (PRIMARY) — Caller context is discarded inside the Runner

`cli/internal/agent/runner.go:239` (`Runner.Run`):
```go
ctx, cancel := context.WithCancel(context.Background())
```

`cli/internal/agent/agent_runner.go:163` (`Runner.RunAgent`):
```go
ctx, cancel := context.WithCancel(context.Background())
```

`RunOptions` (types.go) takes no `context.Context` at all. `agent.ExecuteBead`
likewise does not accept ctx, and `server.WorkerManager.runWorker` passes its
own ctx into `worker.Run(ctx, …)` but that ctx **never reaches the agent
invocation**.

**Impact:** `WorkerManager.Stop(id)` (workers.go:648) calls `handle.cancel()`
which cancels only the worker goroutine's ctx. The running agent provider call
/ subprocess is detached. The only way to interrupt a stuck agent today is a
SIGKILL from outside the process. This is the structural defect that enables
33–142h hangs — once the agent is stuck, nothing inside DDx can stop it.

### RC2 — Idle-reset timer, not wall-clock deadline

`cli/internal/agent/executor.go:35-40` creates an *idle timeout* context value:
```go
func withExecutionTimeout(ctx context.Context, timeout time.Duration) context.Context {
    return context.WithValue(ctx, executionTimeoutKey{}, timeout)
}
```

`executor.go:154-178` consumes it and **resets the timer on every stdout/stderr
byte**. `agent_runner.go:264-285` implements the same pattern against agentlib
events.

The default timeout is 2h (`types.go:363 DefaultTimeoutMS = 7200000`). The
comment claims "2 hours — long enough for any task; supervisor/agent decides
termination" but the actual behaviour is **"2 hours of silence required to fire"**.
A provider that emits a heartbeat, a ping, a retry line, or any byte resets the
timer indefinitely.

The `runClaudeStreaming` path (`claude_stream.go:373-384`) is the only path that
uses a wall-clock deadline — and it is the only path we see actually hitting
the timeout (bundle 20260415T015231 exactly 2h 0m 0.098s).

**Impact:** for `harness=agent` (qwen, minimax, openrouter, etc.), no wall-clock
bound exists. 33–142h hangs are consistent with a provider that dribbles events
slowly enough to defeat the idle reset.

### RC3 — Compaction-stuck breaker fires late

`cli/internal/agent/agent_runner.go:218-241` increments a counter on each no-op
compaction event and trips at `stuckThreshold = maxConsecutiveCompactionFailures`
(50 by default). On observed `efbf91c5` it took **2h 8m** to trip. If one
iteration takes 3 minutes to produce a no-op compaction event, 50 events =
~150 min — matches the observed tail.

**Impact:** on its own, the breaker is too slow to be a useful hang-guard. It
should be replaced or complemented by a wall-clock-bounded variant.

### RC4 — Provider HTTP requests likely lack a per-request deadline

`RunAgent` passes the internal `context.Background()`-derived ctx into
`agentlib.Run(ctx, req)`. The agent library's providers (`openai.Client`,
anthropic client) do not appear to install a per-request deadline themselves
(grepping the v0.3.14 module cache shows short timeouts on discovery/probe
only; the main call path has no WithTimeout). A stalled TCP socket after
headers are received but before the response body arrives can hang a streaming
read indefinitely.

**Impact:** consistent with "alive but mid-LLM-call with no client-side
deadline" — hypothesis (2) in the bead description. Cannot be fully confirmed
from source alone; needs runtime `go tool pprof http://localhost:<port>/debug/pprof/goroutine`
against a live hung worker.

### RC-skipped — BashTool subprocess hang (hypothesis 1)

Not the root cause. `github.com/DocumentDrivenDX/agent@v0.3.14/tool/bash.go:15`
installs a 120s default wall-clock timeout (`context.WithTimeout(ctx, timeout)`)
and calls `exec.CommandContext`. Per-call bash hangs self-terminate at 2min.

### RC-skipped — Tracker-side claim leak (hypothesis 3)

Tracker state is incidental to the hang: the bead's own description notes the
hung workers exist as processes (`ps aux | grep claude`). The worker process is
alive; the claim is alive because the worker is alive. Fixing the claim alone
does not kill the worker. This hypothesis is downstream, not upstream.

### RC-skipped — Worktree file-lock (hypothesis 4)

No evidence in bundles. Execute-bead worktrees are plain git worktrees at
`/tmp/ddx-exec-wt/*`; no process-held file locks would survive across 33–142h
without the owning process. If the process is alive, the lock is alive — same
as RC3.

---

## 3. Relationship to ddx-b808df39

ddx-b808df39 proposes an operator `ddx agent workers stop <id>` command that
sends **SIGTERM at the OS level**, then SIGKILL after a grace period. That
design **does** work around RC1 (process-level signal bypasses the broken ctx
plumbing). So ddx-b808df39 is a *correct mitigation* — but only a mitigation.

Without fixing RC1 (and at least RC2), operators will keep needing to invoke
`workers stop` manually. The autonomous remediations every other system relies
on (cancel ctx, idle timeout, circuit breaker) are each broken today in ways
that require explicit code changes, not just a new operator tool.

**Recommendation:** Ship ddx-b808df39 first (quick operational relief), then
resolve the child beads filed below in order RC1 → RC2 → RC4 → RC3.

---

## 4. Follow-up beads filed

Each addresses one root cause with a deterministic acceptance test. Priority
and ID are shown; full details available via `ddx bead show <id>`.

The IDs below carry an `.execute-bead-wt-…` prefix because `ddx bead create`
was invoked from inside the isolated execute-bead worktree — this is a
separate DDx bug worth noting but orthogonal to the hang investigation. They
are parented to `ddx-0a651925` and resolvable by `ddx bead list --parent ddx-0a651925`.

| RC  | Priority | Bead ID (suffix)                                                           | Title                                                              |
| --- | -------- | -------------------------------------------------------------------------- | ------------------------------------------------------------------ |
| RC1 | P0       | `…-526efaf1` | propagate caller context through Runner.Run and RunAgent           |
| RC2 | P0       | `…-b203c889` | add wall-clock deadline alongside idle timeout for agent harness   |
| RC3 | P2       | `…-b0562380` | bound compaction-stuck breaker by wall-clock time                  |
| RC4 | P1       | `…-3e3913a0` | audit and enforce per-request HTTP deadlines on agent provider calls |
| —   | P1       | `…-34d039b7` | autonomous worker watchdog with process-level reaper (complements ddx-b808df39) |

**Suggested execution order:** RC1 → RC2 → RC4 → RC3 → watchdog. RC1 is a
prerequisite for RC4 being meaningful.

---

## 5. What this investigation could NOT do

- Capture live `ps`/`pprof` from a hung worker — none was accessible from the
  isolated execute-bead worktree. RC4 in particular needs a live `go tool pprof`
  goroutine dump to confirm that the stuck goroutine is in `net/http` streaming
  read, not some other path.
- Confirm whether the agent library's providers install any timeout that was
  missed in grep. The listed RC4 bead should start with a provider-path audit
  rather than an immediate fix.
