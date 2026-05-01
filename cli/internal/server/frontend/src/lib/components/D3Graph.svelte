<script lang="ts">
	import * as d3 from 'd3'
	import { ZoomIn, ZoomOut, Maximize2 } from 'lucide-svelte'

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

	let {
		nodes,
		links,
		onNodeClick
	}: {
		nodes: GraphNode[]
		links: GraphLink[]
		onNodeClick?: (node: GraphNode) => void
	} = $props()

	let svgEl = $state<SVGSVGElement | undefined>(undefined)
	let tooltipNode = $state<GraphNode | null>(null)
	let tooltipX = $state(0)
	let tooltipY = $state(0)

	// Zoom behavior reference — set inside $effect, used by button handlers
	let zoom: d3.ZoomBehavior<SVGSVGElement, unknown> | null = null

	function zoomIn() {
		if (!svgEl || !zoom) return
		d3.select(svgEl).transition().duration(250).call(zoom.scaleBy, 1.4)
	}

	function zoomOut() {
		if (!svgEl || !zoom) return
		d3.select(svgEl).transition().duration(250).call(zoom.scaleBy, 1 / 1.4)
	}

	function resetZoom() {
		if (!svgEl || !zoom) return
		d3.select(svgEl).transition().duration(350).call(zoom.transform, d3.zoomIdentity)
	}

	$effect(() => {
		const el = svgEl
		if (!el) return

		const currentNodes = nodes
		const currentLinks = links

		d3.select(el).selectAll('*').remove()

		const width = el.clientWidth || 960
		const height = el.clientHeight || 700

		const svg = d3.select(el)

		svg
			.append('defs')
			.append('marker')
			.attr('id', 'ddx-arrow')
			.attr('viewBox', '0 -5 10 10')
			.attr('refX', 28)
			.attr('refY', 0)
			.attr('markerWidth', 6)
			.attr('markerHeight', 6)
			.attr('orient', 'auto')
			.append('path')
			.attr('d', 'M0,-5L10,0L0,5')
			.attr('class', 'fill-border-line dark:fill-dark-border-line')

		const g = svg.append('g')

		zoom = d3
			.zoom<SVGSVGElement, unknown>()
			.scaleExtent([0.05, 10])
			.on('zoom', (event) => {
				g.attr('transform', event.transform.toString())
			})

		svg.call(zoom)

		// Double-click to reset zoom
		svg.on('dblclick.zoom', null)

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
					.distance(160)
					.strength(0.4)
			)
			.force('charge', d3.forceManyBody().strength(-600))
			.force('center', d3.forceCenter(width / 2, height / 2))
			.force('collide', d3.forceCollide(48))

		const linkSel = g
			.append('g')
			.selectAll<SVGLineElement, (typeof simLinks)[0]>('line')
			.data(simLinks)
			.join('line')
			.attr('class', 'stroke-border-line dark:stroke-dark-border-line')
			.attr('stroke-width', 1.5)
			.attr('stroke-opacity', 0.6)
			.attr('marker-end', 'url(#ddx-arrow)')

		const nodeGroup = g
			.append('g')
			.selectAll<SVGGElement, SimNode>('g')
			.data(simNodes)
			.join('g')
			.style('cursor', onNodeClick ? 'pointer' : 'grab')

		nodeGroup
			.append('circle')
			.attr('r', 18)
			.attr('class', (d: SimNode) => {
				if (d.dependsOn.length === 0)
					return 'fill-accent-load stroke-accent-fulcrum dark:fill-dark-accent-load dark:stroke-dark-accent-fulcrum'
				return 'fill-accent-lever stroke-accent-fulcrum dark:fill-dark-accent-lever dark:stroke-dark-accent-fulcrum'
			})
			.attr('stroke-width', 2)

		nodeGroup
			.append('text')
			.attr('x', 24)
			.attr('dy', '0.35em')
			.attr('font-size', '12px')
			.attr('class', 'fill-fg-muted dark:fill-dark-fg-muted select-none')
			.attr('pointer-events', 'none')
			.text((d) => (d.title.length > 32 ? d.title.slice(0, 32) + '…' : d.title))

		// Drag with distance tracking to distinguish click from drag
		let dragDistance = 0

		const drag = d3
			.drag<SVGGElement, SimNode>()
			.on('start', (event, d) => {
				dragDistance = 0
				if (!event.active) simulation.alphaTarget(0.3).restart()
				d.fx = d.x
				d.fy = d.y
			})
			.on('drag', (event, d) => {
				dragDistance += Math.abs(event.dx) + Math.abs(event.dy)
				d.fx = event.x
				d.fy = event.y
			})
			.on('end', (event, d) => {
				if (!event.active) simulation.alphaTarget(0)
				d.fx = null
				d.fy = null
			})

		nodeGroup.call(drag)

		// Click to navigate — only fire if the node wasn't dragged
		if (onNodeClick) {
			nodeGroup.on('click', (_event: MouseEvent, d: SimNode) => {
				if (dragDistance > 4) return
				onNodeClick(d)
			})

			nodeGroup
				.on('mouseenter', function () {
					d3.select(this).select('circle').attr('stroke-width', 3).attr('r', 20)
				})
				.on('mouseleave', function () {
					d3.select(this).select('circle').attr('stroke-width', 2).attr('r', 18)
					tooltipNode = null
				})
		}

		// Tooltip on hover
		nodeGroup
			.on('mouseenter.tooltip', (event: MouseEvent, d) => {
				const rect = el.getBoundingClientRect()
				tooltipNode = d
				tooltipX = event.clientX - rect.left + 16
				tooltipY = event.clientY - rect.top - 12
			})
			.on('mousemove.tooltip', (event: MouseEvent) => {
				const rect = el.getBoundingClientRect()
				tooltipX = event.clientX - rect.left + 16
				tooltipY = event.clientY - rect.top - 12
			})
			.on('mouseleave.tooltip', () => {
				tooltipNode = null
			})

		simulation.on('tick', () => {
			linkSel
				.attr('x1', (d: any) => d.source.x ?? 0)
				.attr('y1', (d: any) => d.source.y ?? 0)
				.attr('x2', (d: any) => d.target.x ?? 0)
				.attr('y2', (d: any) => d.target.y ?? 0)

			nodeGroup.attr('transform', (d) => `translate(${d.x ?? 0},${d.y ?? 0})`)
		})

		// Fit to bounds after simulation settles
		simulation.on('end', () => {
			if (!el || !zoom) return
			const bounds = (g.node() as SVGGElement | null)?.getBBox()
			if (!bounds || bounds.width === 0) return
			const w = el.clientWidth || width
			const h = el.clientHeight || height
			const scale = Math.min(0.9, 0.9 / Math.max(bounds.width / w, bounds.height / h))
			const tx = w / 2 - scale * (bounds.x + bounds.width / 2)
			const ty = h / 2 - scale * (bounds.y + bounds.height / 2)
			d3.select(el)
				.transition()
				.duration(400)
				.call(zoom.transform, d3.zoomIdentity.translate(tx, ty).scale(scale))
		})

		return () => {
			simulation.stop()
			tooltipNode = null
			zoom = null
		}
	})
</script>

<div class="relative h-full w-full bg-bg-canvas dark:bg-dark-bg-canvas">
	<svg
		bind:this={svgEl}
		data-testid="doc-graph-svg"
		class="h-full w-full text-fg-ink dark:text-dark-fg-ink"
	/>

	<!-- Zoom controls -->
	<div class="absolute right-3 top-3 flex flex-col gap-1">
		<button
			onclick={zoomIn}
			aria-label="Zoom in"
			class="flex h-8 w-8 items-center justify-center border border-border-line bg-bg-elevated text-fg-muted shadow-sm hover:bg-bg-surface hover:text-fg-ink dark:border-dark-border-line dark:bg-dark-bg-elevated dark:text-dark-fg-muted dark:hover:bg-dark-bg-surface dark:hover:text-dark-fg-ink"
		>
			<ZoomIn class="h-4 w-4" />
		</button>
		<button
			onclick={zoomOut}
			aria-label="Zoom out"
			class="flex h-8 w-8 items-center justify-center border border-border-line bg-bg-elevated text-fg-muted shadow-sm hover:bg-bg-surface hover:text-fg-ink dark:border-dark-border-line dark:bg-dark-bg-elevated dark:text-dark-fg-muted dark:hover:bg-dark-bg-surface dark:hover:text-dark-fg-ink"
		>
			<ZoomOut class="h-4 w-4" />
		</button>
		<button
			onclick={resetZoom}
			aria-label="Fit to view"
			class="flex h-8 w-8 items-center justify-center border border-border-line bg-bg-elevated text-fg-muted shadow-sm hover:bg-bg-surface hover:text-fg-ink dark:border-dark-border-line dark:bg-dark-bg-elevated dark:text-dark-fg-muted dark:hover:bg-dark-bg-surface dark:hover:text-dark-fg-ink"
		>
			<Maximize2 class="h-4 w-4" />
		</button>
	</div>

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
			{#if tooltipNode && typeof onNodeClick === 'function'}
				<div class="mt-2 text-xs text-accent-lever dark:text-dark-accent-lever">Click to open</div>
			{/if}
		</div>
	{/if}
</div>
