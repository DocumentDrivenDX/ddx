import { useState } from 'react'
import { api } from '../api'
import { useFetch } from '../hooks/useFetch'
import type { Bead } from '../types'

const STATUS_COLORS: Record<string, string> = {
  open: 'bg-blue-100 text-blue-800 border-blue-200',
  in_progress: 'bg-yellow-100 text-yellow-800 border-yellow-200',
  closed: 'bg-green-100 text-green-800 border-green-200',
}

const COLUMNS = ['open', 'in_progress', 'closed']

export default function Beads() {
  const [selectedId, setSelectedId] = useState<string | null>(null)
  const beads = useFetch(() => api.beads(), [])

  const grouped: Record<string, Bead[]> = { open: [], in_progress: [], closed: [] }
  for (const b of (beads.data ?? []) as Bead[]) {
    const key = grouped[b.status] ? b.status : 'open'
    grouped[key].push(b)
  }

  return (
    <div className="flex h-full gap-4">
      <div className="flex-1 flex gap-3 overflow-auto">
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
        <div className="w-80 shrink-0">
          <BeadDetail id={selectedId} onClose={() => setSelectedId(null)} />
        </div>
      )}
    </div>
  )
}

function BeadDetail({ id, onClose }: { id: string; onClose: () => void }) {
  const bead = useFetch(() => api.beadDetail(id), [id])
  const depTree = useFetch(() => api.beadDepTree(id), [id])

  if (bead.loading) return <div className="text-gray-400 text-sm">Loading...</div>
  if (bead.error) return <div className="text-red-500 text-sm">Error: {bead.error}</div>

  const b = bead.data as Bead
  return (
    <div className="bg-white rounded-lg shadow border border-gray-200 p-4 text-sm space-y-3">
      <div className="flex justify-between items-start">
        <h3 className="font-bold">{b.title}</h3>
        <button onClick={onClose} className="text-gray-400 hover:text-gray-600">&times;</button>
      </div>
      <div className="text-xs text-gray-500">{b.id} · {b.status} · P{b.priority}</div>
      {b.labels?.length ? (
        <div className="flex gap-1 flex-wrap">
          {b.labels.map((l) => (
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
      {depTree.data?.tree && (
        <div>
          <div className="font-medium text-gray-600 mb-1">Dependencies</div>
          <pre className="text-xs bg-gray-50 rounded p-2 overflow-auto">{depTree.data.tree}</pre>
        </div>
      )}
    </div>
  )
}
