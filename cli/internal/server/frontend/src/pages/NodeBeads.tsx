import { useState, useEffect, useCallback } from 'react'
import { useParams, useSearchParams } from 'react-router-dom'
import { api } from '../api'
import { useFetch } from '../hooks/useFetch'
import type { BeadWithProject, ProjectEntry } from '../types'

const STATUS_OPTIONS = ['open', 'in_progress', 'closed'] as const
type StatusOption = typeof STATUS_OPTIONS[number]

function statusBadge(status: string) {
  const colors: Record<string, string> = {
    open: 'bg-blue-100 text-blue-700',
    in_progress: 'bg-yellow-100 text-yellow-700',
    closed: 'bg-gray-100 text-gray-500',
  }
  const cls = colors[status] ?? 'bg-gray-100 text-gray-500'
  return (
    <span className={`inline-block px-1.5 py-0.5 rounded text-xs font-medium ${cls}`}>
      {status.replace('_', ' ')}
    </span>
  )
}

export default function NodeBeads() {
  useParams<{ nodeId: string }>()
  const [searchParams, setSearchParams] = useSearchParams()
  const [tick, setTick] = useState(0)
  const [projectMap, setProjectMap] = useState<Record<string, string>>({})

  // Decode filter state from URL
  const statusFilter = searchParams.get('status') ?? ''
  const projectFilter = searchParams.get('project_id') ?? ''

  // Auto-refresh every 10 seconds
  useEffect(() => {
    const id = setInterval(() => setTick((t) => t + 1), 10000)
    return () => clearInterval(id)
  }, [])

  // Build project ID → name map
  useEffect(() => {
    api.projects()
      .then((ps: ProjectEntry[]) => {
        const m: Record<string, string> = {}
        for (const p of ps) m[p.id] = p.name
        setProjectMap(m)
      })
      .catch(() => {})
  }, [])

  const fetchBeads = useCallback(
    () => api.allBeads({
      status: statusFilter || undefined,
      project_id: projectFilter || undefined,
    }),
    [tick, statusFilter, projectFilter], // eslint-disable-line react-hooks/exhaustive-deps
  )

  const result = useFetch(fetchBeads, [tick, statusFilter, projectFilter])
  const beads = (result.data ?? []) as BeadWithProject[]

  // Derive available projects from current (unfiltered) bead list for filter chips
  const [allProjects, setAllProjects] = useState<ProjectEntry[]>([])
  useEffect(() => {
    api.projects().then(setAllProjects).catch(() => {})
  }, [])

  function setStatus(s: string) {
    setSearchParams((prev) => {
      const next = new URLSearchParams(prev)
      if (s) next.set('status', s)
      else next.delete('status')
      return next
    })
  }

  function toggleProject(id: string) {
    setSearchParams((prev) => {
      const next = new URLSearchParams(prev)
      if (projectFilter === id) next.delete('project_id')
      else next.set('project_id', id)
      return next
    })
  }

  function projectName(id: string): string {
    return projectMap[id] ?? id.slice(0, 8)
  }

  return (
    <div className="h-full flex flex-col">
      <div className="flex items-center justify-between mb-3">
        <h1 className="text-xl font-bold">All Beads</h1>
        <span className="text-xs text-gray-400">auto-refresh 10s</span>
      </div>

      {/* Status filter chips */}
      <div className="flex gap-2 mb-2 flex-wrap">
        <button
          onClick={() => setStatus('')}
          data-testid="chip-status-all"
          className={`px-3 py-1 rounded-full text-xs font-medium border ${
            statusFilter === '' ? 'bg-gray-800 text-white border-gray-800' : 'border-gray-300 text-gray-600 hover:bg-gray-50'
          }`}
        >
          All statuses
        </button>
        {STATUS_OPTIONS.map((s) => (
          <button
            key={s}
            onClick={() => setStatus(statusFilter === s ? '' : s)}
            data-testid={`chip-status-${s}`}
            className={`px-3 py-1 rounded-full text-xs font-medium border ${
              statusFilter === s ? 'bg-gray-800 text-white border-gray-800' : 'border-gray-300 text-gray-600 hover:bg-gray-50'
            }`}
          >
            {s.replace('_', ' ')}
          </button>
        ))}
      </div>

      {/* Project filter chips */}
      {allProjects.length > 1 && (
        <div className="flex gap-2 mb-3 flex-wrap">
          {allProjects.map((p) => (
            <button
              key={p.id}
              onClick={() => toggleProject(p.id)}
              data-testid={`chip-project-${p.id}`}
              className={`px-3 py-1 rounded-full text-xs font-medium border ${
                projectFilter === p.id ? 'bg-indigo-600 text-white border-indigo-600' : 'border-gray-300 text-gray-600 hover:bg-gray-50'
              }`}
            >
              {p.name}
            </button>
          ))}
        </div>
      )}

      {result.loading && !result.data && (
        <div className="text-gray-400 text-sm">Loading...</div>
      )}
      {result.error && (
        <div className="text-red-500 text-sm">Error: {result.error}</div>
      )}

      <div className="flex-1 overflow-auto">
        <table className="w-full text-sm">
          <thead className="bg-gray-50 sticky top-0">
            <tr>
              <th className="text-left px-3 py-2 font-medium text-gray-500">Project</th>
              <th className="text-left px-3 py-2 font-medium text-gray-500">ID</th>
              <th className="text-left px-3 py-2 font-medium text-gray-500">Title</th>
              <th className="text-left px-3 py-2 font-medium text-gray-500">Status</th>
              <th className="text-left px-3 py-2 font-medium text-gray-500">Labels</th>
              <th className="text-right px-3 py-2 font-medium text-gray-500">Priority</th>
            </tr>
          </thead>
          <tbody>
            {beads.map((b: BeadWithProject) => (
              <tr
                key={`${b.project_id}:${b.id}`}
                className="border-t hover:bg-gray-50"
                data-testid="bead-row"
                data-project-id={b.project_id}
                data-status={b.status}
              >
                <td className="px-3 py-2 text-gray-700" data-testid="project-col">
                  {projectName(b.project_id)}
                </td>
                <td className="px-3 py-2 font-mono text-xs text-gray-500">{b.id}</td>
                <td className="px-3 py-2 text-gray-800">{b.title}</td>
                <td className="px-3 py-2">{statusBadge(b.status)}</td>
                <td className="px-3 py-2 text-xs text-gray-500">
                  {(b.labels ?? []).join(', ')}
                </td>
                <td className="px-3 py-2 text-right tabular-nums text-gray-500">{b.priority}</td>
              </tr>
            ))}
          </tbody>
        </table>
        {!result.loading && beads.length === 0 && (
          <div className="text-gray-400 text-center mt-8" data-testid="empty-state">
            No beads found.
          </div>
        )}
      </div>
    </div>
  )
}
