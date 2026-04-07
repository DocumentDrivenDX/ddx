import type { Bead } from '../types'

// In-memory bead store — plain JSON array with simple filtering.
// Replaces the previous sql.js/WASM implementation.

let beads: Bead[] = []
let depsMap: Map<string, string[]> = new Map() // beadId → [depId, ...]
let beadIndex: Map<string, Bead> = new Map()   // id → bead

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
  const q = query.toLowerCase()
  return beads.filter(b => {
    const hay = [
      b.id,
      b.title,
      b.description ?? '',
      b.acceptance ?? '',
      (b.labels ?? []).join(' '),
      b.owner ?? '',
      (b as any)['spec-id'] ?? (b as any).spec_id ?? '',
    ].join(' ').toLowerCase()
    return hay.includes(q)
  })
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
