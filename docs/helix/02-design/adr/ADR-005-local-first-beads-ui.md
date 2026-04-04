---
ddx:
  id: ADR-005
  depends_on:
    - FEAT-008
    - FEAT-004
    - ADR-002
---
# ADR-005: Local-First Client-Side Data Layer for Beads UI

**Status:** Accepted
**Date:** 2026-04-04
**Context:** The DDx web UI needs a beads view with rich search, filtering,
graph traversal, and kanban board functionality. The design must avoid
building server-side search infrastructure and instead leverage browser-native
data technologies.

## Decision

The beads UI fetches the full bead dataset on load and performs all search,
filter, sort, and graph traversal client-side. The in-browser data layer uses
**SQLite compiled to WASM** (via `sql.js` or `@electric-sql/pglite`) for
structured queries, with TanStack Query managing the fetch/cache lifecycle.

### Why Not Server-Side Search?

DDx beads are small datasets (hundreds to low thousands of records, each
a few KB of JSON). Building server-side search, pagination, and filtering
would add:

- Server complexity: search indexes, query parsing, pagination cursors
- Network latency: every filter change = round trip
- State sync: client and server must agree on filter state
- Maintenance: two implementations of the same query logic

Client-side data eliminates all of this. The browser is the query engine.

### Data Flow

```
Server                          Browser
──────                          ───────
GET /api/beads ──────────────→  TanStack Query cache
  (full JSONL,                    │
   typically <100KB)              ▼
                               SQLite WASM
                               (in-memory db)
                                  │
                         ┌────────┼────────┐
                         ▼        ▼        ▼
                      List     Kanban    Search
                      View     Board    Results
```

1. On mount, TanStack Query fetches `GET /api/beads` (returns all beads as
   JSON array)
2. Response is loaded into an in-memory SQLite database (WASM)
3. All views (list, kanban, search, graph) query the local SQLite
4. Mutations (`POST`, `PUT`) call the server API, then invalidate the
   TanStack Query cache → re-fetch → reload SQLite
5. Polling or SSE keeps the local copy fresh (configurable interval)

### SQLite WASM Schema

```sql
CREATE TABLE beads (
  id TEXT PRIMARY KEY,
  title TEXT NOT NULL,
  status TEXT NOT NULL,
  issue_type TEXT,
  priority INTEGER DEFAULT 2,
  owner TEXT,
  description TEXT,
  acceptance TEXT,
  spec_id TEXT,
  labels TEXT,           -- comma-separated, indexed via FTS
  parent TEXT,
  created_at TEXT,
  updated_at TEXT,
  raw_json TEXT          -- full JSON for fields not in columns
);

CREATE TABLE bead_deps (
  bead_id TEXT NOT NULL,
  dep_id TEXT NOT NULL,
  PRIMARY KEY (bead_id, dep_id)
);

-- Full-text search across text fields
CREATE VIRTUAL TABLE beads_fts USING fts5(
  id, title, description, acceptance, labels,
  content='beads', content_rowid='rowid'
);

CREATE INDEX idx_beads_status ON beads(status);
CREATE INDEX idx_beads_priority ON beads(priority);
CREATE INDEX idx_beads_spec_id ON beads(spec_id);
```

### Why SQLite WASM Over Alternatives

| Option | Verdict | Reason |
|--------|---------|--------|
| **SQLite WASM (sql.js)** | **Chosen** | Full SQL, FTS5 for search, graph queries via recursive CTEs, mature, ~1MB WASM |
| **WebSQL** | Rejected | Deprecated, removed from standards, Chrome-only |
| **IndexedDB + js search** | Considered | No SQL, manual indexing, poor full-text search |
| **OPFS + SQLite** | Future | Persistent storage, but adds complexity for v1 |
| **PGlite (Postgres WASM)** | Overkill | 3MB+, Postgres features not needed for this dataset |
| **Plain array + filter()** | Fragile | Works for 50 beads, breaks at 500 with complex queries |

### Graph Traversal

Dependency graph traversal uses recursive CTEs:

```sql
-- All transitive dependencies of a bead
WITH RECURSIVE deps AS (
  SELECT dep_id FROM bead_deps WHERE bead_id = ?
  UNION
  SELECT bd.dep_id FROM bead_deps bd
  JOIN deps d ON bd.bead_id = d.dep_id
)
SELECT b.* FROM beads b JOIN deps d ON b.id = d.dep_id;

-- "Ready" beads: no open dependencies
SELECT b.* FROM beads b
WHERE b.status = 'open'
AND NOT EXISTS (
  SELECT 1 FROM bead_deps bd
  JOIN beads dep ON bd.dep_id = dep.id
  WHERE bd.bead_id = b.id AND dep.status != 'closed'
);
```

### Kanban Board Data Model

The kanban board is a view over the SQLite data:

```sql
-- Columns: one per status
SELECT * FROM beads WHERE status = ? ORDER BY priority ASC, created_at ASC;

-- Swimlanes: group by label prefix
SELECT * FROM beads
WHERE status = ? AND labels LIKE '%area:cli%'
ORDER BY priority ASC;

-- Dependency clusters: beads sharing a dep chain
WITH RECURSIVE cluster AS (
  SELECT bead_id, dep_id FROM bead_deps WHERE bead_id = ?
  UNION
  SELECT bd.bead_id, bd.dep_id FROM bead_deps bd
  JOIN cluster c ON bd.dep_id = c.bead_id OR bd.bead_id = c.dep_id
)
SELECT DISTINCT bead_id FROM cluster
UNION SELECT DISTINCT dep_id FROM cluster;
```

### Execution Evidence Join

Execution runs linked to beads are fetched on demand (not preloaded):

```
Click bead detail → fetch GET /api/exec/runs?artifact=<bead-id>
                   → fetch GET /api/agent/sessions?bead=<bead-id>
```

These are loaded into separate TanStack Query caches, not SQLite, because
they're per-bead detail data, not list data.

## Consequences

- **No server-side search needed** — the server's `GET /api/beads` is a
  simple dump. No query parameters, no pagination, no search index.
- **Instant UI interactions** — filter/search/sort are sub-millisecond
  because they query local SQLite, not the network.
- **~1MB WASM overhead** — sql.js adds ~1MB to the initial bundle. This is
  acceptable for a developer tool dashboard.
- **Stale data window** — between polling intervals, the local copy may be
  behind. Mutations trigger immediate re-fetch to minimize this.
- **Scale limit** — this approach works for up to ~10,000 beads (~5MB JSON).
  Beyond that, server-side pagination becomes necessary. DDx projects are
  unlikely to exceed this.
- **Offline potential** — if we later add OPFS persistence, the beads view
  works offline. This is a free option, not a v1 requirement.

## Alternatives Considered

- **Server-side search with ElasticSearch/Bleve:** Massive overkill for
  hundreds of records. Adds infrastructure DDx doesn't need.
- **TanStack Table with client-side arrays:** Works for simple filtering
  but can't do recursive graph traversal or FTS efficiently.
- **CRDT-based sync (PowerSync, ElectricSQL):** Interesting for real-time
  multi-user, but DDx is single-user dashboard backed by files. Git is the
  sync mechanism.
