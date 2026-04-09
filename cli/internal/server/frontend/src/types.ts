export interface DocEntry {
  name: string
  type: string
  path: string
}

export interface DocDetail {
  id: string
  path: string
  title?: string
  depends_on?: string[]
  dependents?: string[]
  review?: {
    self_hash: string
    reviewed_at: string
    upstream_hashes?: Record<string, string>
  }
}

export interface Bead {
  id: string
  title: string
  status: string
  priority: number
  issue_type: string
  labels?: string[]
  owner?: string
  description?: string
  acceptance?: string
  dependencies?: { depends_on_id: string; type: string }[]
  created_at: string
  updated_at: string
}

export interface BeadStatus {
  total: number
  open: number
  in_progress: number
  closed: number
  ready: number
  blocked: number
}

export interface GraphNode {
  id: string
  path: string
  title?: string
  depends_on?: string[]
  dependents?: string[]
}

export interface StaleEntry {
  id: string
  path: string
  reasons: string[]
}

export interface SessionEntry {
  id: string
  timestamp: string
  harness: string
  model?: string
  prompt_len: number
  prompt?: string
  prompt_source?: string
  response?: string
  stderr?: string
  tokens?: number
  duration_ms: number
  exit_code: number
  error?: string
  correlation?: Record<string, string>
}
