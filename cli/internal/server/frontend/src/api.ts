const BASE = '/api'

async function fetchJSON<T>(path: string): Promise<T> {
  const res = await fetch(`${BASE}${path}`)
  if (!res.ok) throw new Error(`${res.status} ${res.statusText}`)
  return res.json()
}

export async function fetchText(path: string): Promise<string> {
  const res = await fetch(`${BASE}${path}`)
  if (!res.ok) throw new Error(`${res.status} ${res.statusText}`)
  return res.text()
}

export const api = {
  documents: () => fetchJSON<any[]>('/documents'),
  documentContent: (path: string) => fetchText(`/documents/${path}`),
  search: (q: string) => fetchJSON<any[]>(`/search?q=${encodeURIComponent(q)}`),
  beads: () => fetchJSON<any[]>('/beads'),
  beadDetail: (id: string) => fetchJSON<any>(`/beads/${id}`),
  beadsStatus: () => fetchJSON<any>('/beads/status'),
  beadsReady: () => fetchJSON<any[]>('/beads/ready'),
  beadDepTree: (id: string) => fetchJSON<any>(`/beads/dep/tree/${id}`),
  docGraph: () => fetchJSON<any[]>('/docs/graph'),
  docStale: () => fetchJSON<any[]>('/docs/stale'),
  docShow: (id: string) => fetchJSON<any>(`/docs/${id}`),
  agentSessions: (harness?: string) =>
    fetchJSON<any[]>(`/agent/sessions${harness ? `?harness=${harness}` : ''}`),
  agentSessionDetail: (id: string) => fetchJSON<any>(`/agent/sessions/${id}`),
  health: () => fetchJSON<any>('/health'),
}
