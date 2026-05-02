---
ddx:
  id: ADR-007
  depends_on:
    - FEAT-020
    - FEAT-021
    - ADR-006
---
# ADR-007: Federation Topology (Star, Active-Spoke Hybrid)

**Status:** Accepted
**Date:** 2026-05-02
**Context:** Each `ddx server` has stable node identity, persisted state, and a
project registry (FEAT-020). The web UI (FEAT-021) uses `/nodes/:nodeId/...` as
its canonical URL grammar and was anticipated to fan out across nodes in a
future "coordinator" model. Story 14 (FEAT-026) makes that future concrete:
multiple ddx-server nodes need to present a unified view of beads, runs, and
projects across machines, while the spokes remain fully usable on their own.

## Decision

Federation is a **star topology** with an **active-spoke hybrid** authority
model:

- One node runs as the **hub** (`ddx server --hub-mode`) and exposes
  `/api/federation/*` plus federated read aggregations.
- Other nodes run as **spokes** (`ddx server --hub=<host>`) and push
  registration + heartbeat to the hub. A node may run as both (`hub_spoke`).
- Spokes remain full ddx-servers; if the hub dies, spokes keep working
  autonomously and their UIs remain directly reachable as a fallback path.
- Reads are aggregated by the hub via fan-out over the spokes' existing
  `/graphql` endpoints. Writes are **never broadcast** — the hub may forward a
  mutation to the owning spoke, but federation itself is read-only in v1.

### Naming Convention

Federation always uses **federation / hub / spoke**. Other terms are reserved or
avoided:

| Term | Status | Reason |
|------|--------|--------|
| federation, hub, spoke, hub_spoke | Use | Adopted vocabulary for this feature |
| coordinator | Avoid | Already used for the per-project bead-landing serializer (FEAT-010 `coordinatorRegistry`, `land_coordinator.go`, `coordinatorMetricsByProject`); overloading the term causes confusion |
| primary, replica | Avoid | Implies replication semantics; federation is registration + read fan-out, not data replication |
| leader, follower | Avoid | Implies consensus / failover semantics not present in v1 |
| master, worker | Avoid | Overloaded with FEAT-006 worker pool |

This convention applies to code (package names, identifiers, log messages),
APIs (endpoint paths, GraphQL field names, JSON schema), CLI flags, and
documentation.

## Topology

### Star (one hub, N spokes)

- v1 fan-out is **O(spokes)** per federated query. Concurrency is bounded
  (default 8 in flight) and each spoke call has a per-request timeout (default
  2s for register/heartbeat, 5s for read fan-out).
- The hub does not relay between spokes; spokes never talk to each other.
- Failover is **out of scope**. There is exactly one hub. A new hub is brought
  up by an operator restart; spokes will re-register on their next heartbeat
  cycle.
- Spoke UIs are first-class fallbacks. `/api/node` and CLI status output expose
  the spoke's local URL so operators have a direct path when the hub is
  unreachable.

### Authority: Hybrid (push registration/heartbeat, pull data on demand)

**Push side (spoke → hub):**
- `POST /api/federation/register` on startup. Registration is **idempotent on
  stable `node_id`**: re-register replaces the registry entry by id; URL change
  updates the recorded URL; a duplicate `node_id` from a different identity
  (mismatched name + new URL) is rejected with a logged conflict and an
  operator-actionable error.
- `POST /api/federation/heartbeat` every **30s with random jitter** in
  `[0, 5s]` to avoid synchronized beats from waking the hub at the same instant.
- `POST /api/federation/deregister` on graceful shutdown.

**Pull side (hub → spoke):**
- The hub fans out to spokes' existing `/graphql` over ts-net for federated
  read queries. No new spoke endpoint is added for fan-out beyond what already
  exists for direct UI use.

**Liveness states:**
- `live` — heartbeat within 30s.
- `stale` — no heartbeat for ≥ 2 minutes; UI shows a stale badge but the spoke
  is still queried in fan-out.
- `offline` — fan-out request to the spoke failed (connection refused, ts-net
  unreachable, timeout). Distinct from `stale`: a `live` heartbeat does not
  guarantee the spoke is reachable from the hub at query time.

The hub registry persists at `~/.local/share/ddx/federation-state.json` so the
hub survives restarts and rebuilds its view from disk on startup; spokes
re-confirm on their next heartbeat.

### Discovery: Explicit `--hub=<host>` Over ts-net DNS

- `ddx server --hub-mode` exposes `/api/federation/*` on the existing listener.
- `ddx server --hub=<host>` registers + heartbeats on startup against
  `<host>:<port>`. `<host>` is typically a ts-net hostname (e.g. `eitri`).
- Both flags may be combined (`--hub-mode --hub=<other>`) for `hub_spoke`.
- mDNS / Tailscale-tag auto-discovery is **rejected for v1**: it couples DDx to
  Tailscale ACL policy and adds a discovery surface we don't need yet.
- `.ddx.yml` static config is a **follow-up**. When introduced, the
  precedence rule is: **flag wins over config** (consistent with ADR-006's
  auth-key precedence rule).

### ts-net Policy: Default Strict, Explicit Dangerous Opt-out

- Default behavior: federation refuses non-loopback, non-ts-net peers. The
  policy is enforced at `/api/federation/*` regardless of the listener that
  accepted the request. Loopback is permitted to support same-host
  `hub_spoke` topologies and integration tests.
- `--federation-allow-plain-http` is the explicit opt-out. It is **only valid
  when `--hub-mode` is also set**. When enabled:
  - The hub accepts plain-HTTP federation traffic (loopback or otherwise).
  - The hub emits a warning log on each accepted plain-HTTP registration and
    on startup when the flag is in effect, naming the offending peer.
  - Spokes that wish to send plain-HTTP must opt in symmetrically (see
    ambiguity #1 below — current decision: **both ends must opt in**, so
    spokes carry the same flag).
- Rationale: making the dangerous mode noisy and explicit is safer than
  letting users build worse external tunnels around an over-strict default.
  This mirrors ADR-006's posture on transport-layer auth.

### Write Routing Contract (Story 15 Hand-off)

Federation is **read-only aggregation in v1**. Federated read models expose
per-row routing metadata so a future write path can resolve the owning node:

| Field | Type | Purpose |
|-------|------|---------|
| `node_id` | string | Stable id of the spoke that owns this row |
| `project_id` | string | Project id within that node |
| `project_url` | string | Spoke base URL (for direct fallback) |
| `project_path` | string | On-disk path on the owning node |
| `write_capability` | enum | `local`, `forwardable`, `read_only` |
| `status` | enum | `live`, `stale`, `offline` of the owning node |

When Story 15 (server-side prompt execution) lands, the hub may **forward** a
mutation to the owning spoke based on this metadata, but it must **never
broadcast** writes. ADR-007 documents the contract; Story 15's design
references it.

## Version Handshake

Spokes send a version block on `POST /api/federation/register`:

```json
{
  "node_id": "node-7029e8d6",
  "node_name": "bragi",
  "url": "https://bragi:7743",
  "ddx_version": "0.42.0",
  "schema_version": "1",
  "graphql_schema_version": "2026-04-29",
  "capabilities": ["beads", "runs", "projects", "doc-graph"]
}
```

The hub applies this **compatibility matrix** and either accepts, marks
`degraded`, or rejects:

| Spoke vs Hub | `schema_version` | `graphql_schema_version` | `ddx_version` (semver) | Result |
|---|---|---|---|---|
| Same | equal | equal | equal | **accept** (`live`) |
| Spoke older patch | equal | equal | older patch only | **accept** (`live`) |
| Spoke newer patch | equal | equal | newer patch only | **accept** (`live`) |
| Spoke newer minor | equal | newer date | newer minor | **accept as `degraded`** — hub queries only the capability set both sides advertise |
| Spoke older minor | equal | older date | older minor | **accept as `degraded`** — hub avoids fields the spoke does not advertise |
| Mismatched major | equal | n/a | major mismatch | **reject** at register time, with explicit error |
| Mismatched `schema_version` | not equal | n/a | n/a | **reject** at register time |
| Missing `capabilities` | n/a | n/a | n/a | **reject** — hub cannot make safe field-level decisions |

`degraded` is surfaced in the UI as a yellow badge on the affected spoke and
on every row sourced from it. Reject decisions are made at register time so
operators see the failure immediately rather than at query time.

## Spec Surface (Compatibility Hooks)

ADR-007 is implemented across three feature specs:

- **FEAT-026** (new) — owns federation end-to-end: registry schema,
  hub/spoke endpoints, fan-out client, federated GraphQL fields, UI routes.
- **FEAT-020 amendment** — adds `federation-state.json` schema, the
  `--hub-mode`, `--hub=<host>`, `--federation-allow-plain-http` flags, and
  the `node.federation_role` field on `/api/node`. Compatibility hooks only,
  no implementation.
- **FEAT-021 amendment** — adds the `/federation` overview route, hub-resolved
  `/nodes/:nodeId/...` routing across registered spokes, and `?scope=federation`
  on combined views. Compatibility hooks only, no implementation.

## Consequences

- **Operator clarity:** one hub, N spokes; explicit flags; explicit failure
  modes (`stale`, `offline`, `degraded`, conflict-rejected).
- **No replication, no consensus:** the hub holds an in-memory + persisted
  view of where data lives, not a copy of the data. Loss of the hub state file
  is recoverable on the next heartbeat cycle.
- **Spoke autonomy:** spokes do not depend on the hub for correctness. Each
  spoke UI remains usable; CLI and direct browser access work without the hub.
- **Hub fan-out cost:** O(spokes) per federated read. Bounded concurrency and
  per-call timeouts cap the cost; a single slow spoke cannot block the view.
- **Plain-HTTP escape valve is documented and noisy:** the dangerous mode is
  explicit, log-loud, and gated behind both `--hub-mode` and a separate flag.
- **Future write path is preserved:** routing metadata fields are part of
  every federated row, so Story 15 can layer write forwarding without a v2
  schema migration.
- **Naming discipline:** `coordinator`, `primary`, `replica`, `leader` are
  reserved/avoided so the federation vocabulary does not collide with
  pre-existing per-project subsystems.

## Alternatives Considered

| Alternative | Verdict | Reason |
|---|---|---|
| Star topology, push-only (spokes push all data) | Rejected | Doubles storage and forces continuous replication; federation use cases are read-aggregation, not redundancy |
| Star topology, pull-only (hub polls spokes) | Rejected | Hub does not know about spokes until they push; auto-discovery of spokes is explicitly out of scope |
| Mesh topology (every node peers with every other node) | Rejected | O(N²) connections, no clear authority, harder to reason about for v1 |
| Single shared SQLite over a network drive | Rejected | Lock contention, no isolation, fragile across machines |
| Failover / hot standby hub | Deferred | Operator restart is acceptable for v1; complexity not justified yet |
| mDNS / Tailscale-tag spoke discovery | Deferred | Couples DDx to Tailscale ACL policy; explicit `--hub=` is sufficient |

## Flagged Ambiguities (resolved here unless noted)

1. **Plain-HTTP opt-out scope.** Decision: **both ends must opt in**. A spoke
   that wishes to send plain-HTTP carries `--federation-allow-plain-http`; the
   hub must also carry it to accept. This keeps the dangerous surface
   symmetric and audit-loggable.
2. **Version-skew granularity.** Decision: matrix above. Reject on major or
   `schema_version` mismatch; mark `degraded` on minor/graphql-schema drift.
3. **`scope: LOCAL | FEDERATION` GraphQL migration.** **Open.** v1 ships
   parallel `federated*` fields per FEAT-026 design. A future migration to
   unify under a `scope` argument is allowed but not committed to a release
   in ADR-007.
4. **Heartbeat cadence config surface.** Deferred to v1.1: flag, env var, or
   `.ddx.yml`. v1 is hard-coded at 30s + jitter.
5. **Duplicate `node_id` recovery UX.** v1 hard-rejects with a logged
   conflict and an operator-actionable error. Soft-rebind on operator action
   is a follow-up.
