# Work — Drain the Queue, Execute, Verify, Close

"Doing work" in DDx means draining the ready queue: pick the top
ready bead, dispatch an agent, verify the result, and close the bead
on success (or unclaim it on failure).

## Default surface: `ddx work`

```bash
ddx work
```

`ddx work` is an alias for `ddx agent execute-loop`. It drains the
queue by picking the top ready bead, running it through
`ddx agent execute-bead`, and advancing to the next one. Prefer this
over running individual commands.

Flags worth knowing:

- `--once` — pick one bead and stop (don't loop).
- `--poll-interval <dur>` — continuous worker mode; wait between
  iterations.
- `--harness <name>` — force a specific harness (overrides profile).
- `--profile default|cheap|fast|smart` — routing intent (default `default`).
- `--model <ref>` — exact model pin (overrides profile).
- `--local` — run inline in the current process (no subprocess).

## Primitive: `ddx agent execute-bead`

For targeted re-runs, debugging, or running a specific bead:

```bash
ddx agent execute-bead <bead-id>
ddx agent execute-bead <bead-id> --from <rev>      # base commit override
ddx agent execute-bead <bead-id> --no-merge        # preserve result, don't land
```

`execute-bead` runs an agent against one bead in an isolated git
worktree. It's what `ddx work` calls under the hood.

## Pick by default, not by ID

Under normal operation, **don't specify a bead ID**. `ddx work` picks
the top ready bead based on priority + dependency satisfaction. Only
pin a specific ID when debugging (`ddx agent execute-bead <id>`) or
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

DDx execute-bead outcomes form a specific taxonomy. Each outcome
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
`execute-bead` directly, apply them manually: `ddx bead update <id>
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
- **Squashing execute-bead commits**: the per-attempt history is
  an audit trail (evidence commits, heartbeats). Use only
  `git merge --ff-only` or `--no-ff`; never squash/rebase/filter.
- **Running `--harness` without a reason**: profile-first routing
  (`--profile smart`) picks a reasonable default. Override only
  when pinning for a bug repro or controlled test.
- **Parallel workers on the same claimed bead**: the tracker
  guards against this via claim semantics, but don't try to defeat
  it — each claim represents an in-flight attempt.

## CLI reference

```bash
ddx work                                    # default queue drain
ddx work --once                             # one bead, then stop
ddx work --poll-interval 30s                # continuous worker
ddx work --harness claude --profile smart   # pin harness, profile

ddx agent execute-bead <id>                 # primitive
ddx agent execute-bead <id> --from <rev>    # override base commit
ddx agent execute-bead <id> --no-merge      # preserve iteration

ddx agent execute-loop                      # same as ddx work
```

Full flag list: `ddx work --help`, `ddx agent execute-bead --help`.
