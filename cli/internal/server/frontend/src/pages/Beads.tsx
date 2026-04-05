import { useState } from 'react'
import { useBeadSync, useBeadsByStatus, useBeadSearch, useBeadDependencies } from '../hooks/useBeads'
import type { Bead } from '../types'

const STATUS_COLORS: Record<string, string> = {
  open: 'bg-blue-100 text-blue-800 border-blue-200',
  in_progress: 'bg-yellow-100 text-yellow-800 border-yellow-200',
  closed: 'bg-green-100 text-green-800 border-green-200',
}

const COLUMNS = ['open', 'in_progress', 'closed']

export default function Beads() {
  const [selectedId, setSelectedId] = useState<string | null>(null)
  const [searchQuery, setSearchQuery] = useState('')
  const { isLoading, error } = useBeadSync()

  const searchResults = useBeadSearch(searchQuery)
  const isSearching = searchQuery.trim().length > 0

  // Per-status queries from local SQLite
  const openBeads = useBeadsByStatus('open')
  const inProgressBeads = useBeadsByStatus('in_progress')
  const closedBeads = useBeadsByStatus('closed')

  const grouped: Record<string, Bead[]> = isSearching
    ? { open: [], in_progress: [], closed: [] }
    : { open: openBeads, in_progress: inProgressBeads, closed: closedBeads }

  if (isSearching) {
    for (const b of searchResults) {
      const key = grouped[b.status] ? b.status : 'open'
      grouped[key].push(b)
    }
  }

  if (isLoading) return <div className="text-gray-400 p-4">Loading beads...</div>
  if (error) return <div className="text-red-500 p-4">Error loading beads</div>

  return (
    <div className="flex flex-col h-full gap-3">
      <div className="px-1">
        <input
          type="text"
          placeholder="Search beads (full-text)..."
          value={searchQuery}
          onChange={(e) => setSearchQuery(e.target.value)}
          className="w-full max-w-md px-3 py-1.5 text-sm border rounded-lg border-gray-300 focus:outline-none focus:ring-2 focus:ring-blue-400"
        />
      </div>
      <div className="flex flex-1 gap-3 overflow-auto">
        {COLUMNS.map((col) => (
          <div key={col} className="flex-1 min-w-[240px]">
            <h2 className="text-sm font-bold uppercase text-gray-500 mb-2">
              {col.replace('_', ' ')} ({grouped[col].length})
            </h2>
            <div className="space-y-2">
              {grouped[col].map((b) => (
                <button
                  key={b.id}
                  onClick={() => setSelectedId(b.id === selectedId ? null : b.id)}
                  className={`w-full text-left rounded-lg border p-3 text-sm shadow-sm ${
                    selectedId === b.id ? 'ring-2 ring-blue-400' : ''
                  } ${STATUS_COLORS[col] ?? 'bg-gray-50 border-gray-200'}`}
                >
                  <div className="font-medium">{b.title}</div>
                  <div className="text-xs mt-1 opacity-70">
                    {b.id} · P{b.priority}
                    {b.labels?.length ? ` · ${b.labels.join(', ')}` : ''}
                  </div>
                </button>
              ))}
              {grouped[col].length === 0 && (
                <div className="text-xs text-gray-400 p-2">None</div>
              )}
            </div>
          </div>
        ))}
      </div>
      {selectedId && (
        <div className="fixed right-4 top-16 w-80 max-h-[calc(100vh-5rem)] overflow-auto">
          <BeadDetail id={selectedId} onClose={() => setSelectedId(null)} />
        </div>
      )}
    </div>
  )
}

function BeadDetail({ id, onClose }: { id: string; onClose: () => void }) {
  const { data: allBeads } = useBeadSync()
  const deps = useBeadDependencies(id)

  const b = (allBeads ?? []).find((b: Bead) => b.id === id)
  if (!b) return <div className="text-gray-400 text-sm">Loading...</div>

  return (
    <div className="bg-white rounded-lg shadow border border-gray-200 p-4 text-sm space-y-3">
      <div className="flex justify-between items-start">
        <h3 className="font-bold">{b.title}</h3>
        <button onClick={onClose} className="text-gray-400 hover:text-gray-600">&times;</button>
      </div>
      <div className="text-xs text-gray-500">{b.id} · {b.status} · P{b.priority}</div>
      {b.labels?.length ? (
        <div className="flex gap-1 flex-wrap">
          {b.labels.map((l: string) => (
            <span key={l} className="bg-gray-100 px-2 py-0.5 rounded text-xs">{l}</span>
          ))}
        </div>
      ) : null}
      {b.description && (
        <div>
          <div className="font-medium text-gray-600 mb-1">Description</div>
          <p className="text-gray-700">{b.description}</p>
        </div>
      )}
      {b.acceptance && (
        <div>
          <div className="font-medium text-gray-600 mb-1">Acceptance</div>
          <p className="text-gray-700">{b.acceptance}</p>
        </div>
      )}
      {deps.length > 0 && (
        <div>
          <div className="font-medium text-gray-600 mb-1">Dependencies ({deps.length})</div>
          <div className="space-y-1">
            {deps.map((d) => (
              <div key={d.id} className="text-xs bg-gray-50 rounded p-1.5">
                <span className={d.status === 'closed' ? 'text-green-600' : 'text-gray-600'}>
                  {d.status === 'closed' ? '✓' : '○'}
                </span>{' '}
                {d.id} — {d.title}
              </div>
            ))}
          </div>
        </div>
      )}
    </div>
  )
}
