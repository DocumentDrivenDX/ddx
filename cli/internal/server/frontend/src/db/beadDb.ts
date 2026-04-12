import MiniSearch from 'minisearch'
import type { Bead } from '../types'

// In-memory bead store — plain JSON array with MiniSearch for full-text search.
// Handles 1,000+ beads efficiently (~28KB library, no WASM).

let beads: Bead[] = []
let depsMap: Map<string, string[]> = new Map() // beadId → [depId, ...]
let beadIndex: Map<string, Bead> = new Map()   // id → bead
let searchIndex: MiniSearch<Bead> | null = null

export async function initDb(): Promise<void> {
  // No-op — kept for API compatibility with useBeadSync
}

export async function loadBeads(incoming: Bead[]): Promise<void> {
  beads = incoming
  beadIndex = new Map(incoming.map(b => [b.id, b]))
  depsMap = new Map()
  for (const b of incoming) {
    if (b.dependencies?.length) {
      depsMap.set(b.id, b.dependencies.map(d => d.depends_on_id))
    }
  }

  // Rebuild search index
  searchIndex = new MiniSearch<Bead>({
    fields: ['id', 'title', 'description', 'acceptance', '_labels', 'owner', '_specId'],
    storeFields: ['id'],
    extractField: (doc, field) => {
      if (field === '_labels') return (doc.labels ?? []).join(' ')
      if (field === '_specId') return (doc as any)['spec-id'] ?? (doc as any).spec_id ?? ''
      return (doc as any)[field] ?? ''
    },
    searchOptions: { prefix: true, fuzzy: 0.2 },
  })
  searchIndex.addAll(incoming)
}

export function queryBeadsByStatus(status: string): Bead[] {
  return beads
    .filter(b => b.status === status)
    .sort((a, b) => (a.priority ?? 2) - (b.priority ?? 2) || a.created_at.localeCompare(b.created_at))
}

export function queryAllBeads(): Bead[] {
  return [...beads].sort((a, b) => (a.priority ?? 2) - (b.priority ?? 2) || a.created_at.localeCompare(b.created_at))
}

export function searchBeads(query: string): Bead[] {
  if (!query.trim()) return queryAllBeads()
  if (!searchIndex) return queryAllBeads()

  const results = searchIndex.search(query, { prefix: true, fuzzy: 0.2 })
  const ids = new Set(results.map(r => r.id))
  return beads.filter(b => ids.has(b.id))
}

export function queryReadyBeads(): Bead[] {
  return beads
    .filter(b => {
      if (b.status !== 'open') return false
      const deps = depsMap.get(b.id) ?? []
      return deps.every(depId => {
        const dep = beadIndex.get(depId)
        return dep && dep.status === 'closed'
      })
    })
    .sort((a, b) => (a.priority ?? 2) - (b.priority ?? 2) || a.created_at.localeCompare(b.created_at))
}

export function queryDependencies(beadId: string): Bead[] {
  // Transitive dependencies via BFS
  const visited = new Set<string>()
  const queue = [...(depsMap.get(beadId) ?? [])]
  const result: Bead[] = []
  while (queue.length > 0) {
    const id = queue.shift()!
    if (visited.has(id)) continue
    visited.add(id)
    const b = beadIndex.get(id)
    if (b) {
      result.push(b)
      for (const next of depsMap.get(id) ?? []) {
        if (!visited.has(next)) queue.push(next)
      }
    }
  }
  return result
}

export function queryStatusCounts(): Record<string, number> {
  const counts: Record<string, number> = {}
  for (const b of beads) {
    counts[b.status] = (counts[b.status] ?? 0) + 1
  }
  return counts
}
