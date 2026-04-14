import { useState, useEffect, useRef } from 'react'
import { api } from '../api'
import { useFetch } from '../hooks/useFetch'
import type { WorkerRecord } from '../types'

function stateBadge(state: string) {
  const colors: Record<string, string> = {
    running: 'bg-green-100 text-green-700',
    exited: 'bg-gray-100 text-gray-600',
    failed: 'bg-red-100 text-red-700',
  }
  const cls = colors[state] ?? 'bg-gray-100 text-gray-600'
  return (
    <span className={`inline-block px-1.5 py-0.5 rounded text-xs font-medium ${cls}`}>
      {state}
    </span>
  )
}

function elapsed(startedAt?: string, finishedAt?: string): string {
  if (!startedAt) return '-'
  const start = new Date(startedAt).getTime()
  const end = finishedAt ? new Date(finishedAt).getTime() : Date.now()
  const ms = end - start
  const s = Math.floor(ms / 1000)
  if (s < 60) return `${s}s`
  const m = Math.floor(s / 60)
  const rem = s % 60
  return `${m}m${rem}s`
}

function shortRef(rev?: string) {
  if (!rev) return null
  return rev.length > 8 ? rev.slice(0, 8) : rev
}

// ---- Worker list with auto-refresh ----

export default function Workers() {
  const [selectedId, setSelectedId] = useState<string | null>(null)
  const [tick, setTick] = useState(0)

  // Auto-refresh every 2 seconds
  useEffect(() => {
    const id = setInterval(() => setTick((t) => t + 1), 2000)
    return () => clearInterval(id)
  }, [])

  const workers = useFetch(() => api.agentWorkers(), [tick])

  return (
    <div className="h-full flex flex-col">
      <div className="flex items-center justify-between mb-4">
        <h1 className="text-xl font-bold">Workers</h1>
        <span className="text-xs text-gray-400">auto-refresh 2s</span>
      </div>

      {workers.loading && !workers.data && (
        <div className="text-gray-400 text-sm">Loading...</div>
      )}
      {workers.error && (
        <div className="text-red-500 text-sm">Error: {workers.error}</div>
      )}

      <div className="flex-1 overflow-auto">
        <table className="w-full text-sm">
          <thead className="bg-gray-50 sticky top-0">
            <tr>
              <th className="text-left px-3 py-2 font-medium text-gray-500">ID</th>
              <th className="text-left px-3 py-2 font-medium text-gray-500">State</th>
              <th className="text-left px-3 py-2 font-medium text-gray-500">Bead</th>
              <th className="text-left px-3 py-2 font-medium text-gray-500">Harness</th>
              <th className="text-left px-3 py-2 font-medium text-gray-500">Model</th>
              <th className="text-left px-3 py-2 font-medium text-gray-500">Phase</th>
              <th className="text-right px-3 py-2 font-medium text-gray-500">Elapsed</th>
              <th className="text-right px-3 py-2 font-medium text-gray-500">Attempts</th>
            </tr>
          </thead>
          <tbody>
            {(workers.data ?? []).map((w: WorkerRecord) => {
              const phase = w.current_attempt?.phase ?? w.last_attempt?.phase ?? '-'
              const bead = w.current_attempt?.bead_id ?? w.current_bead ?? '-'
              return (
                <tr
                  key={w.id}
                  onClick={() => setSelectedId(w.id === selectedId ? null : w.id)}
                  className={`border-t cursor-pointer ${
                    selectedId === w.id ? 'bg-blue-50' : 'hover:bg-gray-50'
                  }`}
                >
                  <td className="px-3 py-2 font-mono text-xs">{w.id.slice(0, 12)}</td>
                  <td className="px-3 py-2">{stateBadge(w.state)}</td>
                  <td className="px-3 py-2 font-mono text-xs text-gray-600">{bead}</td>
                  <td className="px-3 py-2 text-gray-600">{w.harness ?? '-'}</td>
                  <td className="px-3 py-2 text-gray-500 text-xs">{w.model ?? '-'}</td>
                  <td className="px-3 py-2 text-gray-500">{phase}</td>
                  <td className="px-3 py-2 text-right tabular-nums">
                    {elapsed(w.started_at, w.state === 'running' ? undefined : w.finished_at)}
                  </td>
                  <td className="px-3 py-2 text-right tabular-nums">{w.attempts ?? 0}</td>
                </tr>
              )
            })}
          </tbody>
        </table>
        {!workers.loading && (workers.data ?? []).length === 0 && (
          <div className="text-gray-400 text-center mt-8">No workers found.</div>
        )}
      </div>

      {selectedId && (
        <WorkerDetail key={selectedId} id={selectedId} />
      )}
    </div>
  )
}

// ---- Worker detail panels ----

function WorkerDetail({ id }: { id: string }) {
  const [tab, setTab] = useState<'prompt' | 'log' | 'utilization'>('log')
  const [tick, setTick] = useState(0)

  useEffect(() => {
    const tid = setInterval(() => setTick((t) => t + 1), 2000)
    return () => clearInterval(tid)
  }, [])

  const detail = useFetch(() => api.agentWorkerDetail(id), [id, tick])
  const w = detail.data as WorkerRecord | null

  return (
    <div className="mt-4 bg-white rounded-lg border border-gray-200 shadow text-sm" data-testid="worker-detail">
      <div className="flex items-center gap-2 px-4 py-3 border-b bg-gray-50 rounded-t-lg">
        <span className="font-mono font-medium text-xs">{id}</span>
        {w && stateBadge(w.state)}
        <div className="ml-auto flex gap-1">
          {(['log', 'prompt', 'utilization'] as const).map((t) => (
            <button
              key={t}
              onClick={() => setTab(t)}
              className={`px-3 py-1 rounded text-xs font-medium capitalize ${
                tab === t ? 'bg-gray-800 text-white' : 'bg-gray-100 hover:bg-gray-200 text-gray-700'
              }`}
            >
              {t}
            </button>
          ))}
        </div>
      </div>

      {detail.loading && !detail.data && (
        <div className="p-4 text-gray-400">Loading...</div>
      )}
      {detail.error && (
        <div className="p-4 text-red-500">Error: {detail.error}</div>
      )}

      {w && tab === 'log' && <LogPanel id={id} tick={tick} />}
      {w && tab === 'prompt' && <PromptPanel id={id} />}
      {w && tab === 'utilization' && <UtilizationPanel worker={w} />}
    </div>
  )
}

// ---- Log panel ----

function LogPanel({ id, tick }: { id: string; tick: number }) {
  const log = useFetch(() => api.agentWorkerLog(id), [id, tick])
  const bottomRef = useRef<HTMLDivElement>(null)

  useEffect(() => {
    bottomRef.current?.scrollIntoView({ behavior: 'smooth' })
  }, [log.data])

  const text = log.data ? (log.data.stdout || '') : ''

  return (
    <div className="p-0" data-testid="log-panel">
      {log.loading && !log.data && (
        <div className="p-4 text-gray-400 text-xs">Loading log...</div>
      )}
      {log.error && (
        <div className="p-4 text-red-500 text-xs">Error: {log.error}</div>
      )}
      {!log.loading && !text && (
        <div className="p-4 text-gray-400 text-xs">No log output yet.</div>
      )}
      {text && (
        <pre className="p-4 overflow-auto max-h-80 text-xs font-mono whitespace-pre-wrap bg-gray-900 text-green-300 rounded-b-lg">
          {text}
          <div ref={bottomRef} />
        </pre>
      )}
    </div>
  )
}

// ---- Prompt panel ----

function PromptPanel({ id }: { id: string }) {
  const prompt = useFetch(() => api.agentWorkerPrompt(id), [id])

  return (
    <div className="p-0" data-testid="prompt-panel">
      {prompt.loading && (
        <div className="p-4 text-gray-400 text-xs">Loading prompt...</div>
      )}
      {prompt.error && (
        <div className="p-4 text-red-500 text-xs">
          Prompt not available: {prompt.error}
        </div>
      )}
      {prompt.data && (
        <pre className="p-4 overflow-auto max-h-80 text-xs font-mono whitespace-pre-wrap bg-gray-50 rounded-b-lg">
          {prompt.data as string}
        </pre>
      )}
    </div>
  )
}

// ---- Utilization panel ----

function UtilizationPanel({ worker: w }: { worker: WorkerRecord }) {
  const r = w.last_result
  const attempt = w.current_attempt ?? w.last_attempt

  return (
    <div className="p-4" data-testid="utilization-panel">
      <div className="grid grid-cols-2 gap-x-8 gap-y-2 text-sm">
        <Stat label="State" value={w.state} />
        <Stat label="Started" value={w.started_at ? new Date(w.started_at).toLocaleString() : '-'} />
        <Stat label="Elapsed" value={elapsed(w.started_at, w.state === 'running' ? undefined : w.finished_at)} />
        <Stat label="Poll interval" value={w.poll_interval ?? '-'} />
        <Stat label="Attempts" value={String(w.attempts ?? 0)} />
        <Stat label="Successes" value={String(w.successes ?? 0)} />
        <Stat label="Failures" value={String(w.failures ?? 0)} />
        <Stat label="Harness" value={w.harness ?? '-'} />
        <Stat label="Model" value={w.model ?? '-'} />
        <Stat label="Effort" value={w.effort ?? '-'} />
      </div>

      {attempt && (
        <div className="mt-4">
          <div className="font-medium text-xs text-gray-500 uppercase mb-2">
            {w.current_attempt ? 'Current Attempt' : 'Last Attempt'}
          </div>
          <div className="grid grid-cols-2 gap-x-8 gap-y-2 text-sm">
            <Stat label="Attempt ID" value={attempt.attempt_id} mono />
            <Stat label="Bead" value={attempt.bead_id} mono />
            {w.current_attempt?.bead_title && <Stat label="Title" value={w.current_attempt.bead_title} />}
            <Stat label="Phase" value={attempt.phase} />
            <Stat label="Elapsed" value={`${Math.round((attempt.elapsed_ms ?? 0) / 1000)}s`} />
          </div>
        </div>
      )}

      {r && (
        <div className="mt-4">
          <div className="font-medium text-xs text-gray-500 uppercase mb-2">Last Result</div>
          <div className="grid grid-cols-2 gap-x-8 gap-y-2 text-sm">
            <Stat label="Status" value={r.status ?? '-'} />
            {r.detail && <Stat label="Detail" value={r.detail} />}
            {r.base_rev && <Stat label="Base rev" value={shortRef(r.base_rev) ?? '-'} mono />}
            {r.result_rev && <Stat label="Result rev" value={shortRef(r.result_rev) ?? '-'} mono />}
            {r.session_id && <Stat label="Session" value={shortRef(r.session_id) ?? '-'} mono />}
          </div>
        </div>
      )}

      {w.last_error && (
        <div className="mt-4 rounded bg-red-50 border border-red-200 p-3 text-xs text-red-700">
          <span className="font-medium">Last error:</span> {w.last_error}
        </div>
      )}
    </div>
  )
}

function Stat({ label, value, mono }: { label: string; value: string; mono?: boolean }) {
  return (
    <div>
      <span className="text-gray-500">{label}: </span>
      <span className={mono ? 'font-mono text-xs' : 'font-medium'}>{value}</span>
    </div>
  )
}
