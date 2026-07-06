# ADR-029: Federation hub-arbitrated pre-claim lease

- Status: Accepted
- Date: 2026-07-06
- Related: ADR-006 (ts-net authentication), ADR-007 (federation topology), FEAT-026 (federation)

## Context

Two federated DDx nodes (a hub and its spokes) can drain the **same** project's
bead queue. Bead claims today are local-only: a per-node `beads.jsonl` claim plus a
`/tmp` PID-based liveness lease (`internal/bead/claim_liveness.go`), whose PID
staleness check only works same-machine. Nothing coordinates claims across nodes,
and the hub's own managed workers do not consult federation state at all. As a
result two nodes independently claim and execute the same bead, producing duplicate
work and divergent git history (this already yielded two incompatible storage-layer
rewrites of the same beads).

A separate change adds **force-sync around claims** (git pull before claim, push
after land). That propagates claims but leaves a TOCTOU window: both nodes pull
(bead free) → both claim → both push. The window is the whole attempt for slow
beads, and git auto-merge of append-only `beads.jsonl` will not reliably surface the
double-work as a conflict.

## Decision

Introduce a **hub-arbitrated pre-claim lease** as the authoritative cross-node
mutual-exclusion primitive, composed with (not replacing) force-sync.

1. **Local-server-proxied acquire.** `ddx work`/`ddx try` run in a process that
   cannot see federation role, `node_id`, or hub URL — those live in the server. The
   worker queries `GET /api/federation/self` (`{federated, node_id, role}`) and, when
   federated, calls **local** claim endpoints via the skip-verify local client. The
   local server services them as hub or forwards them as spoke. Non-federated nodes
   are a hard no-op with zero overhead.

2. **Picker-layer hook, not the intake hook.** The acquire runs at candidate
   selection (where `pickerSkip` lives). A hub deny yields a new transient
   `lease_held_remote` skip reason that skips the candidate, is **excluded from the
   stall-escalation streak** (`preClaimIdleEscalationThreshold`), and surfaces in
   `ddx work status`/`doctor` as "waiting on peer lease."

3. **Lease protocol** (hub, under its existing `sync.Mutex`, persisted with
   Sync+atomic-rename, persist-before-respond), keyed `{project_id, bead_id}`:
   - Acquire is **idempotent per `{project,bead,node}`**; reassigns only after
     `expires_at + grace` (`grace ≥ max_clock_skew + one_renew_interval`), lazily at
     acquire time (no background sweep).
   - Renew and release **match on `lease_id == current`** (no-op otherwise), so a
     late release after reassignment cannot delete the successor's lease.
   - **Fence-at-land**: the worker calls a `validate` endpoint the hub answers
     authoritatively under its mutex immediately before push; the node-local expiry
     timer is never trusted at land.

4. **Fail-closed.** When the hub is unreachable the worker skips (does not drain).
   A mutex must not let an isolated node fail-open into split-brain during a
   partition. Single-node (non-federated) draining is unaffected.

5. **Timing reconciled with local liveness.** Renew is driven off the real
   `HeartbeatInterval` (30s); the federation lease TTL is aligned near local
   `HeartbeatTTL` (90s) so a hung node's cross-node lease and same-machine claim
   expire on comparable timescales.

6. **Identity bound to the peer.** The hub derives the holder from
   `federationRequestIdentity` (ts-net peer / registered spoke) and rejects requests
   whose peer ≠ claimed `node_id` (today's gate trusts the body `node_id`, spoofable
   over loopback). Hub-local workers use an explicit authenticated self-node path.

7. **State schema guard.** Adding `Claims` bumps the federation state
   `schema_version`; `SaveStateTo` refuses to write (hub read-only + WARN) when the
   on-disk schema is newer than the binary, preventing an older binary from silently
   dropping leases.

8. **Ownership invariant relaxed for shared-drain.** Lease keying is independent of
   project ownership; `RouteMutationToProjectOwner` (which errors on "multiple spokes
   claim the same project") is relaxed for shared-drain projects, which now have no
   single mutation owner.

9. **Sequencing contract with force-sync:** `acquire → pull → local claim → work →
   fence-validate → land → push → release`; release strictly after push; **no release
   on fenced-abort** (TTL reclaims). The two changes land together or one gates the
   other.

## Consequences

- Cross-node duplicate execution of a shared bead is eliminated in the normal case;
  the fencing token closes the land-after-reassign hole force-sync alone cannot.
- The hub becomes a hard dependency for **cross-node** draining (fail-closed); hub
  availability/fast-restart matters. Non-federated single-node draining is unchanged.
- Scope limit: exclusion is per shared `bead_id`. Two nodes that each *file* their
  own bead for the same task get distinct IDs and are not covered (a filing-time
  dedup concern). Widening bead-ID entropy for federated projects is a follow-up.
