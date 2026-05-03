# Queue ops parity benchmark — JSONL vs axon (1100-bead fixture)

Bead: ddx-6602559c
Run: 20260503T224250-eaffad54
Base rev: d22276ea6c337e3d855c7cde20b2a6513a081e2a

## Setup

- Benchmark: `cli/internal/bead/bench_test.go`
- Fixture: 1100 synthesized beads (`makeBenchBeads`):
  - 770 closed (70%)
  - 330 open (30%), partitioned into:
    - no deps (ready) — `i % 3 == 0`
    - dep on a closed bead (ready) — `i % 3 == 1`
    - dep on an open bead (blocked) — `i % 3 == 2`
- Both stores are seeded by `WriteAll`, so steady-state ReadAll/Get/Ready/Blocked
  cost is what is measured, not init.
- Host: linux/arm64, GOMAXPROCS=12.
- Command: `cd cli && go test -run='^$' -bench=BenchmarkQueueOps -benchmem -benchtime=3s -count=3 ./internal/bead/`

## Results (3 runs each, ns/op + B/op + allocs/op)

### Ready

| backend | ns/op (run 1) | ns/op (run 2) | ns/op (run 3) | mean ns/op | B/op    | allocs/op |
|---------|---------------|---------------|---------------|------------|---------|-----------|
| jsonl   |  10,203,875   |   9,839,896   |   9,676,577   |  9,906,783 | 5,310,990 |    50,666 |
| axon    |  12,230,155   |  12,406,345   |  12,277,749   | 12,304,750 | 5,602,085 |    57,498 |

axon vs jsonl: **+24.2%** ns/op, **+5.5%** bytes, **+13.5%** allocs.

### Blocked

| backend | ns/op (run 1) | ns/op (run 2) | ns/op (run 3) | mean ns/op | B/op    | allocs/op |
|---------|---------------|---------------|---------------|------------|---------|-----------|
| jsonl   |  10,093,746   |   9,585,360   |  10,310,737   |  9,996,614 | 5,229,076 |    50,665 |
| axon    |  12,143,847   |  12,074,667   |  11,733,034   | 11,983,849 | 5,520,193 |    57,497 |

axon vs jsonl: **+19.9%** ns/op, **+5.6%** bytes, **+13.5%** allocs.

### Show (Get by ID)

| backend | ns/op (run 1) | ns/op (run 2) | ns/op (run 3) | mean ns/op | B/op    | allocs/op |
|---------|---------------|---------------|---------------|------------|---------|-----------|
| jsonl   |   9,785,559   |  10,042,288   |   9,754,156   |  9,860,668 | 4,988,824 |    50,421 |
| axon    |  11,542,308   |  11,718,443   |  11,316,988   | 11,525,913 | 5,279,942 |    57,253 |

axon vs jsonl: **+16.9%** ns/op, **+5.8%** bytes, **+13.5%** allocs.

## Verdict (AC §3, §4)

**axon is NOT at parity with JSONL on any of the three queue ops at 1100-bead
scale.** axon is consistently 17–25% slower in wall time and ~14% higher in
allocation count.

Per AC §4 the bead is **held open** for triage. See
`.ddx/executions/20260503T224250-eaffad54/no_changes_rationale.txt`.

## Likely root cause (for the follow-up bead)

axon's `ReadAll` reads two JSONL files (`ddx_beads` + `ddx_bead_events`) and
re-merges events back into `Extra["events"]` so the higher-level Store sees
the inline-events shape it expects (see `axon_backend.go` package doc and the
constants `AxonBeadsCollection` / `AxonEventsCollection`). The fixture has no
events at all, so the per-bead merge is pure overhead — every queue op pays
for two file reads, two parse passes, and a per-bead post-process loop where
JSONL pays for one. The follow-up should:

1. Confirm the same gap appears under `pprof` / allocation profiling on the
   `ReadAll` path, and
2. Decide whether axon should skip the events-merge fast-path when no events
   exist for a bead, or whether the queue ops should consult a beads-only view
   that bypasses the merge entirely.
