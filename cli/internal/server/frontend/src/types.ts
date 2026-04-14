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

export interface AgentSessionSummary {
  id: string
  timestamp: string
  harness: string
  surface?: string
  canonical_target?: string
  model?: string
  prompt_len: number
  prompt_source?: string
  correlation?: Record<string, string>
  native_session_id?: string
  native_log_ref?: string
  trace_id?: string
  span_id?: string
  stderr?: string
  tokens?: number
  input_tokens?: number
  output_tokens?: number
  cost_usd?: number
  duration_ms: number
  exit_code: number
  error?: string
  total_tokens?: number
  base_rev?: string
  result_rev?: string
}

export interface AgentSessionDetail extends AgentSessionSummary {
  prompt_available: boolean
  response_available: boolean
  prompt?: string
  response?: string
}

export interface CurrentAttemptInfo {
  attempt_id: string
  bead_id: string
  bead_title?: string
  harness?: string
  model?: string
  profile?: string
  phase: string
  phase_seq: number
  started_at: string
  elapsed_ms: number
}

export interface PhaseTransition {
  phase: string
  ts: string
  phase_seq: number
}

export interface LastAttemptInfo {
  attempt_id: string
  bead_id: string
  phase: string
  started_at: string
  ended_at: string
  elapsed_ms: number
}

export interface WorkerExecutionResult {
  bead_id?: string
  attempt_id?: string
  worker_id?: string
  harness?: string
  provider?: string
  model?: string
  status?: string
  detail?: string
  session_id?: string
  base_rev?: string
  result_rev?: string
  retry_after?: string
}

export interface WorkerRecord {
  id: string
  kind: string
  state: string
  status?: string
  project_root: string
  harness?: string
  provider?: string
  model?: string
  effort?: string
  once?: boolean
  poll_interval?: string
  started_at?: string
  finished_at?: string
  error?: string
  stdout_path?: string
  spec_path?: string
  attempts?: number
  successes?: number
  failures?: number
  current_bead?: string
  last_error?: string
  last_result?: WorkerExecutionResult
  current_attempt?: CurrentAttemptInfo
  recent_phases?: PhaseTransition[]
  last_attempt?: LastAttemptInfo
}
