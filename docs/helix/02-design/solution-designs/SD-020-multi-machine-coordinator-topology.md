---
ddx:
  id: SD-020
  depends_on:
    - FEAT-002
    - FEAT-006
    - API-001
    - SD-019
---
# Solution Design: Multi-Machine Land Coordinator Topology

## Purpose

Define the operator-visible contract for running `ddx-server` on multiple
machines against the same project. Each machine runs its own land coordinator;
a shared git remote is the serialization point that prevents conflicting
landings without any machine-to-machine coordination protocol.

## Scope

- One land coordinator per machine × project
- Push-to-origin as the distributed serialization point
- Conflict, divergence, and recovery paths
- Operator setup and daily commands

Out of scope:

- Cross-machine `beads.jsonl` replication (see ddx-e2f497c3 for framing)
- Cross-machine worker supervision or bead-claim visibility
- Cross-machine observability aggregation (see ddx-e2f497c3)
- Cross-machine beads.jsonl concurrency (see ddx-2791dc4f)

## Topology Model

Each machine that participates in execute-loop work runs one `ddx-server`
process. That server hosts one land coordinator goroutine per registered
project. The coordinator owns the rebase → fast-forward → push sequence for
all landings on that machine.

```
  eitri                           bragi
  ┌──────────────────┐            ┌──────────────────┐
  │  ddx-server      │            │  ddx-server      │
  │  land coordinator│            │  land coordinator│
  │  (project: ddx)  │            │  (project: ddx)  │
  └────────┬─────────┘            └────────┬─────────┘
           │ git push --ff-only             │ git push --ff-only
           └──────────────┐  ┌─────────────┘
                          ▼  ▼
                     origin (shared remote)
                     single source of truth
                     for target branch
```

Both coordinators are autonomous. Neither knows about the other. The remote
serializes them through the atomicity of `git push`.

## Setup

Each machine needs:

1. A local clone of the project with `origin` pointing at the shared remote.
2. `ddx server` running (or enabled as a user service per FEAT-002 and SD-019).
3. The machine's `ddx-server` registered with the project root so the land
   coordinator starts.

No additional configuration is required. Machines do not need to reach each
other; they only need write access to the shared `origin`.

## Happy Path

1. Worker on machine A claims a bead (bead is marked claimed in local
   `beads.jsonl`; not yet visible on machine B).
2. Worker executes the bead in an isolated worktree.
3. Land coordinator on machine A: `git fetch origin`, rebase onto
   `origin/<target>`, verify fast-forward, `git push --ff-only origin <target>`.
4. Push succeeds. Origin advances. Machine A closes the bead.
5. Machine B's coordinator will see the new tip on its next fetch before its
   own rebase step.

Operators see a linear history on origin. Each landed commit carries the
`Ddx-Attempt-Id` and `Ddx-Worker-Id` trailers so post-hoc attribution is
unambiguous across machines.

## Conflict Path (Concurrent Push)

Two machines race: both complete a bead and attempt to push at roughly the
same time.

- **Winner** (push accepted): origin advances, bead is closed, history is
  linear.
- **Loser** (push rejected with non-fast-forward): the coordinator detects the
  rejection, preserves the iteration under `refs/ddx/iterations/<attempt-id>`,
  sets result status `land_conflict`, and unclaims the bead. The loser **never
  force-pushes**.

The unclaimed bead is now ready for retry from the new origin tip. Machine B's
worker (or machine A's next loop pass) will pick it up, fetch the updated base,
and run it again from scratch.

This is identical to the single-machine land-conflict path described in
FEAT-006 and API-001; push rejection from a remote is handled the same way as
a local fast-forward conflict.

## Divergence Path (Force-Push or Manual Ref Rewrite)

If someone force-pushes to the target branch on origin from outside DDx, the
affected machine's coordinator will detect a non-ff divergence on its next
`git fetch` before the rebase step. The coordinator fails the submission with
a clear error and stops.

**DDx does NOT auto-force-push to recover.** Operator must reconcile manually:

```bash
git fetch origin
git log --oneline HEAD..origin/<branch>   # see what diverged
git reset --hard origin/<branch>          # realign local to origin
```

After realignment, `ddx-server` can be restarted (or the coordinator
re-enabled) and the execute-loop resumes from the current origin tip.

## Operator Commands

**See what other machines have landed:**

```bash
git fetch origin && git log --oneline HEAD..origin/<branch>
```

**Check active workers on each machine:**

```bash
# run on each machine individually
ddx server workers list
```

**Inspect a preserved conflict iteration:**

```bash
git show refs/ddx/iterations/<attempt-id>
```

**After manual divergence recovery:**

```bash
git fetch origin && git reset --hard origin/<branch>
# then restart ddx-server or re-enable the coordinator
```

## Bead Visibility Across Machines

Each machine's bead store (`beads.jsonl`) is local. A bead claimed on machine A
is not visible as claimed on machine B unless both machines share or replicate
`beads.jsonl` through external means. This design does **not** address that
synchronization. In practice:

- For single-operator workflows, one machine drives the bead queue and the
  other machines run read-only or are idle.
- For peer-mesh workflows where multiple machines claim beads concurrently,
  bead-store concurrency must be solved first (ddx-2791dc4f) before
  multi-machine claiming is safe.

## Registry and Tombstoning

Each machine maintains its own project registry and handles its own coordinator
lifecycle. Registry deduplication and cleanup per machine follows ddx-23395b6a.
There is no cross-machine registry sync in this design.

## Relationship to SD-019

SD-019 defines the single-machine, multi-project topology: one `ddx-server`
process hosting multiple project coordinators on one host. SD-020 extends that
to multiple hosts: each host runs the SD-019 topology independently, and the
origin remote provides the only coordination point between hosts.

## Invariants

1. A coordinator **never force-pushes** to origin.
2. A coordinator always fetches from origin before its rebase step.
3. A push rejection always results in preserve-under-ref + unclaim, never a
   retry with force.
4. The origin target branch is the single source of truth. Local branch state
   is always subordinate to origin after a fetch.

## Validation

This design should be covered by integration tests that verify:

- two concurrent coordinators targeting the same origin: one lands, the other
  preserves and unclaims without force-pushing
- a coordinator that detects origin divergence (non-ff) stops and surfaces a
  clear operator-facing error
- the losing coordinator's preserved ref contains the exact iteration commit
  and the bead returns to `open` status
