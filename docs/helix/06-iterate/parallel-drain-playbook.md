# Parallel drain operator playbook

Use this playbook only after `ddx-1f774edf` has landed. Before that, `Land()`
still spans network I/O under `withMainGitLock`, so running multiple
`ddx work --once` shells in parallel is unsafe and mostly just creates lock
contention.

## 1. When To Use Parallel Drain

Parallel drain is for the post-`ddx-1f774edf` state, when the drain path is
local-only and the bottleneck moves from network round-trips to normal merge
conflicts. If that bead has not landed yet, do not try to scale by opening
more shells.

## 2. How To Start

Open `N` tmux panes or terminal shells and run a separate `ddx work --once`
loop in each one. The goal is external parallelism, not a new `ddx work
--parallel N` flag.

## 3. Mix Harnesses

Do not run all workers on the same harness if you want true throughput.
Mixing harnesses avoids serializing on a single account or provider quota.

Example:

```sh
# Pane 1
ddx work --once --harness claude --max-cost 25

# Pane 2
ddx work --once --harness codex --max-cost 25

# Pane 3
ddx work --once --harness openrouter --max-cost 25
```

If `openrouter` is not the right local name in your environment, use an
equivalent third harness or provider-backed shell. The point is to spread load
across different rate-limit buckets instead of pinning every worker to the same
one.

## 4. Budget The Cost Ceiling

Each `ddx work` invocation accepts `--max-cost`, and that ceiling applies per
shell. Budget the session as the sum across all `N` shells, not as one shared
global cap.

## 5. Expect Some Preservation

Parallel landing does not mean every attempt lands cleanly. Under load, some
workers will preserve their iteration ref instead of merging immediately. That
is normal. Keep the preserved refs and merge them manually once the shared-main
conflict clears.

See `ddx-42741260` for chaos coverage of that preservation behavior under load.

## 6. Watch The Metrics

When contention looks suspicious, check the lock-wait, lock-hold, and retry
metrics emitted by `ddx-128c7f9d`. Those are the operator-visible channels for
debugging whether the remaining bottleneck is lock acquisition, merge pressure,
or something else.

## Notes

- This playbook is about external shells. It does not introduce a new router or
  worker-spawn design.
- The single-worker harness selection guidance still applies inside each shell;
  this section only adds the parallel-drain operator pattern.
