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
  const content = useFetch(() => api.documentContent(path), [path])

  if (content.loading) return <div className="text-gray-400">Loading...</div>
  if (content.error) return <div className="text-red-500">Error: {content.error}</div>

  return (
    <div className="prose prose-sm max-w-none">
      <div className="text-xs text-gray-400 mb-4 font-mono">{path}</div>
      <ReactMarkdown remarkPlugins={[remarkGfm]}>{content.data ?? ''}</ReactMarkdown>
    </div>
  )
}
