import { useState, useCallback, useEffect } from 'react'
import { api } from '../api'
import { useBeadSync, useBeadsByStatus, useBeadSearch, useBeadDependencies, useInvalidateBeads } from '../hooks/useBeads'
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
  const [showCreateForm, setShowCreateForm] = useState(false)
  const { isLoading, error } = useBeadSync()
  const invalidate = useInvalidateBeads()

  const searchResults = useBeadSearch(searchQuery)
  const isSearching = searchQuery.trim().length > 0

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

  const handleDrop = useCallback(async (beadId: string, newStatus: string) => {
    try {
      await api.updateBead(beadId, { status: newStatus })
      invalidate()
    } catch (e) {
      console.error('Failed to update status:', e)
    }
  }, [invalidate])

  if (isLoading) return <div className="text-gray-400 p-4">Loading beads...</div>
  if (error) return <div className="text-red-500 p-4">Error loading beads</div>

  return (
    <div className="flex flex-col h-full gap-3">
      <div className="flex items-center gap-3 px-1">
        <input
          type="text"
          placeholder="Search beads (full-text)..."
          value={searchQuery}
          onChange={(e) => setSearchQuery(e.target.value)}
          className="flex-1 max-w-md px-3 py-1.5 text-sm border rounded-lg border-gray-300 focus:outline-none focus:ring-2 focus:ring-blue-400"
        />
        <button
          onClick={() => setShowCreateForm(true)}
          className="px-3 py-1.5 text-sm bg-blue-600 text-white rounded-lg hover:bg-blue-700"
        >
          + New Bead
        </button>
      </div>
      <div className="flex flex-1 gap-3 overflow-auto">
        {COLUMNS.map((col) => (
          <KanbanColumn
            key={col}
            status={col}
            beads={grouped[col]}
            selectedId={selectedId}
            onSelect={(id) => setSelectedId(id === selectedId ? null : id)}
            onDrop={handleDrop}
          />
        ))}
      </div>
      {selectedId && (
        <div className="fixed right-4 top-16 w-96 max-h-[calc(100vh-5rem)] overflow-auto z-10">
          <BeadDetail id={selectedId} onClose={() => setSelectedId(null)} onMutate={invalidate} />
        </div>
      )}
      {showCreateForm && (
        <CreateBeadModal onClose={() => setShowCreateForm(false)} onCreated={invalidate} />
      )}
    </div>
  )
}

function KanbanColumn({
  status, beads, selectedId, onSelect, onDrop,
}: {
  status: string; beads: Bead[]; selectedId: string | null;
  onSelect: (id: string) => void;
  onDrop: (id: string, status: string) => void;
}) {
  const handleDragOver = (e: React.DragEvent) => { e.preventDefault(); e.dataTransfer.dropEffect = 'move' }
  const handleDrop = (e: React.DragEvent) => {
    e.preventDefault()
    const beadId = e.dataTransfer.getData('text/plain')
    if (beadId) onDrop(beadId, status)
  }

  return (
    <div
      className="flex-1 min-w-[240px]"
      onDragOver={handleDragOver}
      onDrop={handleDrop}
    >
      <h2 className="text-sm font-bold uppercase text-gray-500 mb-2">
        {status.replace('_', ' ')} ({beads.length})
      </h2>
      <div className="space-y-2">
        {beads.map((b) => (
          <div
            key={b.id}
            draggable
            onDragStart={(e) => e.dataTransfer.setData('text/plain', b.id)}
            onClick={() => onSelect(b.id)}
            className={`cursor-grab active:cursor-grabbing w-full text-left rounded-lg border p-3 text-sm shadow-sm transition-shadow hover:shadow-md ${
              selectedId === b.id ? 'ring-2 ring-blue-400' : ''
            } ${STATUS_COLORS[status] ?? 'bg-gray-50 border-gray-200'}`}
          >
            <div className="font-medium">{b.title}</div>
            <div className="text-xs mt-1 opacity-70">
              {b.id} · P{b.priority}
              {b.labels?.length ? ` · ${b.labels.join(', ')}` : ''}
            </div>
          </div>
        ))}
        {beads.length === 0 && (
          <div className="text-xs text-gray-400 p-2 border-2 border-dashed border-gray-200 rounded-lg text-center">
            Drop beads here
          </div>
        )}
      </div>
    </div>
  )
}

function BeadDetail({ id, onClose, onMutate }: { id: string; onClose: () => void; onMutate: () => void }) {
  const { data: allBeads } = useBeadSync()
  const deps = useBeadDependencies(id)
  const [reopenReason, setReopenReason] = useState('')
  const [showReopen, setShowReopen] = useState(false)

  const b = (allBeads ?? []).find((b: Bead) => b.id === id)
  if (!b) return <div className="text-gray-400 text-sm p-4">Loading...</div>

  const handleClaim = async () => {
    await api.claimBead(id)
    onMutate()
  }
  const handleUnclaim = async () => {
    await api.unclaimBead(id)
    onMutate()
  }
  const handleClose = async () => {
    await api.updateBead(id, { status: 'closed' })
    onMutate()
  }
  const handleReopen = async () => {
    await api.reopenBead(id, reopenReason)
    setShowReopen(false)
    setReopenReason('')
    onMutate()
  }

  return (
    <div className="bg-white rounded-lg shadow-lg border border-gray-200 p-4 text-sm space-y-3">
      <div className="flex justify-between items-start">
        <h3 className="font-bold">{b.title}</h3>
        <button onClick={onClose} className="text-gray-400 hover:text-gray-600 text-lg">&times;</button>
      </div>
      <div className="text-xs text-gray-500">{b.id} · {b.status} · P{b.priority}</div>

      {/* Action buttons */}
      <div className="flex gap-2 flex-wrap">
        {b.status === 'open' && (
          <button onClick={handleClaim} className="px-2 py-1 text-xs bg-yellow-500 text-white rounded hover:bg-yellow-600">
            Claim
          </button>
        )}
        {b.status === 'in_progress' && (
          <>
            <button onClick={handleUnclaim} className="px-2 py-1 text-xs bg-gray-500 text-white rounded hover:bg-gray-600">
              Unclaim
            </button>
            <button onClick={handleClose} className="px-2 py-1 text-xs bg-green-600 text-white rounded hover:bg-green-700">
              Close
            </button>
          </>
        )}
        {b.status === 'closed' && (
          <button onClick={() => setShowReopen(true)} className="px-2 py-1 text-xs bg-blue-600 text-white rounded hover:bg-blue-700">
            Re-open
          </button>
        )}
      </div>

      {showReopen && (
        <div className="space-y-2 border-t pt-2">
          <input
            type="text"
            placeholder="Reason for re-opening..."
            value={reopenReason}
            onChange={(e) => setReopenReason(e.target.value)}
            className="w-full px-2 py-1 text-xs border rounded"
          />
          <div className="flex gap-2">
            <button onClick={handleReopen} className="px-2 py-1 text-xs bg-blue-600 text-white rounded">Confirm</button>
            <button onClick={() => setShowReopen(false)} className="px-2 py-1 text-xs bg-gray-300 rounded">Cancel</button>
          </div>
        </div>
      )}

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
          <p className="text-gray-700 whitespace-pre-wrap">{b.description}</p>
        </div>
      )}
      {b.acceptance && (
        <div>
          <div className="font-medium text-gray-600 mb-1">Acceptance</div>
          <p className="text-gray-700 whitespace-pre-wrap">{b.acceptance}</p>
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
      <ExecutionEvidence beadId={id} />
    </div>
  )
}

function ExecutionEvidence({ beadId }: { beadId: string }) {
  const [runs, setRuns] = useState<any[]>([])
  const [selectedRun, setSelectedRun] = useState<string | null>(null)
  const [runLog, setRunLog] = useState<{ stdout: string; stderr: string } | null>(null)
  const [loading, setLoading] = useState(true)

  useEffect(() => {
    setLoading(true)
    setSelectedRun(null)
    setRunLog(null)
    api.execRuns(beadId).then(setRuns).catch(() => setRuns([])).finally(() => setLoading(false))
  }, [beadId])

  useEffect(() => {
    if (!selectedRun) { setRunLog(null); return }
    api.execRunLog(selectedRun).then(setRunLog).catch(() => setRunLog(null))
  }, [selectedRun])

  if (loading) return null
  if (runs.length === 0) return null

  const statusColor = (s: string) => {
    if (s === 'success') return 'text-green-600'
    if (s === 'failed') return 'text-red-600'
    return 'text-yellow-600'
  }

  return (
    <div>
      <div className="font-medium text-gray-600 mb-1">Execution Runs ({runs.length})</div>
      <div className="space-y-1">
        {runs.map((r: any) => (
          <button
            key={r.run_id}
            onClick={() => setSelectedRun(r.run_id === selectedRun ? null : r.run_id)}
            className={`w-full text-left text-xs bg-gray-50 rounded p-1.5 hover:bg-gray-100 ${
              selectedRun === r.run_id ? 'ring-1 ring-blue-400' : ''
            }`}
          >
            <span className={statusColor(r.status)}>{r.status}</span>
            {' · '}
            {r.definition_id}
            {r.started_at && ` · ${new Date(r.started_at).toLocaleString()}`}
          </button>
        ))}
      </div>
      {selectedRun && runLog && (
        <div className="mt-2 space-y-2">
          {runLog.stdout && (
            <div>
              <div className="text-xs font-medium text-gray-500">stdout</div>
              <pre className="text-xs bg-gray-900 text-green-400 rounded p-2 overflow-auto max-h-40">{runLog.stdout}</pre>
            </div>
          )}
          {runLog.stderr && (
            <div>
              <div className="text-xs font-medium text-gray-500">stderr</div>
              <pre className="text-xs bg-gray-900 text-red-400 rounded p-2 overflow-auto max-h-40">{runLog.stderr}</pre>
            </div>
          )}
        </div>
      )}
    </div>
  )
}

function CreateBeadModal({ onClose, onCreated }: { onClose: () => void; onCreated: () => void }) {
  const [title, setTitle] = useState('')
  const [type, setType] = useState('task')
  const [priority, setPriority] = useState(2)
  const [labels, setLabels] = useState('')
  const [description, setDescription] = useState('')
  const [acceptance, setAcceptance] = useState('')
  const [error, setError] = useState('')
  const [submitting, setSubmitting] = useState(false)

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault()
    if (!title.trim()) { setError('Title is required'); return }
    setSubmitting(true)
    setError('')
    try {
      await api.createBead({
        title: title.trim(),
        type,
        priority,
        labels: labels ? labels.split(',').map((l) => l.trim()).filter(Boolean) : undefined,
        description: description || undefined,
        acceptance: acceptance || undefined,
      })
      onCreated()
      onClose()
    } catch (e: any) {
      setError(e.message || 'Failed to create bead')
    } finally {
      setSubmitting(false)
    }
  }

  return (
    <div className="fixed inset-0 bg-black/30 flex items-center justify-center z-20" onClick={onClose}>
      <form
        onClick={(e) => e.stopPropagation()}
        onSubmit={handleSubmit}
        className="bg-white rounded-lg shadow-xl p-6 w-full max-w-lg space-y-4"
      >
        <h2 className="text-lg font-bold">New Bead</h2>
        {error && <div className="text-red-500 text-sm">{error}</div>}

        <div>
          <label className="block text-sm font-medium text-gray-700 mb-1">Title *</label>
          <input
            type="text" value={title} onChange={(e) => setTitle(e.target.value)}
            className="w-full px-3 py-2 border rounded-lg text-sm" autoFocus
          />
        </div>

        <div className="flex gap-4">
          <div className="flex-1">
            <label className="block text-sm font-medium text-gray-700 mb-1">Type</label>
            <select value={type} onChange={(e) => setType(e.target.value)}
              className="w-full px-3 py-2 border rounded-lg text-sm">
              <option value="task">Task</option>
              <option value="bug">Bug</option>
              <option value="epic">Epic</option>
              <option value="chore">Chore</option>
            </select>
          </div>
          <div className="flex-1">
            <label className="block text-sm font-medium text-gray-700 mb-1">Priority</label>
            <select value={priority} onChange={(e) => setPriority(Number(e.target.value))}
              className="w-full px-3 py-2 border rounded-lg text-sm">
              <option value={0}>P0 (Highest)</option>
              <option value={1}>P1</option>
              <option value={2}>P2 (Default)</option>
              <option value={3}>P3</option>
              <option value={4}>P4 (Lowest)</option>
            </select>
          </div>
        </div>

        <div>
          <label className="block text-sm font-medium text-gray-700 mb-1">Labels (comma-separated)</label>
          <input
            type="text" value={labels} onChange={(e) => setLabels(e.target.value)}
            placeholder="helix, phase:build, area:cli"
            className="w-full px-3 py-2 border rounded-lg text-sm"
          />
        </div>

        <div>
          <label className="block text-sm font-medium text-gray-700 mb-1">Description</label>
          <textarea value={description} onChange={(e) => setDescription(e.target.value)}
            rows={3} className="w-full px-3 py-2 border rounded-lg text-sm" />
        </div>

        <div>
          <label className="block text-sm font-medium text-gray-700 mb-1">Acceptance Criteria</label>
          <textarea value={acceptance} onChange={(e) => setAcceptance(e.target.value)}
            rows={3} className="w-full px-3 py-2 border rounded-lg text-sm" />
        </div>

        <div className="flex justify-end gap-3 pt-2">
          <button type="button" onClick={onClose} className="px-4 py-2 text-sm text-gray-600 hover:text-gray-800">
            Cancel
          </button>
          <button type="submit" disabled={submitting}
            className="px-4 py-2 text-sm bg-blue-600 text-white rounded-lg hover:bg-blue-700 disabled:opacity-50">
            {submitting ? 'Creating...' : 'Create Bead'}
          </button>
        </div>
      </form>
    </div>
  )
}
