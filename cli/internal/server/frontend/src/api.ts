import type { AgentSessionDetail, AgentSessionSummary, NodeInfo, ProjectEntry, WorkerRecord } from './types'

const BASE = '/api'

async function fetchJSON<T>(path: string): Promise<T> {
  const res = await fetch(`${BASE}${path}`)
  if (!res.ok) throw new Error(`${res.status} ${res.statusText}`)
  return res.json()
}

async function postJSON<T>(path: string, body: any): Promise<T> {
  const res = await fetch(`${BASE}${path}`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(body),
  })
  if (!res.ok) throw new Error(`${res.status} ${res.statusText}`)
  return res.json()
}

async function putJSON<T>(path: string, body: any): Promise<T> {
  const res = await fetch(`${BASE}${path}`, {
    method: 'PUT',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(body),
  })
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
  createBead: (data: {
    title: string; type?: string; priority?: number;
    labels?: string[]; description?: string; acceptance?: string;
  }) => postJSON<any>('/beads', data),
  updateBead: (id: string, data: {
    status?: string; labels?: string[]; description?: string;
    acceptance?: string; priority?: number; notes?: string;
  }) => putJSON<any>(`/beads/${id}`, data),
  claimBead: (id: string, assignee?: string) =>
    postJSON<any>(`/beads/${id}/claim`, { assignee: assignee ?? '' }),
  unclaimBead: (id: string) => postJSON<any>(`/beads/${id}/unclaim`, {}),
  reopenBead: (id: string, reason: string) =>
    postJSON<any>(`/beads/${id}/reopen`, { reason }),
  beadDeps: (id: string, action: 'add' | 'remove', depId: string) =>
    postJSON<any>(`/beads/${id}/deps`, { action, dep_id: depId }),
  saveDocument: (path: string, content: string) =>
    putJSON<any>(`/documents/${path}`, { content }),
  execDefinitions: (artifact?: string) =>
    fetchJSON<any[]>(`/exec/definitions${artifact ? `?artifact=${artifact}` : ''}`),
  execRuns: (artifact?: string, definition?: string) => {
    const params = new URLSearchParams()
    if (artifact) params.set('artifact', artifact)
    if (definition) params.set('definition', definition)
    const qs = params.toString()
    return fetchJSON<any[]>(`/exec/runs${qs ? `?${qs}` : ''}`)
  },
  execRunDetail: (id: string) => fetchJSON<any>(`/exec/runs/${id}`),
  execRunLog: (id: string) => fetchJSON<any>(`/exec/runs/${id}/log`),
  execDispatch: (definitionId: string) =>
    postJSON<any>(`/exec/run/${definitionId}`, {}),
  docGraph: () => fetchJSON<any[]>('/docs/graph'),
  docStale: () => fetchJSON<any[]>('/docs/stale'),
  docShow: (id: string) => fetchJSON<any>(`/docs/${id}`),
  agentSessions: (harness?: string) =>
    fetchJSON<AgentSessionSummary[]>(`/agent/sessions${harness ? `?harness=${harness}` : ''}`),
  agentSessionDetail: (id: string) => fetchJSON<AgentSessionDetail>(`/agent/sessions/${id}`),
  agentWorkers: () => fetchJSON<WorkerRecord[]>('/agent/workers'),
  agentWorkerDetail: (id: string) => fetchJSON<WorkerRecord>(`/agent/workers/${id}`),
  agentWorkerLog: (id: string) => fetchJSON<{ stdout: string; stderr: string }>(`/agent/workers/${id}/log`),
  agentWorkerPrompt: (id: string) => fetchText(`/agent/workers/${id}/prompt`),
  personas: () => fetchJSON<any[]>('/personas'),
  personaDetail: (role: string) => fetchJSON<any>(`/personas/${role}`),
  health: () => fetchJSON<any>('/health'),
  node: () => fetchJSON<NodeInfo>('/node'),
  projects: () => fetchJSON<ProjectEntry[]>('/projects'),
}
