<script lang="ts">
	import * as d3 from 'd3'

	interface GraphNode {
		id: string
		path: string
		title: string
		dependsOn: string[]
		dependents: string[]
	}

	interface GraphLink {
		source: string
		target: string
	}

	let { nodes, links }: { nodes: GraphNode[]; links: GraphLink[] } = $props()

	let svgEl = $state<SVGSVGElement | undefined>(undefined)
	let tooltipNode = $state<GraphNode | null>(null)
	let tooltipX = $state(0)
	let tooltipY = $state(0)

	$effect(() => {
		const el = svgEl
		if (!el) return

		// Access props inside effect so changes trigger re-run
		const currentNodes = nodes
		const currentLinks = links

		d3.select(el).selectAll('*').remove()

		const width = el.clientWidth || 800
		const height = el.clientHeight || 600

		const svg = d3.select(el)

		// Arrow marker for directed edges
		svg
			.append('defs')
			.append('marker')
			.attr('id', 'ddx-arrow')
			.attr('viewBox', '0 -5 10 10')
			.attr('refX', 24)
			.attr('refY', 0)
			.attr('markerWidth', 6)
			.attr('markerHeight', 6)
			.attr('orient', 'auto')
			.append('path')
			.attr('d', 'M0,-5L10,0L0,5')
			.attr('class', 'fill-border-line dark:fill-dark-border-line')

		// Container for pan/zoom
		const g = svg.append('g')

		const zoom = d3
			.zoom<SVGSVGElement, unknown>()
			.scaleExtent([0.05, 8])
			.on('zoom', (event) => {
				g.attr('transform', event.transform.toString())
			})

		svg.call(zoom)

		// Clone nodes/links for simulation mutation
		type SimNode = GraphNode & d3.SimulationNodeDatum
		const simNodes: SimNode[] = currentNodes.map((n) => ({ ...n }))
		const nodeById = new Map(simNodes.map((n) => [n.id, n]))

		const simLinks = currentLinks
			.filter((l) => nodeById.has(l.source) && nodeById.has(l.target))
			.map((l) => ({ source: l.source, target: l.target }))

		const simulation = d3
			.forceSimulation(simNodes)
			.force(
				'link',
				d3
					.forceLink<SimNode, (typeof simLinks)[0]>(simLinks)
					.id((d) => d.id)
					.distance(130)
					.strength(0.5)
			)
			.force('charge', d3.forceManyBody().strength(-450))
			.force('center', d3.forceCenter(width / 2, height / 2))
			.force('collide', d3.forceCollide(38))

		// Links
		const linkSel = g
			.append('g')
			.selectAll<SVGLineElement, (typeof simLinks)[0]>('line')
			.data(simLinks)
			.join('line')
			.attr('class', 'stroke-border-line dark:stroke-dark-border-line')
			.attr('stroke-width', 1.5)
			.attr('stroke-opacity', 0.7)
			.attr('marker-end', 'url(#ddx-arrow)')

		// Node groups
		const nodeGroup = g
			.append('g')
			.selectAll<SVGGElement, SimNode>('g')
			.data(simNodes)
			.join('g')
			.style('cursor', 'grab')

		nodeGroup
			.append('circle')
			.attr('r', 14)
			.attr('class', (d: SimNode) => {
				if (d.dependsOn.length === 0)
					return 'fill-accent-load stroke-accent-fulcrum dark:fill-dark-accent-load dark:stroke-dark-accent-fulcrum'
				return 'fill-accent-lever stroke-accent-fulcrum dark:fill-dark-accent-lever dark:stroke-dark-accent-fulcrum'
			})
			.attr('stroke-width', 1.5)

		nodeGroup
			.append('text')
			.attr('x', 18)
			.attr('dy', '0.35em')
			.attr('font-size', '11px')
			.attr('class', 'fill-fg-muted dark:fill-dark-fg-muted')
			.attr('pointer-events', 'none')
			.text((d) => (d.title.length > 28 ? d.title.slice(0, 28) + '\u2026' : d.title))

		// Drag
		const drag = d3
			.drag<SVGGElement, SimNode>()
			.on('start', (event, d) => {
				if (!event.active) simulation.alphaTarget(0.3).restart()
				d.fx = d.x
				d.fy = d.y
			})
			.on('drag', (event, d) => {
				d.fx = event.x
				d.fy = event.y
			})
			.on('end', (event, d) => {
				if (!event.active) simulation.alphaTarget(0)
				d.fx = null
				d.fy = null
			})

		nodeGroup.call(drag)

		// Tooltip on hover
		nodeGroup
			.on('mouseenter', (event: MouseEvent, d) => {
				const rect = el.getBoundingClientRect()
				tooltipNode = d
				tooltipX = event.clientX - rect.left + 14
				tooltipY = event.clientY - rect.top - 10
			})
			.on('mousemove', (event: MouseEvent) => {
				const rect = el.getBoundingClientRect()
				tooltipX = event.clientX - rect.left + 14
				tooltipY = event.clientY - rect.top - 10
			})
			.on('mouseleave', () => {
				tooltipNode = null
			})

		// Tick
		simulation.on('tick', () => {
			linkSel
				.attr('x1', (d: any) => d.source.x ?? 0)
				.attr('y1', (d: any) => d.source.y ?? 0)
				.attr('x2', (d: any) => d.target.x ?? 0)
				.attr('y2', (d: any) => d.target.y ?? 0)

			nodeGroup.attr('transform', (d) => `translate(${d.x ?? 0},${d.y ?? 0})`)
		})

		return () => {
			simulation.stop()
			tooltipNode = null
		}
	})
</script>

<div class="relative h-full w-full bg-bg-canvas dark:bg-dark-bg-canvas">
	<svg
		bind:this={svgEl}
		data-testid="doc-graph-svg"
		class="h-full w-full text-fg-ink dark:text-dark-fg-ink"
	/>

	{#if tooltipNode}
		<div
			class="pointer-events-none absolute z-10 max-w-xs rounded-none border border-border-line bg-bg-elevated p-3 text-sm shadow-lg shadow-fg-ink/15 dark:border-dark-border-line dark:bg-dark-bg-elevated dark:shadow-black/30"
			style="left: {tooltipX}px; top: {tooltipY}px;"
		>
			<div class="font-semibold text-fg-ink dark:text-dark-fg-ink">{tooltipNode.title}</div>
			<div class="mt-1 break-all font-mono-code text-mono-code text-fg-muted dark:text-dark-fg-muted">
				{tooltipNode.path}
			</div>
			<div class="mt-2 flex gap-3 text-xs text-fg-muted dark:text-dark-fg-muted">
				<span>{tooltipNode.dependsOn.length} deps out</span>
				<span>{tooltipNode.dependents.length} deps in</span>
			</div>
		</div>
	{/if}
</div>
