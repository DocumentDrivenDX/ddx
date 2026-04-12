import { useQuery, useQueryClient } from '@tanstack/react-query'
import { api } from '../api'
import { loadBeads, queryBeadsByStatus, queryAllBeads, searchBeads, queryReadyBeads, queryStatusCounts, queryDependencies, initDb } from '../db/beadDb'
import type { Bead } from '../types'

const BEADS_KEY = ['beads'] as const

/** Fetch all beads and load into in-memory bead store. Refetch every 30s. */
export function useBeadSync() {
  return useQuery({
    queryKey: BEADS_KEY,
    queryFn: async () => {
      await initDb()
      const beads = await api.beads()
      await loadBeads(beads)
      return beads as Bead[]
    },
    refetchInterval: 30_000,
    staleTime: 10_000,
  })
}

/** Query beads from local bead cache by status. Depends on sync query. */
export function useBeadsByStatus(status: string): Bead[] {
  const { data } = useBeadSync()
  if (!data) return []
  return queryBeadsByStatus(status)
}

/** Full-text search across beads in local bead cache. */
export function useBeadSearch(query: string): Bead[] {
  const { data } = useBeadSync()
  if (!data) return []
  return searchBeads(query)
}

/** Get ready beads (open, all deps closed) from local bead cache. */
export function useReadyBeads(): Bead[] {
  const { data } = useBeadSync()
  if (!data) return []
  return queryReadyBeads()
}

/** Status counts from local bead cache. */
export function useBeadStatusCounts(): Record<string, number> {
  const { data } = useBeadSync()
  if (!data) return {}
  return queryStatusCounts()
}

/** Get transitive dependencies from local bead cache. */
export function useBeadDependencies(beadId: string): Bead[] {
  const { data } = useBeadSync()
  if (!data) return []
  return queryDependencies(beadId)
}

/** Invalidate bead cache to trigger re-fetch and in-memory store reload. */
export function useInvalidateBeads() {
  const queryClient = useQueryClient()
  return () => queryClient.invalidateQueries({ queryKey: BEADS_KEY })
}
