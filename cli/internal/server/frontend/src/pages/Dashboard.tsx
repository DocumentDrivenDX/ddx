import { Link } from 'react-router-dom'
import { api } from '../api'
import { useFetch } from '../hooks/useFetch'

export default function Dashboard() {
  const health = useFetch(() => api.health(), [])
  const status = useFetch(() => api.beadsStatus(), [])
  const stale = useFetch(() => api.docStale(), [])
  const docs = useFetch(() => api.documents(), [])

  return (
    <div>
      <h1 className="text-2xl font-bold mb-6">Dashboard</h1>
      <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-4 gap-4 mb-8">
        <Card title="Documents" loading={docs.loading}>
          <span className="text-3xl font-bold">{docs.data?.length ?? 0}</span>
          <Link to="/documents" className="text-blue-600 text-sm block mt-1">Browse &rarr;</Link>
        </Card>
        <Card title="Beads" loading={status.loading}>
          {status.data && (
            <div className="space-y-1 text-sm">
              <div>Ready: <b>{status.data.ready}</b></div>
              <div>In Progress: <b>{status.data.in_progress}</b></div>
              <div>Open: <b>{status.data.open}</b></div>
              <div>Closed: <b>{status.data.closed}</b></div>
            </div>
          )}
          <Link to="/beads" className="text-blue-600 text-sm block mt-1">View board &rarr;</Link>
        </Card>
        <Card title="Stale Docs" loading={stale.loading}>
          <span className="text-3xl font-bold">{stale.data?.length ?? 0}</span>
          <Link to="/graph" className="text-blue-600 text-sm block mt-1">View graph &rarr;</Link>
        </Card>
        <Card title="Server" loading={health.loading}>
          {health.data && (
            <div className="text-sm">
              <div>Status: <b className="text-green-600">{health.data.status}</b></div>
              <div className="text-gray-500 text-xs mt-1">Started: {health.data.started_at}</div>
            </div>
          )}
        </Card>
      </div>
    </div>
  )
}

function Card({ title, loading, children }: { title: string; loading: boolean; children: React.ReactNode }) {
  return (
    <div className="bg-white rounded-lg shadow p-4 border border-gray-200">
      <h2 className="text-sm font-medium text-gray-500 mb-2">{title}</h2>
      {loading ? <div className="text-gray-400 text-sm">Loading...</div> : children}
    </div>
  )
}
