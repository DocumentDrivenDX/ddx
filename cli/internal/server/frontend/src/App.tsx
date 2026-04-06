import { Routes, Route, Navigate } from 'react-router-dom'
import Layout from './components/Layout'
import Dashboard from './pages/Dashboard'
import Documents from './pages/Documents'
import Beads from './pages/Beads'
import Graph from './pages/Graph'
import Agent from './pages/Agent'
import Personas from './pages/Personas'

export default function App() {
  return (
    <Layout>
      <Routes>
        <Route path="/" element={<Dashboard />} />
        <Route path="/documents" element={<Documents />} />
        <Route path="/documents/*" element={<Documents />} />
        <Route path="/beads" element={<Beads />} />
        <Route path="/graph" element={<Graph />} />
        <Route path="/agent" element={<Agent />} />
        <Route path="/personas" element={<Personas />} />
        <Route path="*" element={<Navigate to="/" replace />} />
      </Routes>
    </Layout>
  )
}
