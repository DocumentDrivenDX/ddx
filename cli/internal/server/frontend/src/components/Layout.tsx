import { NavLink } from 'react-router-dom'
import type { ReactNode } from 'react'

const links = [
  { to: '/', label: 'Dashboard' },
  { to: '/documents', label: 'Documents' },
  { to: '/beads', label: 'Beads' },
  { to: '/graph', label: 'Graph' },
  { to: '/agent', label: 'Agent' },
  { to: '/workers', label: 'Workers' },
  { to: '/personas', label: 'Personas' },
]

export default function Layout({ children }: { children: ReactNode }) {
  return (
    <div className="flex h-screen">
      <nav className="w-48 bg-gray-900 text-gray-200 flex flex-col p-4 space-y-1 shrink-0">
        <div className="text-lg font-bold text-white mb-4">DDx</div>
        {links.map((l) => (
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
      </nav>
      <main className="flex-1 overflow-auto p-6">{children}</main>
    </div>
  )
}
