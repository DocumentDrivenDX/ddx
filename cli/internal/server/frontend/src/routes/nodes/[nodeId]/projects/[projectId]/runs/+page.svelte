<script lang="ts">
	import type { PageData } from './$types'
	import { page } from '$app/stores'
	import { goto } from '$app/navigation'
	import { createClient } from '$lib/gql/client'
	import { PROJECT_RUNS_QUERY, PAGE_SIZE } from './+page'
	import type { RunEdge, RunConnection, PageInfo } from './+page'

	let { data }: { data: PageData } = $props()

	let appendedEdges = $state<RunEdge[]>([])
	let appendedPageInfo = $state<PageInfo | null>(null)
	let loadingMore = $state(false)
	let harnessInput = $state(data.activeHarness ?? '')

	let filterKey = $derived(`${data.activeLayer}::${data.activeStatus}::${data.activeHarness}`)
	let prevFilterKey = $state('')
	$effect(() => {
		if (filterKey !== prevFilterKey) {
			prevFilterKey = filterKey
			appendedEdges = []
			appendedPageInfo = null
		}
	})

	let edges = $derived([...data.runs.edges, ...appendedEdges])
	let pageInfo = $derived<PageInfo>(appendedPageInfo ?? data.runs.pageInfo)

	const LAYER_OPTIONS = ['work', 'try', 'run']
	const STATUS_OPTIONS = ['pending', 'running', 'success', 'failure', 'preserved']

	function setFilter(key: 'layer' | 'status' | 'harness', value: string | null) {
		const params = new URLSearchParams($page.url.searchParams)
		if (value === null || value === '') {
			params.delete(key)
		} else {
			params.set(key, value)
		}
		const search = params.toString()
		goto(search ? `?${search}` : $page.url.pathname, { replaceState: false })
	}

	function toggleLayer(layer: string) {
		setFilter('layer', data.activeLayer === layer ? null : layer)
	}

	function toggleStatus(status: string) {
		setFilter('status', data.activeStatus === status ? null : status)
	}

	function applyHarness() {
		setFilter('harness', harnessInput.trim() || null)
	}

	async function loadMore() {
		if (!pageInfo.hasNextPage || loadingMore) return
		loadingMore = true
		try {
			const client = createClient()
			const result = await client.request<{ runs: RunConnection }>(PROJECT_RUNS_QUERY, {
				projectID: data.projectId,
				first: PAGE_SIZE,
				after: pageInfo.endCursor,
				layer: data.activeLayer ?? undefined,
				status: data.activeStatus ?? undefined,
				harness: data.activeHarness ?? undefined
			})
			appendedEdges = [...appendedEdges, ...result.runs.edges]
			appendedPageInfo = result.runs.pageInfo
		} finally {
			loadingMore = false
		}
	}

	function runDetailHref(runId: string): string {
		return `/nodes/${data.nodeId}/projects/${data.projectId}/runs/${runId}`
	}

	function beadHref(beadId: string): string {
		return `/nodes/${data.nodeId}/projects/${data.projectId}/beads/${beadId}`
	}

	function fmtDate(iso: string | null): string {
		if (!iso) return '—'
		return new Date(iso).toLocaleString()
	}

	function fmtDuration(ms: number | null): string {
		if (ms == null) return '—'
		if (ms < 1000) return `${ms}ms`
		if (ms < 60_000) return `${(ms / 1000).toFixed(1)}s`
		const m = Math.floor(ms / 60_000)
		const s = Math.floor((ms % 60_000) / 1000)
		return `${m}m ${s}s`
	}

	function layerBadgeClass(layer: string): string {
		switch (layer) {
			case 'work':
				return 'badge-layer-work'
			case 'try':
				return 'badge-layer-try'
			case 'run':
				return 'badge-layer-run'
			default:
				return 'badge-status-neutral'
		}
	}

	function statusBadgeClass(status: string): string {
		switch (status) {
			case 'success':
				return 'badge-status-closed'
			case 'failure':
				return 'badge-status-failed'
			case 'running':
				return 'badge-status-running'
			case 'preserved':
				return 'badge-status-in-progress'
			default:
				return 'badge-status-open'
		}
	}

	function chipClass(active: boolean): string {
		return active
			? 'rounded-sm border px-3 py-1 text-xs font-medium border-accent-lever bg-accent-lever/10 text-accent-lever dark:border-dark-accent-lever dark:bg-dark-accent-lever/20 dark:text-dark-accent-lever'
			: 'rounded-sm border px-3 py-1 text-xs font-medium border-border-line bg-bg-elevated text-fg-ink hover:border-fg-muted hover:bg-bg-surface dark:border-dark-border-line dark:bg-dark-bg-elevated dark:text-dark-fg-ink dark:hover:bg-dark-bg-surface'
	}
</script>

<div class="space-y-4">
	<div class="flex items-center justify-between">
		<h1 class="text-headline-lg font-headline-lg text-fg-ink dark:text-dark-fg-ink">Runs</h1>
		<span class="text-body-sm text-fg-muted dark:text-dark-fg-muted">
			{edges.length} of {data.runs.totalCount}
		</span>
	</div>

	<!-- Layer filter chips -->
	<div class="flex flex-wrap gap-2">
		<span class="self-center text-xs text-fg-muted dark:text-dark-fg-muted">Layer:</span>
		{#each LAYER_OPTIONS as layer}
			<button class={chipClass(data.activeLayer === layer)} onclick={() => toggleLayer(layer)}>
				{layer}
			</button>
		{/each}
		{#if data.activeLayer}
			<button
				class="rounded-sm border border-border-line bg-bg-elevated px-3 py-1 text-xs text-fg-ink hover:bg-bg-surface dark:border-dark-border-line dark:bg-dark-bg-elevated dark:text-dark-fg-ink dark:hover:bg-dark-bg-surface"
				onclick={() => setFilter('layer', null)}
			>
				clear
			</button>
		{/if}
	</div>

	<!-- Status filter chips -->
	<div class="flex flex-wrap gap-2">
		<span class="self-center text-xs text-fg-muted dark:text-dark-fg-muted">Status:</span>
		{#each STATUS_OPTIONS as status}
			<button class={chipClass(data.activeStatus === status)} onclick={() => toggleStatus(status)}>
				{status}
			</button>
		{/each}
		{#if data.activeStatus}
			<button
				class="rounded-sm border border-border-line bg-bg-elevated px-3 py-1 text-xs text-fg-ink hover:bg-bg-surface dark:border-dark-border-line dark:bg-dark-bg-elevated dark:text-dark-fg-ink dark:hover:bg-dark-bg-surface"
				onclick={() => setFilter('status', null)}
			>
				clear
			</button>
		{/if}
	</div>

	<!-- Harness filter -->
	<form
		class="flex items-center gap-2"
		onsubmit={(e) => {
			e.preventDefault()
			applyHarness()
		}}
	>
		<span class="text-xs text-fg-muted dark:text-dark-fg-muted">Harness:</span>
		<input
			type="text"
			bind:value={harnessInput}
			placeholder="claude / codex / agent"
			class="w-44 border border-border-line bg-bg-elevated px-2 py-1 font-mono-code text-mono-code text-fg-ink dark:border-dark-border-line dark:bg-dark-bg-elevated dark:text-dark-fg-ink"
		/>
		<button
			type="submit"
			class="border border-border-line bg-bg-elevated px-2 py-1 text-xs text-fg-muted hover:bg-bg-surface dark:border-dark-border-line dark:bg-dark-bg-elevated dark:text-dark-fg-muted dark:hover:bg-dark-bg-surface"
		>
			Apply
		</button>
		{#if data.activeHarness}
			<button
				type="button"
				onclick={() => {
					harnessInput = ''
					setFilter('harness', null)
				}}
				class="border border-border-line bg-bg-elevated px-2 py-1 text-xs text-fg-muted hover:bg-bg-surface dark:border-dark-border-line dark:bg-dark-bg-elevated dark:text-dark-fg-muted dark:hover:bg-dark-bg-surface"
			>
				Clear
			</button>
		{/if}
	</form>

	<div class="overflow-hidden border border-border-line dark:border-dark-border-line">
		<table class="w-full text-sm">
			<thead>
				<tr class="border-b border-border-line bg-bg-surface dark:border-dark-border-line dark:bg-dark-bg-surface">
					<th class="px-4 py-3 text-left font-label-caps text-label-caps uppercase tracking-wide text-fg-muted dark:text-dark-fg-muted">Layer</th>
					<th class="px-4 py-3 text-left font-label-caps text-label-caps uppercase tracking-wide text-fg-muted dark:text-dark-fg-muted">Status</th>
					<th class="px-4 py-3 text-left font-label-caps text-label-caps uppercase tracking-wide text-fg-muted dark:text-dark-fg-muted">Bead</th>
					<th class="px-4 py-3 text-left font-label-caps text-label-caps uppercase tracking-wide text-fg-muted dark:text-dark-fg-muted">Harness</th>
					<th class="px-4 py-3 text-left font-label-caps text-label-caps uppercase tracking-wide text-fg-muted dark:text-dark-fg-muted">Started</th>
					<th class="px-4 py-3 text-right font-label-caps text-label-caps uppercase tracking-wide text-fg-muted dark:text-dark-fg-muted">Duration</th>
				</tr>
			</thead>
			<tbody>
				{#each edges as edge (edge.cursor)}
					<tr
						class="cursor-pointer border-b border-border-line last:border-0 hover:bg-bg-surface dark:border-dark-border-line dark:hover:bg-dark-bg-surface"
						onclick={() => goto(runDetailHref(edge.node.id))}
					>
						<td class="px-4 py-3">
							<span class="inline-block rounded-full px-2 py-0.5 font-label-caps text-label-caps uppercase {layerBadgeClass(edge.node.layer)}">
								{edge.node.layer}
							</span>
						</td>
						<td class="px-4 py-3">
							<span class="inline-block border px-1.5 py-0.5 font-mono-code text-mono-code uppercase {statusBadgeClass(edge.node.status)}">
								{edge.node.status}
							</span>
						</td>
						<td class="px-4 py-3">
							{#if edge.node.beadId}
								<a
									href={beadHref(edge.node.beadId)}
									onclick={(e) => e.stopPropagation()}
									class="font-mono-code text-mono-code text-accent-lever hover:underline dark:text-dark-accent-lever"
								>
									{edge.node.beadId}
								</a>
							{:else}
								<span class="text-fg-muted dark:text-dark-fg-muted">—</span>
							{/if}
						</td>
						<td class="px-4 py-3 font-mono-code text-mono-code text-fg-muted dark:text-dark-fg-muted">
							{edge.node.harness ?? '—'}
						</td>
						<td class="px-4 py-3 font-mono-code text-mono-code text-fg-muted dark:text-dark-fg-muted">
							{fmtDate(edge.node.startedAt)}
						</td>
						<td class="px-4 py-3 text-right font-mono-code text-mono-code text-fg-muted dark:text-dark-fg-muted">
							{fmtDuration(edge.node.durationMs)}
						</td>
					</tr>
				{/each}
				{#if edges.length === 0}
					<tr>
						<td colspan="6" class="px-4 py-8 text-center text-body-sm text-fg-muted dark:text-dark-fg-muted">
							No runs found.
						</td>
					</tr>
				{/if}
			</tbody>
		</table>
	</div>

	{#if pageInfo.hasNextPage}
		<div class="flex justify-center">
			<button
				onclick={loadMore}
				disabled={loadingMore}
				class="rounded border border-border-line bg-bg-surface px-4 py-2 text-body-sm text-fg-muted hover:bg-bg-elevated hover:text-fg-ink disabled:opacity-50 dark:border-dark-border-line dark:bg-dark-bg-surface dark:text-dark-fg-muted dark:hover:bg-dark-bg-elevated dark:hover:text-dark-fg-ink"
			>
				{loadingMore ? 'Loading…' : 'Load more'}
			</button>
		</div>
	{/if}
</div>
