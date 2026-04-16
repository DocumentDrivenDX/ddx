import { useState } from 'react'
import ReactMarkdown from 'react-markdown'
import remarkGfm from 'remark-gfm'
import { api } from '../api'
import { useFetch } from '../hooks/useFetch'
import type { DocEntry } from '../types'

export default function Documents() {
  const [selected, setSelected] = useState<string | null>(null)
  const [search, setSearch] = useState('')
  const [typeFilter, setTypeFilter] = useState('')
  const docs = useFetch(() => api.documents(), [])

  const filtered = docs.data?.filter((d: DocEntry) => {
    if (typeFilter && d.type !== typeFilter) return false
    if (search && !d.name.toLowerCase().includes(search.toLowerCase())) return false
    return true
  }) ?? []

  const types = [...new Set(docs.data?.map((d: DocEntry) => d.type) ?? [])]

  return (
    <div className="flex h-full gap-4">
      <div className="w-72 shrink-0 flex flex-col">
        <h1 className="text-xl font-bold mb-3">Documents</h1>
        <input
          type="text"
          placeholder="Search..."
          value={search}
          onChange={(e) => setSearch(e.target.value)}
          className="border rounded px-2 py-1 text-sm mb-2"
        />
        <select
          value={typeFilter}
          onChange={(e) => setTypeFilter(e.target.value)}
          className="border rounded px-2 py-1 text-sm mb-3"
        >
          <option value="">All types</option>
          {types.map((t) => <option key={t} value={t}>{t}</option>)}
        </select>
        <div className="overflow-auto flex-1 space-y-0.5">
          {filtered.map((d: DocEntry) => (
            <button
              key={d.path}
              onClick={() => setSelected(d.path)}
              className={`w-full text-left px-2 py-1.5 rounded text-sm ${
                selected === d.path ? 'bg-blue-100 text-blue-800' : 'hover:bg-gray-100'
              }`}
            >
              <div className="font-medium truncate">{d.name}</div>
              <div className="text-xs text-gray-500">{d.type}</div>
            </button>
          ))}
          {filtered.length === 0 && !docs.loading && (
            <div className="text-sm text-gray-400 p-2">No documents found.</div>
          )}
        </div>
      </div>
      <div className="flex-1 overflow-auto bg-white rounded-lg shadow border border-gray-200 p-6">
        {selected ? (
          <DocumentViewer path={selected} />
        ) : (
          <div className="text-gray-400 text-center mt-20">Select a document to view</div>
        )}
      </div>
    </div>
  )
}

function DocumentViewer({ path }: { path: string }) {
  const [editing, setEditing] = useState(false)
  const [editContent, setEditContent] = useState('')
  const [saving, setSaving] = useState(false)
  const [saveError, setSaveError] = useState('')
  const content = useFetch(() => api.documentContent(path), [path])

  if (content.loading) return <div className="text-gray-400">Loading...</div>
  if (content.error) return <div className="text-red-500">Error: {content.error}</div>

  const startEditing = () => {
    setEditContent(content.data ?? '')
    setEditing(true)
    setSaveError('')
  }

  const handleSave = async () => {
    setSaving(true)
    setSaveError('')
    try {
      await api.saveDocument(path, editContent)
      setEditing(false)
      // Force re-fetch by updating content
      window.location.reload()
    } catch (e: any) {
      setSaveError(e.message || 'Failed to save')
    } finally {
      setSaving(false)
    }
  }

  const handleCancel = () => {
    setEditing(false)
    setSaveError('')
  }

  return (
    <div>
      <div className="flex items-center justify-between mb-4">
        <div className="text-xs text-gray-400 font-mono">{path}</div>
        {!editing ? (
          <button
            onClick={startEditing}
            className="px-3 py-1 text-xs bg-gray-100 border rounded hover:bg-gray-200"
          >
            Edit
          </button>
        ) : (
          <div className="flex gap-2">
            {saveError && <span className="text-red-500 text-xs">{saveError}</span>}
            <button
              onClick={handleCancel}
              className="px-3 py-1 text-xs bg-gray-100 border rounded hover:bg-gray-200"
            >
              Cancel
            </button>
            <button
              onClick={handleSave}
              disabled={saving}
              className="px-3 py-1 text-xs bg-blue-600 text-white rounded hover:bg-blue-700 disabled:opacity-50"
            >
              {saving ? 'Saving...' : 'Save'}
            </button>
          </div>
        )}
      </div>
      {editing ? (
        <textarea
          value={editContent}
          onChange={(e) => setEditContent(e.target.value)}
          className="w-full h-[calc(100vh-14rem)] font-mono text-sm p-4 border rounded-lg resize-none focus:outline-none focus:ring-2 focus:ring-blue-400"
        />
      ) : (
        <div className="prose prose-sm max-w-none">
          <ReactMarkdown remarkPlugins={[remarkGfm]}>{content.data ?? ''}</ReactMarkdown>
        </div>
      )}
    </div>
  )
}
