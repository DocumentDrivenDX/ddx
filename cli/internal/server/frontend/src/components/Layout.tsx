import { useState, useEffect } from 'react'
import { NavLink, useNavigate } from 'react-router-dom'
import type { ReactNode } from 'react'
import { api } from '../api'
import type { ProjectEntry } from '../types'

const staticLinks = [
  { to: '/', label: 'Dashboard' },
  { to: '/documents', label: 'Documents' },
  { to: '/beads', label: 'Beads' },
  { to: '/graph', label: 'Graph' },
  { to: '/agent', label: 'Agent' },
  { to: '/workers', label: 'Workers' },
  { to: '/personas', label: 'Personas' },
]

export default function Layout({ children }: { children: ReactNode }) {
  const navigate = useNavigate()
  const [nodeId, setNodeId] = useState<string | null>(null)
  const [projects, setProjects] = useState<ProjectEntry[]>([])

  useEffect(() => {
    api.node().then((n) => setNodeId(n.id)).catch(() => {})
    api.projects().then((ps) => setProjects(ps)).catch(() => {})
  }, [])

  function handleProjectChange(e: React.ChangeEvent<HTMLSelectElement>) {
    const value = e.target.value
    if (value === '__all__' && nodeId) {
      navigate(`/nodes/${nodeId}/agents`)
    } else if (value === '__all_beads__' && nodeId) {
      navigate(`/nodes/${nodeId}/beads`)
    }
  }

  return (
    <div className="flex h-screen">
      <nav className="w-48 bg-gray-900 text-gray-200 flex flex-col p-4 space-y-1 shrink-0">
        <div className="text-lg font-bold text-white mb-4">DDx</div>
        {staticLinks.map((l) => (
          <NavLink
            key={l.to}
            to={l.to}
            end={l.to === '/'}
            className={({ isActive }) =>
              `px-3 py-2 rounded text-sm ${isActive ? 'bg-gray-700 text-white' : 'hover:bg-gray-800'}`
            }
          >
            {l.label}
          </NavLink>
        ))}
        {nodeId && (
          <NavLink
            to={`/nodes/${nodeId}/agents`}
            className={({ isActive }) =>
              `px-3 py-2 rounded text-sm ${isActive ? 'bg-gray-700 text-white' : 'hover:bg-gray-800'}`
            }
          >
            All Agents
          </NavLink>
        )}
        {nodeId && (
          <NavLink
            to={`/nodes/${nodeId}/beads`}
            className={({ isActive }) =>
              `px-3 py-2 rounded text-sm ${isActive ? 'bg-gray-700 text-white' : 'hover:bg-gray-800'}`
            }
          >
            All Beads
          </NavLink>
        )}
        {projects.length > 0 && nodeId && (
          <div className="mt-4 pt-4 border-t border-gray-700">
            <label className="text-xs text-gray-400 uppercase tracking-wide mb-1 block">Project</label>
            <select
              onChange={handleProjectChange}
              defaultValue=""
              className="w-full bg-gray-800 text-gray-200 text-xs rounded px-2 py-1 border border-gray-700"
              data-testid="project-picker"
            >
              <option value="">— pick project —</option>
              <option value="__all__">All Agents</option>
              <option value="__all_beads__">All Beads</option>
              {projects.map((p) => (
                <option key={p.id} value={p.id}>{p.name}</option>
              ))}
            </select>
          </div>
        )}
      </nav>
      <main className="flex-1 overflow-auto p-6">{children}</main>
    </div>
  )
}
