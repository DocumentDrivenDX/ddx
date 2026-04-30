# Work — Drain the Queue, Execute, Verify, Close

"Doing work" in DDx means draining the ready queue: pick the top ready bead,
run one or more `ddx try` attempts, verify the result, and close the bead on
success or leave it available for a future eligible retry.

## Default surface: `ddx work`

```bash
ddx work
```

`ddx work` drains the queue by picking ready beads and invoking `ddx try`.
`ddx try` wraps `ddx run`, which is the single agent invocation primitive. DDx
owns queue iteration, attempt evidence, and retry policy; the upstream agent
owns provider/model routing.

Flags worth knowing:

- `--once` — pick one bead and stop (don't loop).
- `--poll-interval <dur>` — continuous worker mode; wait between
  iterations.
- `--min-power <n>` / `--max-power <n>` — requested agent power bounds.
- `--top-power` — choose a `MinPower` threshold from the agent catalog.
- `--harness <name>` / `--provider <name>` / `--model <ref>` — passthrough
  constraints only. DDx sends them unchanged and does not route on them.
- `--local` — run inline in the current process (no subprocess).

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

| Outcome | Meaning | Action |
|---|---|---|
| `success` | Tests pass, AC met, commit landed | `ddx bead close <id>` |
| `already_satisfied` | No changes needed (AC was already green) | `ddx bead close <id>` |
| `no_changes` | Agent returned without producing commits | Leave open, **unclaim** |
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
- **Closing on no_changes**: only `success` and `already_satisfied`
  close a bead. `no_changes` means the agent returned without doing
  anything — unclaim and investigate.
- **Squashing bead-attempt commits**: the per-attempt history is
  an audit trail (evidence commits, heartbeats). Use only
  `git merge --ff-only` or `--no-ff`; never squash/rebase/filter.
- **Running passthrough pins without a reason**: power-bound dispatch lets the
  agent choose an appropriate route. Use `--harness`, `--provider`, or
  `--model` only for explicit operator constraints, bug repros, or controlled
  tests.
- **Parallel workers on the same claimed bead**: the tracker
  guards against this via claim semantics, but don't try to defeat
  it — each claim represents an in-flight attempt.

## CLI reference

```bash
ddx work                                    # default queue drain
ddx work --once                             # one bead, then stop
ddx work --poll-interval 30s                # continuous worker
ddx work --min-power 10                     # request stronger attempts
ddx work --harness claude                   # passthrough constraint

ddx try <id>                                # one bead attempt
ddx try <id> --from <rev>                   # override base commit
ddx try <id> --no-merge                     # preserve iteration
```

Full flag list: `ddx work --help`, `ddx try --help`, `ddx run --help`.
