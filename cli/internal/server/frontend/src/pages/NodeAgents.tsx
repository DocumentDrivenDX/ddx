import { useState, useEffect } from 'react'
import { useParams, useNavigate } from 'react-router-dom'
import { api } from '../api'
import { useFetch } from '../hooks/useFetch'
import type { ProjectEntry, WorkerRecord } from '../types'

function stateBadge(state: string) {
  const colors: Record<string, string> = {
    running: 'bg-green-100 text-green-700',
    exited: 'bg-gray-100 text-gray-600',
    failed: 'bg-red-100 text-red-700',
  }
  const cls = colors[state] ?? 'bg-gray-100 text-gray-600'
  return (
    <span className={`inline-block px-1.5 py-0.5 rounded text-xs font-medium ${cls}`}>
      {state}
    </span>
  )
}

function fmtTime(ts?: string): string {
  if (!ts) return '-'
  return new Date(ts).toLocaleString()
}

// Maps project_root path → { id, name }
type ProjectLookup = Record<string, { id: string; name: string }>

function buildProjectLookup(projects: ProjectEntry[]): ProjectLookup {
  const m: ProjectLookup = {}
  for (const p of projects) {
    m[p.path] = { id: p.id, name: p.name }
  }
  return m
}

export default function NodeAgents() {
  const { nodeId } = useParams<{ nodeId: string }>()
  const navigate = useNavigate()
  const [tick, setTick] = useState(0)
  const [projectLookup, setProjectLookup] = useState<ProjectLookup>({})

  // Auto-refresh every 3 seconds
  useEffect(() => {
    const id = setInterval(() => setTick((t) => t + 1), 3000)
    return () => clearInterval(id)
  }, [])

  // Load projects once to build path→{id,name} map
  useEffect(() => {
    api.projects()
      .then((ps) => setProjectLookup(buildProjectLookup(ps)))
      .catch(() => {})
  }, [])

  const workers = useFetch(() => api.agentWorkers(), [tick])
  const workerList = (workers.data ?? []) as WorkerRecord[]

  function projectName(w: WorkerRecord): string {
    const proj = projectLookup[w.project_root]
    if (proj) return proj.name
    // Fallback: last path segment
    return w.project_root.split('/').filter(Boolean).pop() ?? w.project_root
  }

  function projectId(w: WorkerRecord): string | null {
    return projectLookup[w.project_root]?.id ?? null
  }

  function handleRowClick(w: WorkerRecord) {
    const pid = projectId(w)
    if (pid && nodeId) {
      navigate(`/nodes/${nodeId}/projects/${pid}/agents/${w.id}`)
    }
  }

  return (
    <div className="h-full flex flex-col">
      <div className="flex items-center justify-between mb-4">
        <h1 className="text-xl font-bold">All Agents</h1>
        <span className="text-xs text-gray-400">auto-refresh 3s</span>
      </div>

      {workers.loading && !workers.data && (
        <div className="text-gray-400 text-sm">Loading...</div>
      )}
      {workers.error && (
        <div className="text-red-500 text-sm">Error: {workers.error}</div>
      )}

      <div className="flex-1 overflow-auto">
        <table className="w-full text-sm">
          <thead className="bg-gray-50 sticky top-0">
            <tr>
              <th className="text-left px-3 py-2 font-medium text-gray-500">Project</th>
              <th className="text-left px-3 py-2 font-medium text-gray-500">Worker ID</th>
              <th className="text-left px-3 py-2 font-medium text-gray-500">Bead</th>
              <th className="text-left px-3 py-2 font-medium text-gray-500">Harness</th>
              <th className="text-left px-3 py-2 font-medium text-gray-500">Model</th>
              <th className="text-left px-3 py-2 font-medium text-gray-500">State</th>
              <th className="text-left px-3 py-2 font-medium text-gray-500">Started At</th>
              <th className="text-right px-3 py-2 font-medium text-gray-500">Attempts</th>
              <th className="text-left px-3 py-2 font-medium text-gray-500">Last Status</th>
            </tr>
          </thead>
          <tbody>
            {workerList.map((w: WorkerRecord) => {
              const bead = w.current_attempt?.bead_id ?? w.current_bead ?? '-'
              const lastStatus = w.last_result?.status ?? w.status ?? '-'
              const pid = projectId(w)
              return (
                <tr
                  key={w.id}
                  onClick={() => handleRowClick(w)}
                  className="border-t cursor-pointer hover:bg-gray-50"
                  data-project-id={pid ?? ''}
                  data-worker-id={w.id}
                >
                  <td className="px-3 py-2 text-gray-700" data-testid="project-col">{projectName(w)}</td>
                  <td className="px-3 py-2 font-mono text-xs">{w.id.slice(0, 12)}</td>
                  <td className="px-3 py-2 font-mono text-xs text-gray-600">{bead}</td>
                  <td className="px-3 py-2 text-gray-600">{w.harness ?? '-'}</td>
                  <td className="px-3 py-2 text-gray-500 text-xs">{w.model ?? '-'}</td>
                  <td className="px-3 py-2">{stateBadge(w.state)}</td>
                  <td className="px-3 py-2 text-gray-500 text-xs">{fmtTime(w.started_at)}</td>
                  <td className="px-3 py-2 text-right tabular-nums">{w.attempts ?? 0}</td>
                  <td className="px-3 py-2 text-gray-500 text-xs">{lastStatus}</td>
                </tr>
              )
            })}
          </tbody>
        </table>
        {!workers.loading && workerList.length === 0 && (
          <div className="text-gray-400 text-center mt-8" data-testid="empty-state">
            No workers running.
          </div>
        )}
      </div>
    </div>
  )
}
