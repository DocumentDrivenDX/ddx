import { useState } from 'react'
import { useFetch } from '../hooks/useFetch'
import { api } from '../api'

interface PersonaSummary {
  name: string
  description: string
  roles: string[]
  tags: string[]
}

export default function Personas() {
  const { data: personas, loading } = useFetch<PersonaSummary[]>(() => api.personas())
  const [selected, setSelected] = useState<string | null>(null)
  const [content, setContent] = useState<string>('')
  const [loadingContent, setLoadingContent] = useState(false)

  const loadContent = async (name: string) => {
    setSelected(name)
    setLoadingContent(true)
    try {
      // Resolve persona content via the role endpoint (use first role or name)
      const persona = personas?.find(p => p.name === name)
      const role = persona?.roles?.[0] || name
      const data = await api.personaDetail(role)
      setContent(data.content || 'No content available')
    } catch {
      setContent('Could not load persona content')
    }
    setLoadingContent(false)
  }

  if (loading) return <div className="p-6 text-gray-500">Loading personas...</div>
  if (!personas?.length) return <div className="p-6 text-gray-500">No personas found</div>

  return (
    <div className="flex h-full">
      <div className="w-80 border-r overflow-y-auto p-4">
        <h2 className="text-lg font-bold mb-4">Personas</h2>
        {personas.map(p => (
          <button
            key={p.name}
            onClick={() => loadContent(p.name)}
            className={`w-full text-left p-3 rounded-lg mb-2 border transition ${
              selected === p.name
                ? 'border-blue-500 bg-blue-50'
                : 'border-gray-200 hover:border-gray-300'
            }`}
          >
            <div className="font-medium text-sm">{p.name}</div>
            {p.description && (
              <div className="text-xs text-gray-500 mt-1">{p.description}</div>
            )}
            {p.roles?.length > 0 && (
              <div className="flex gap-1 mt-2 flex-wrap">
                {p.roles.map(r => (
                  <span key={r} className="text-xs bg-blue-100 text-blue-700 px-1.5 py-0.5 rounded">
                    {r}
                  </span>
                ))}
              </div>
            )}
            {p.tags?.length > 0 && (
              <div className="flex gap-1 mt-1 flex-wrap">
                {p.tags.map(t => (
                  <span key={t} className="text-xs bg-gray-100 text-gray-600 px-1.5 py-0.5 rounded">
                    {t}
                  </span>
                ))}
              </div>
            )}
          </button>
        ))}
      </div>
      <div className="flex-1 overflow-y-auto p-6">
        {selected ? (
          loadingContent ? (
            <div className="text-gray-500">Loading...</div>
          ) : (
            <div>
              <h2 className="text-xl font-bold mb-4">{selected}</h2>
              <pre className="whitespace-pre-wrap text-sm bg-gray-50 border rounded-lg p-4 font-mono">
                {content}
              </pre>
            </div>
          )
        ) : (
          <div className="text-gray-400 text-center mt-20">
            Select a persona to view its content
          </div>
        )}
      </div>
    </div>
  )
}
