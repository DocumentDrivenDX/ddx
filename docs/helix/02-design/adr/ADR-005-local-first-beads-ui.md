---
ddx:
  id: ADR-005
  depends_on:
    - FEAT-008
    - FEAT-004
    - ADR-002
---
# ADR-005: Local-First Client-Side Data Layer for Beads UI

**Status:** Accepted (revised 2026-04-07)
**Date:** 2026-04-04
**Revised:** 2026-04-07
**Context:** The DDx web UI needs a beads view with search, filtering,
dependency traversal, and kanban board functionality. The design must avoid
building server-side search infrastructure and instead leverage the browser.

## Decision

The beads UI fetches the full bead dataset on load and performs all search,
filter, sort, and graph traversal client-side using **plain JavaScript arrays**
with TanStack Query managing the fetch/cache lifecycle.

### Revision History

**v1 (2026-04-04):** Chose SQLite WASM (sql.js) for client-side queries.

**v2 (2026-04-07):** Replaced sql.js with plain arrays. SQLite WASM introduced
unacceptable deployment complexity:

- 660KB WASM binary required in the build output
- WASM MIME type (`application/wasm`) not served correctly by Go's embed FS
  SPA handler, causing fatal Emscripten crashes with no error recovery
- `go run` caching issues meant the WASM binary was not consistently embedded
- The dataset (~200 beads, <100KB JSON) is far too small to benefit from SQL
  indexes, FTS5, or recursive CTEs — `Array.filter()` executes in <1ms

### Why Not Server-Side Search?

DDx beads are small datasets (hundreds to low thousands of records, each
a few KB of JSON). Building server-side search, pagination, and filtering
would add unnecessary complexity. The browser handles it trivially.

### Data Flow

```
Server                          Browser
──────                          ───────
GET /api/beads ──────────────→  TanStack Query cache
  (full JSON array,               │
   typically <100KB)               ▼
                               Plain JS array
                               (in-memory)
                                  │
                         ┌────────┼────────┐
                         ▼        ▼        ▼
                      List     Kanban    Search
                      View     Board    Results
```

1. On mount, TanStack Query fetches `GET /api/beads` (returns all beads)
2. Response is stored as a plain array in memory
3. All views query using `Array.filter()`, `.sort()`, `.find()`
4. Full-text search uses case-insensitive substring matching across all
   text fields (id, title, description, acceptance, labels, owner, spec-id)
5. Mutations (`POST`, `PUT`) call the server API, then invalidate the
   TanStack Query cache → re-fetch
6. Polling keeps the local copy fresh (30s interval)

### Query Implementation

```typescript
// Status filtering
beads.filter(b => b.status === status)
  .sort((a, b) => a.priority - b.priority || a.created_at.localeCompare(b.created_at))

// Full-text search
const q = query.toLowerCase()
beads.filter(b => [b.id, b.title, b.description, b.acceptance, b.labels.join(' ')]
  .join(' ').toLowerCase().includes(q))

// Ready beads (open, all deps closed)
beads.filter(b => b.status === 'open' &&
  (deps.get(b.id) ?? []).every(depId => beadIndex.get(depId)?.status === 'closed'))

// Transitive dependencies (BFS)
function queryDependencies(beadId: string): Bead[] {
  const visited = new Set<string>()
  const queue = [...(depsMap.get(beadId) ?? [])]
  const result: Bead[] = []
  while (queue.length > 0) {
    const id = queue.shift()!
    if (visited.has(id)) continue
    visited.add(id)
    const b = beadIndex.get(id)
    if (b) { result.push(b); queue.push(...(depsMap.get(id) ?? [])) }
  }
  return result
}
```

### Why Plain Arrays Over Alternatives

| Option | Verdict | Reason |
|--------|---------|--------|
| **Plain arrays + MiniSearch** | **Chosen** | Arrays for filtering/sorting, MiniSearch (~18KB) for full-text search with prefix and fuzzy matching. Scales to 1,000+ beads. |
| **SQLite WASM (sql.js)** | Rejected (v2) | 660KB WASM, MIME type issues, fatal crashes, deployment fragility |
| **IndexedDB** | Rejected | Async API adds complexity, no benefit for ephemeral cache |
| **PGlite (Postgres WASM)** | Rejected | 3MB+, massive overkill |

### Execution Evidence Join

Execution runs linked to beads are fetched on demand (not preloaded):

```
Click bead detail → fetch GET /api/exec/runs?artifact=<bead-id>
```

These are loaded into separate TanStack Query caches because
they're per-bead detail data, not list data.

## Consequences

- **No server-side search needed** — the server's `GET /api/beads` is a
  simple dump. No query parameters, no pagination, no search index.
- **Instant UI interactions** — filter/search/sort are sub-millisecond.
- **Minimal bundle weight** — MiniSearch adds ~18KB (vs 660KB for sql.js).
- **No deployment complexity** — no WASM files to serve, no MIME types
  to configure, no build pipeline steps.
- **Stale data window** — between polling intervals, the local copy may be
  behind. Mutations trigger immediate re-fetch to minimize this.
- **Scale limit** — MiniSearch handles 10,000+ documents efficiently. Array
  filtering is sub-millisecond at 5,000 items. Beyond that, consider
  server-side filtering. Known deployments already exceed 1,000 beads.
