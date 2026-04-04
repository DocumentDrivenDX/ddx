import { useRef, useEffect, useState } from 'react'
import * as d3 from 'd3'
import { api } from '../api'
import { useFetch } from '../hooks/useFetch'
import type { GraphNode, StaleEntry } from '../types'

interface D3Node extends d3.SimulationNodeDatum {
  id: string
  path: string
  title?: string
  stale: boolean
}

interface D3Link extends d3.SimulationLinkDatum<D3Node> {
  source: string | D3Node
  target: string | D3Node
}

export default function Graph() {
  const svgRef = useRef<SVGSVGElement>(null)
  const graph = useFetch(() => api.docGraph(), [])
  const stale = useFetch(() => api.docStale(), [])
  const [hovered, setHovered] = useState<string | null>(null)

  useEffect(() => {
    if (!graph.data || !svgRef.current) return

    const staleIds = new Set((stale.data ?? []).map((s: StaleEntry) => s.id))
    const nodes: D3Node[] = (graph.data as GraphNode[]).map((n) => ({
      id: n.id,
      path: n.path,
      title: n.title,
      stale: staleIds.has(n.id),
    }))

    const nodeIds = new Set(nodes.map((n) => n.id))
    const links: D3Link[] = []
    for (const n of graph.data as GraphNode[]) {
      for (const dep of n.depends_on ?? []) {
        if (nodeIds.has(dep)) {
          links.push({ source: dep, target: n.id })
        }
      }
    }

    const svg = d3.select(svgRef.current)
    svg.selectAll('*').remove()

    const width = svgRef.current.clientWidth
    const height = svgRef.current.clientHeight

    const g = svg.append('g')

    const zoom = d3.zoom<SVGSVGElement, unknown>()
      .scaleExtent([0.2, 4])
      .on('zoom', (event) => g.attr('transform', event.transform))
    svg.call(zoom)

    const sim = d3.forceSimulation(nodes)
      .force('link', d3.forceLink<D3Node, D3Link>(links).id((d) => d.id).distance(100))
      .force('charge', d3.forceManyBody().strength(-300))
      .force('center', d3.forceCenter(width / 2, height / 2))

    const link = g.append('g')
      .selectAll('line')
      .data(links)
      .join('line')
      .attr('stroke', '#cbd5e1')
      .attr('stroke-width', 1.5)
      .attr('marker-end', 'url(#arrow)')

    svg.append('defs').append('marker')
      .attr('id', 'arrow')
      .attr('viewBox', '0 -5 10 10')
      .attr('refX', 20)
      .attr('refY', 0)
      .attr('markerWidth', 6)
      .attr('markerHeight', 6)
      .attr('orient', 'auto')
      .append('path')
      .attr('d', 'M0,-5L10,0L0,5')
      .attr('fill', '#94a3b8')

    const node = g.append('g')
      .selectAll<SVGCircleElement, D3Node>('circle')
      .data(nodes)
      .join('circle')
      .attr('r', 8)
      .attr('fill', (d) => d.stale ? '#f97316' : '#3b82f6')
      .attr('stroke', '#fff')
      .attr('stroke-width', 2)
      .attr('cursor', 'pointer')
      .on('mouseover', (_, d) => setHovered(d.id))
      .on('mouseout', () => setHovered(null))
      .call(d3.drag<SVGCircleElement, D3Node>()
        .on('start', (event, d) => { if (!event.active) sim.alphaTarget(0.3).restart(); d.fx = d.x; d.fy = d.y })
        .on('drag', (event, d) => { d.fx = event.x; d.fy = event.y })
        .on('end', (event, d) => { if (!event.active) sim.alphaTarget(0); d.fx = null; d.fy = null })
      )

    const label = g.append('g')
      .selectAll('text')
      .data(nodes)
      .join('text')
      .text((d) => d.title || d.id)
      .attr('font-size', 10)
      .attr('dx', 12)
      .attr('dy', 4)
      .attr('fill', '#374151')

    sim.on('tick', () => {
      link
        .attr('x1', (d: any) => d.source.x)
        .attr('y1', (d: any) => d.source.y)
        .attr('x2', (d: any) => d.target.x)
        .attr('y2', (d: any) => d.target.y)
      node.attr('cx', (d) => d.x!).attr('cy', (d) => d.y!)
      label.attr('x', (d) => d.x!).attr('y', (d) => d.y!)
    })

    return () => { sim.stop() }
  }, [graph.data, stale.data])

  if (graph.loading) return <div className="text-gray-400">Loading graph...</div>
  if (graph.error) return <div className="text-red-500">Error: {graph.error}</div>

  return (
    <div className="h-full flex flex-col">
      <div className="flex items-center justify-between mb-3">
        <h1 className="text-xl font-bold">Document Dependency Graph</h1>
        <div className="flex items-center gap-4 text-xs text-gray-500">
          <span className="flex items-center gap-1">
            <span className="w-3 h-3 rounded-full bg-blue-500 inline-block" /> Fresh
          </span>
          <span className="flex items-center gap-1">
            <span className="w-3 h-3 rounded-full bg-orange-500 inline-block" /> Stale
          </span>
        </div>
      </div>
      {hovered && (
        <div className="text-sm text-gray-600 mb-2">
          <b>{hovered}</b>
        </div>
      )}
      <div className="flex-1 bg-white rounded-lg shadow border border-gray-200 overflow-hidden">
        <svg ref={svgRef} className="w-full h-full" />
      </div>
    </div>
  )
}
