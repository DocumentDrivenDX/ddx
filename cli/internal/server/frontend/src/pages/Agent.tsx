import { useState, useMemo } from 'react'
import { api } from '../api'
import { useFetch } from '../hooks/useFetch'
import type { SessionEntry } from '../types'

function TokenSummary({ sessions }: { sessions: SessionEntry[] }) {
  const summary = useMemo(() => {
    const byHarness: Record<string, { tokens: number; count: number; duration: number }> = {}
    for (const s of sessions) {
      const h = s.harness || 'unknown'
      if (!byHarness[h]) byHarness[h] = { tokens: 0, count: 0, duration: 0 }
      byHarness[h].tokens += s.tokens ?? 0
      byHarness[h].count += 1
      byHarness[h].duration += s.duration_ms ?? 0
    }
    return byHarness
  }, [sessions])

  const entries = Object.entries(summary)
  if (entries.length === 0) return null

  const totalTokens = entries.reduce((sum, [, v]) => sum + v.tokens, 0)
  const totalSessions = entries.reduce((sum, [, v]) => sum + v.count, 0)

  return (
    <div className="flex gap-4 mb-4 flex-wrap">
      <div className="bg-gray-50 rounded-lg border px-4 py-2 text-sm">
        <div className="text-gray-500 text-xs">Total</div>
        <div className="font-bold">{totalTokens.toLocaleString()} tokens</div>
        <div className="text-xs text-gray-500">{totalSessions} sessions</div>
      </div>
      {entries.map(([harness, data]) => (
        <div key={harness} className="bg-gray-50 rounded-lg border px-4 py-2 text-sm">
          <div className="text-gray-500 text-xs">{harness}</div>
          <div className="font-bold">{data.tokens.toLocaleString()} tokens</div>
          <div className="text-xs text-gray-500">
            {data.count} sessions · {(data.duration / 1000).toFixed(0)}s total
          </div>
        </div>
      ))}
    </div>
  )
}

export default function Agent() {
  const [harnessFilter, setHarnessFilter] = useState('')
  const [selectedId, setSelectedId] = useState<string | null>(null)
  const sessions = useFetch(
    () => api.agentSessions(harnessFilter || undefined),
    [harnessFilter]
  )

  const harnesses = [...new Set((sessions.data ?? []).map((s: SessionEntry) => s.harness))]

  return (
    <div className="h-full flex flex-col">
      <div className="flex items-center justify-between mb-4">
        <h1 className="text-xl font-bold">Agent Sessions</h1>
        <select
          value={harnessFilter}
          onChange={(e) => setHarnessFilter(e.target.value)}
          className="border rounded px-2 py-1 text-sm"
        >
          <option value="">All harnesses</option>
          {harnesses.map((h) => <option key={h} value={h}>{h}</option>)}
        </select>
      </div>

      {!sessions.loading && sessions.data && <TokenSummary sessions={sessions.data as SessionEntry[]} />}

      {sessions.loading && <div className="text-gray-400 text-sm">Loading...</div>}
      {sessions.error && <div className="text-red-500 text-sm">Error: {sessions.error}</div>}

      <div className="flex-1 overflow-auto">
        <table className="w-full text-sm">
          <thead className="bg-gray-50 sticky top-0">
            <tr>
              <th className="text-left px-3 py-2 font-medium text-gray-500">Time</th>
              <th className="text-left px-3 py-2 font-medium text-gray-500">ID</th>
              <th className="text-left px-3 py-2 font-medium text-gray-500">Harness</th>
              <th className="text-left px-3 py-2 font-medium text-gray-500">Model</th>
              <th className="text-right px-3 py-2 font-medium text-gray-500">Tokens</th>
              <th className="text-right px-3 py-2 font-medium text-gray-500">Duration</th>
              <th className="text-center px-3 py-2 font-medium text-gray-500">Exit</th>
            </tr>
          </thead>
          <tbody>
            {(sessions.data ?? []).map((s: SessionEntry) => (
              <tr
                key={s.id}
                onClick={() => setSelectedId(s.id === selectedId ? null : s.id)}
                className={`border-t cursor-pointer ${
                  selectedId === s.id ? 'bg-blue-50' : 'hover:bg-gray-50'
                }`}
              >
                <td className="px-3 py-2 text-gray-500">{new Date(s.timestamp).toLocaleString()}</td>
                <td className="px-3 py-2 font-mono text-xs">{s.id}</td>
                <td className="px-3 py-2">{s.harness}</td>
                <td className="px-3 py-2 text-gray-500">{s.model || '-'}</td>
                <td className="px-3 py-2 text-right">{s.tokens ?? '-'}</td>
                <td className="px-3 py-2 text-right">{(s.duration_ms / 1000).toFixed(1)}s</td>
                <td className="px-3 py-2 text-center">
                  <span className={`inline-block px-1.5 py-0.5 rounded text-xs font-medium ${
                    s.exit_code === 0 ? 'bg-green-100 text-green-700' : 'bg-red-100 text-red-700'
                  }`}>
                    {s.exit_code}
                  </span>
                </td>
              </tr>
            ))}
          </tbody>
        </table>
        {!sessions.loading && (sessions.data ?? []).length === 0 && (
          <div className="text-gray-400 text-center mt-8">No agent sessions recorded.</div>
        )}
      </div>

      {selectedId && <SessionDetail id={selectedId} />}
    </div>
  )
}

function SessionDetail({ id }: { id: string }) {
  const detail = useFetch(() => api.agentSessionDetail(id), [id])

  if (detail.loading) return <div className="mt-4 text-gray-400 text-sm">Loading detail...</div>
  if (detail.error) return <div className="mt-4 text-red-500 text-sm">Error: {detail.error}</div>

  const s = detail.data as SessionEntry
  return (
    <div className="mt-4 bg-white rounded-lg border border-gray-200 shadow p-4 text-sm">
      <h3 className="font-bold mb-2">Session {s.id}</h3>
      <div className="grid grid-cols-2 gap-2 text-gray-600">
        <div>Harness: <b>{s.harness}</b></div>
        <div>Model: <b>{s.model || '-'}</b></div>
        <div>Tokens: <b>{s.tokens ?? 0}</b></div>
        <div>Duration: <b>{(s.duration_ms / 1000).toFixed(1)}s</b></div>
        <div>Prompt Length: <b>{s.prompt_len}</b></div>
        <div>Exit Code: <b>{s.exit_code}</b></div>
      </div>
      {s.correlation && Object.keys(s.correlation).length > 0 && (
        <div className="mt-3 text-gray-600">
          <div className="font-medium mb-1">Correlation</div>
          <pre className="bg-gray-50 rounded p-2 overflow-x-auto">{JSON.stringify(s.correlation, null, 2)}</pre>
        </div>
      )}
      {s.prompt && (
        <div className="mt-3">
          <div className="font-medium mb-1">Prompt</div>
          <pre className="bg-gray-50 rounded p-2 overflow-x-auto whitespace-pre-wrap">{s.prompt}</pre>
        </div>
      )}
      {s.response && (
        <div className="mt-3">
          <div className="font-medium mb-1">Response</div>
          <pre className="bg-gray-50 rounded p-2 overflow-x-auto whitespace-pre-wrap">{s.response}</pre>
        </div>
      )}
      {s.stderr && (
        <div className="mt-3">
          <div className="font-medium mb-1">Stderr</div>
          <pre className="bg-red-50 text-red-700 rounded p-2 overflow-x-auto whitespace-pre-wrap">{s.stderr}</pre>
        </div>
      )}
      {s.error && (
        <div className="mt-2 text-red-600 bg-red-50 rounded p-2">
          <b>Error:</b> {s.error}
        </div>
      )}
    </div>
  )
}
