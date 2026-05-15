# Work — Drain the Queue, Execute, Verify, Close

## Mode note

This reference covers **`bead_execution` mode**: explicit worker commands such as
`ddx work`, `ddx try <id>`, "execute bead `<id>`", or "start the worker".

If the user said "what should I work on next?", "what's blocking the queue?", or
"work the queue" without an explicit bead ID or `ddx work` command, they are in
**`interactive-steward`** mode — see `reference/interactive.md`. The steward
plans; the worker executes. Do not instruct manual ready-bead implementation in
response to broad orientation prompts.

---

"Doing work" in DDx means draining the ready queue: pick the top ready bead,
run one or more `ddx try` attempts, verify the result, and close the bead on
success or leave it available for a future eligible retry.

## Default surface: `ddx work`

```bash
ddx work
```

`ddx work` drains the queue by picking ready beads and invoking `ddx try`.
`ddx try` wraps `ddx run`, which is the single agent invocation primitive. DDx
owns queue iteration, attempt evidence, and retry policy; Fizeau owns concrete
routing, provider/model discovery, alias resolution, fuzzy matching, catalog
lookups, and transcript/session rendering.

Flags worth knowing:

- `--once` — pick one bead and stop (don't loop).
- `--watch` — keep running after the current ready queue drains; wait for newly-ready beads.
- `--idle-interval <dur>` — sleep duration between empty-queue scans in watch mode (default 30s).
- `--min-power <n>` / `--max-power <n>` — requested agent power bounds.
- `--top-power` — choose a `MinPower` threshold from the service's reported
  power bands.
- `--harness <name>` / `--provider <name>` / `--model <ref>` / `--profile <name>` — passthrough constraints only. DDx sends them
  unchanged and does not route on them.

## Are workers running for this project?

When someone asks "is the worker still working beads?" or "how is the queue
progressing?", do not answer with a global `ps aux | grep -E 'ddx work|ddx try'`
scan. That returns workers from every checkout on the host and falsely
signals progress on whichever project happens to be asked about.

```bash
ddx work status                # live workers scoped to this project
ddx work status --json         # machine-readable
ddx work status --all-projects # explicit cross-project escape hatch
```

`ddx work status` defaults to the current project root (CLI flag → env
`DDX_PROJECT_ROOT` → CWD git root). It reports pid, age, command line,
project root, and — when inferable from argv or an isolated execute-bead
worktree — the active bead ID. Always name the project root the answer
applies to; never substitute a worker from a different repository as
evidence that this project is progressing.

`--all-projects` is the only sanctioned way to ask "what ddx workers
exist anywhere on this host?" — use it only when the question is
explicitly cross-project.

## Primitive: `ddx try`

For targeted re-runs, debugging, or running a specific bead:

```bash
ddx try <bead-id>
ddx try <bead-id> --from <rev>      # base commit override
ddx try <bead-id> --no-merge        # preserve result, don't land
```

`ddx try` runs one bead attempt in an isolated git worktree. It calls `ddx run`
for the actual agent invocation.

## Pick by default, not by ID

Under normal operation, **don't specify a bead ID**. `ddx work` picks
the top ready bead based on priority + dependency satisfaction. Only
pin a specific ID when debugging (`ddx try <id>`) or
when the queue ordering would pick the wrong bead and you need to
override.

## Starting and monitoring workers in the background

When the user asks you to **start workers and watch progress**, do
not invent your own `tail | grep` pipeline — the obvious shapes have
all bitten in the field. Use these vetted patterns.

### One worker, foreground drain

If the user just wants to drain once and read the result at the end,
run `ddx work` directly and let it print. Don't wrap with `head`,
`tail`, or `tee` — pipes to those tools buffer stdout for the entire
run, so the user sees nothing during execution and everything at the
end.

### One or more workers, watch live

```bash
# Start each worker, one log per worker:
ddx work > /tmp/ddx-worker1.log 2>&1 &   # repeat for worker2, etc.

# Stream with per-worker prefixes (drops the `==>` separator noise
# that `tail -F` injects when switching files):
(tail -F /tmp/ddx-worker1.log 2>/dev/null | awk '{print "w1: "$0; fflush()}' &
 tail -F /tmp/ddx-worker2.log 2>/dev/null | awk '{print "w2: "$0; fflush()}' &
 wait) | grep -E --line-buffered \
  "^w[0-9]+: (▶|✗|✓|→ |worker exited|attempts:|failed:|landing worktree has|lock_contention|panic|FATAL|excluded|caller_excluded|ladder exhausted|escalation|powerClass=|do route|i/o timeout|connection refused|provider error)"
```

Why those specific tokens — together they cover **every actionable
event** in a drain:

- `▶ <bead>` — a new bead was claimed. `✗` and `✓` close it.
- `→` — outcome arrow, also follows a bead-state transition.
- `worker exited` / `attempts:` / `failed:` — end-of-drain summary.
- `landing worktree has` — pre-claim hook hit a stale index; queue
  is jammed if recovery doesn't kick in.
- `lock_contention`, `panic`, `FATAL` — infrastructure problems.
- `excluded`, `caller_excluded`, `ladder exhausted`,
  `escalation`, `powerClass=` — the escalation path actually
  firing; absence on a failed bead means the loop is silently
  no-op'ing instead of climbing the ladder.
- `do route` — the implementation-phase routing decision, surfaces
  the provider/model that's about to be hit. Without this in the
  alternation, the monitor goes quiet during the most interesting
  window (provider call) and only resurfaces on success/failure.
- `i/o timeout`, `connection refused`, `provider error` — transport
  failures. Two in a row to the same `(provider, model)` is the
  signature of routing that isn't honoring exclusions.

### What NOT to do

- **Don't wrap `ddx work` in `| tail -N`** to "see the last N
  lines". The pipe buffers until the producer exits, so you see
  nothing for the whole run. Redirect to a file and tail the file
  separately.
- **Don't grep `^==>` into the alternation.** Without the awk
  prefix, `tail -F file1 file2` emits `==> filename <==` whenever
  the active file switches — every switch becomes a noise event in
  a Monitor pipeline. Strip those, or prefix lines with `awk`.
- **Don't tail without `--line-buffered`/`fflush()`.** Pipe
  buffering means events arrive in 4KB chunks (often minutes apart)
  instead of as they happen. The `awk` form above flushes per line.
- **Don't share one log file across workers.** Interleaved output
  destroys attributability and any chance of debugging which worker
  hit what.

## Verify independently before closing

**Agents hallucinate successful completions.** Do not trust the
agent's self-report. Before closing a bead:

1. Run the acceptance-criteria command yourself (from the bead's
   `accept:` field). If it's `go test ./foo/...`, run that.
2. Check the resulting commit against the in-scope file list from
   the bead description. Out-of-scope files touched? Reject the
   attempt.
3. Read the commit message — does it reference the bead ID?
   (`[ddx-<id>]` or similar in the commit subject is the convention.)

If all three pass: close the bead.

## Close on success, unclaim on failure

DDx try/work outcomes form a specific taxonomy. Each outcome
maps to a concrete follow-up action:

During a live `ddx work` / `ddx try` attempt, the current bead's lifecycle is
orchestrator-owned. The worker executing that bead must not run
`ddx bead update <bead-id> --claim`, `ddx bead update <bead-id> --status <status>`,
`ddx bead update <bead-id> --unclaim`, or `ddx bead close <bead-id>` for the
current bead. The Step 0 split-first path still allows child-bead
`ddx bead create`, `ddx bead dep add`, and the parent-note update
`ddx bead update <parent-id> --notes 'decomposed into <child-ids>'`.

| Outcome | Meaning | Action |
|---|---|---|
| `success` | Tests pass, AC met, commit landed | `ddx bead close <id>` |
| `already_satisfied` | No changes needed (AC was already green) | `ddx bead close <id>` |
| `no_changes` | Agent returned without producing commits and wrote `no_changes_rationale.txt` | Leave open, **unclaim** |
| `no_evidence_produced` | Agent exited without a commit or rationale | Leave open, **unclaim**, investigate harness/commit failure |
| `land_conflict` | Merge conflict on landing the result | Leave open, **unclaim** |
| `post_run_check_failed` | Tests or gate failed after landing | Leave open, **unclaim**, investigate |
| `execution_failed` | Agent subprocess errored (timeout, crash, provider error) | Leave open, **unclaim** |
| `structural_validation_failed` | Result failed structural sanity check | Leave open, **unclaim**, investigate |

`ddx work` applies these actions automatically. If you're running
`ddx try` directly, apply them manually: `ddx bead update <id>
--unclaim` to release a bead after a non-closing outcome, so another
worker can pick it up.

**Never leave a bead half-owned** — every execution either closes or
unclaims.

## Testing expectations for bead implementations

When an agent implements a bead, its output should include tests
that exercise the new code:

- **Unit tests** for logic with in-memory stubs for first-party
  collaborators. Favor stubs over mocks; mocks that assert on call
  sequences test implementation, not behavior.
- **Integration tests** against real collaborators where the cost
  is small (real git in a temp dir, real DB in a temp file, real
  HTTP via a local test server).
- **Real e2e tests** at the outermost boundary. E2e tests that mock
  the database or the network are unit tests lying about their
  scope.
- **Coverage measurement only where the project tracks it.** Don't
  introduce coverage tooling just for one bead; use what the
  project already has. Coverage is a signal, not a target.
- **Performance claims require baselines.** If the bead's AC
  mentions "faster" or "scales", the acceptance test must include:
  (a) a numeric baseline, (b) an explicit boundary (what's
  measured, what's excluded), (c) a reproducible harness.

## Anti-patterns

- **Trusting the agent's "done"**: always re-run the AC command
  yourself before closing.
- **Closing on no_changes/no_evidence**: only `success` and `already_satisfied`
  close a bead. `no_changes` requires an explicit rationale. `no_evidence_produced`
  means the agent returned without a commit or rationale — unclaim and investigate.
- **Squashing bead-attempt commits**: the per-attempt history is
  an audit trail (evidence commits, heartbeats). Use only
  `git merge --ff-only` or `--no-ff`; never squash/rebase/filter.
- **Running passthrough pins without a reason**: power-bound dispatch lets the
  agent choose an appropriate route. Use `--harness`, `--provider`, `--model`,
  or `--profile` only for explicit operator constraints, bug
  repros, or controlled tests.
- **Parallel workers on the same claimed bead**: the tracker
  guards against this via claim semantics, but don't try to defeat
  it — each claim represents an in-flight attempt.

## CLI reference

```bash
ddx work                                    # default queue drain
ddx work --once                             # one bead, then stop
ddx work --watch                            # continuous worker
ddx work --watch --idle-interval 15s        # shorter idle scan interval
ddx work --min-power 10                     # request stronger attempts
ddx work --harness claude                   # passthrough constraint
ddx work --profile default                  # passthrough constraint

ddx try <id>                                # one bead attempt
ddx try <id> --from <rev>                   # override base commit
ddx try <id> --no-merge                     # preserve iteration

ddx work status                             # live workers for this project
ddx work status --json                      # machine-readable
ddx work status --all-projects              # opt-in cross-project view
```

Full flag list: `ddx work --help`, `ddx try --help`, `ddx run --help`, `ddx work status --help`.
