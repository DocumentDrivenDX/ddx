<script lang="ts">
	import type { PageData } from './$types'
	import { goto } from '$app/navigation'
	import { Search } from 'lucide-svelte'
	import { createClient } from '$lib/gql/client'
	import { ARTIFACTS_QUERY, PAGE_SIZE } from './+page'
	import type { ArtifactEdge, ArtifactConnection, PageInfo } from './+page'
	import { writeState } from '$lib/urlState'
	import {
		groupItems,
		GROUP_BY_LABELS,
		type GroupBy
	} from './grouping'

	let { data }: { data: PageData } = $props()

	let allEdges = $state<ArtifactEdge[]>(data.artifacts.edges)
	let pageInfo = $state<PageInfo>(data.artifacts.pageInfo)
	let loading = $state(false)

	$effect(() => {
		allEdges = data.artifacts.edges
		pageInfo = data.artifacts.pageInfo
	})

	let q = $state(data.q)
	let groupBy = $state<GroupBy>(data.groupBy)

	// sessionStorage persistence with stable keys (per project).
	const SS_KEY_GROUP_BY = `artifacts:groupBy:${data.projectId}`
	const SS_KEY_MEDIA_TYPE = `artifacts:mediaType:${data.projectId}`

	$effect(() => {
		if (typeof sessionStorage === 'undefined') return
		try {
			sessionStorage.setItem(SS_KEY_GROUP_BY, groupBy)
			if (data.mediaType) sessionStorage.setItem(SS_KEY_MEDIA_TYPE, data.mediaType)
			else sessionStorage.removeItem(SS_KEY_MEDIA_TYPE)
		} catch {
			// sessionStorage may be unavailable (private mode, SSR); ignore.
		}
	})

	// Server-side search: q is sent to the backend so results are correct
	// across all pages, not just the loaded edges.
	const filtered = $derived(allEdges)

	const groups = $derived(
		groupItems(
			filtered.map((e) => ({ ...e, path: e.node.path, mediaType: e.node.mediaType })),
			groupBy
		)
	)

	function onSearchInput(e: Event) {
		q = (e.target as HTMLInputElement).value
		const url = new URL(window.location.href)
		const next = writeState(url.searchParams, { q })
		url.search = next.toString()
		history.replaceState(null, '', url.toString())
	}

	function selectGroupBy(next: GroupBy) {
		groupBy = next
		const url = new URL(window.location.href)
		const params = writeState(url.searchParams, { groupBy: next })
		const search = params.toString()
		goto(url.pathname + (search ? `?${search}` : ''))
	}

	const MEDIA_TYPES: { label: string; value: string | null }[] = [
		{ label: 'All', value: null },
		{ label: 'Markdown', value: 'text/markdown' },
		{ label: 'SVG', value: 'image/svg+xml' },
		{ label: 'Image', value: 'image/*' },
		{ label: 'PDF', value: 'application/pdf' },
		{ label: 'Excalidraw', value: 'application/vnd.excalidraw+json' },
		{ label: 'Unknown', value: 'unknown' }
	]

	function selectMediaType(mediaType: string | null) {
		const url = new URL(window.location.href)
		const params = writeState(url.searchParams, { mediaType, q })
		const search = params.toString()
		goto(url.pathname + (search ? `?${search}` : ''))
	}

	function openGraph() {
		goto(`/nodes/${data.nodeId}/projects/${data.projectId}/graph`)
	}

	function viewInGraph(artifactId: string) {
		const docId = artifactId.replace(/^doc:/, '')
		goto(`/nodes/${data.nodeId}/projects/${data.projectId}/graph?highlight=${encodeURIComponent(docId)}`)
	}

	function openArtifact(id: string) {
		// Pass current filter/search state as a "back" param so the detail page
		// can return to the same filtered list state.
		const listUrl = new URL(window.location.href)
		const detailBase = `/nodes/${data.nodeId}/projects/${data.projectId}/artifacts/${encodeURIComponent(id)}`
		const detailUrl = new URL(detailBase, window.location.origin)
		const backHref = listUrl.pathname + listUrl.search
		if (backHref !== `/nodes/${data.nodeId}/projects/${data.projectId}/artifacts`) {
			detailUrl.searchParams.set('back', backHref)
		}
		goto(detailUrl.pathname + detailUrl.search)
	}

	async function loadMore() {
		if (!pageInfo.hasNextPage || loading) return
		loading = true
		try {
			const client = createClient()
			const result = await client.request<{ artifacts: ArtifactConnection }>(ARTIFACTS_QUERY, {
				projectID: data.projectId,
				first: PAGE_SIZE,
				after: pageInfo.endCursor,
				mediaType: data.mediaType ?? undefined,
				search: q ? q : undefined
			})
			allEdges = [...allEdges, ...result.artifacts.edges]
			pageInfo = result.artifacts.pageInfo
		} finally {
			loading = false
		}
	}

	function stalenessBadge(staleness: string): { label: string; cls: string } {
		switch (staleness) {
			case 'fresh':
				return {
					label: 'fresh',
					cls: 'bg-green-100 text-green-800 dark:bg-green-900/30 dark:text-green-400'
				}
			case 'stale':
				return {
					label: 'stale',
					cls: 'bg-amber-100 text-amber-800 dark:bg-amber-900/30 dark:text-amber-400'
				}
			case 'missing':
				return {
					label: 'missing',
					cls: 'bg-red-100 text-red-800 dark:bg-red-900/30 dark:text-red-400'
				}
			default:
				return { label: staleness, cls: 'bg-bg-surface text-fg-muted dark:bg-dark-bg-surface dark:text-dark-fg-muted' }
		}
	}
</script>

<div class="space-y-4">
	<div class="flex items-center justify-between">
		<h1 class="text-headline-md font-headline-md text-fg-ink dark:text-dark-fg-ink">Artifacts</h1>
		<div class="flex items-center gap-3">
			<span class="text-body-sm text-fg-muted dark:text-dark-fg-muted">
				{data.artifacts.totalCount} total
			</span>
			<button
				onclick={openGraph}
				class="rounded border border-border-line bg-bg-surface px-3 py-1.5 text-body-sm text-fg-muted hover:bg-bg-elevated hover:text-fg-ink dark:border-dark-border-line dark:bg-dark-bg-surface dark:text-dark-fg-muted dark:hover:bg-dark-bg-elevated dark:hover:text-dark-fg-ink"
			>
				Open Graph
			</button>
		</div>
	</div>

	<!-- Filter chips -->
	<div class="flex flex-wrap gap-2">
		{#each MEDIA_TYPES as chip}
			{@const active = chip.value === data.mediaType}
			<button
				onclick={() => selectMediaType(chip.value)}
				class="rounded-full px-3 py-1 font-label-caps text-label-caps uppercase transition-colors {active
					? 'bg-accent-lever text-white dark:bg-dark-accent-lever'
					: 'bg-bg-surface text-fg-muted hover:bg-bg-elevated hover:text-fg-ink dark:bg-dark-bg-surface dark:text-dark-fg-muted dark:hover:bg-dark-bg-elevated dark:hover:text-dark-fg-ink'}"
			>
				{chip.label}
			</button>
		{/each}
	</div>

	<!-- Search bar + group-by selector -->
	<div class="flex items-center gap-3">
		<div class="relative flex-1">
			<Search
				class="pointer-events-none absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-fg-muted dark:text-dark-fg-muted"
			/>
			<input
				type="search"
				placeholder="Search title or path…"
				value={q}
				oninput={onSearchInput}
				class="w-full rounded border border-border-line bg-bg-surface py-2 pl-9 pr-3 text-body-sm text-fg-ink placeholder:text-fg-muted focus:outline-none focus:ring-1 focus:ring-accent-lever dark:border-dark-border-line dark:bg-dark-bg-surface dark:text-dark-fg-ink dark:placeholder:text-dark-fg-muted dark:focus:ring-dark-accent-lever"
			/>
		</div>
		<label class="flex items-center gap-2 text-body-sm text-fg-muted dark:text-dark-fg-muted">
			<span>Group by</span>
			<select
				aria-label="Group by"
				value={groupBy}
				onchange={(e) => selectGroupBy((e.target as HTMLSelectElement).value as GroupBy)}
				class="rounded border border-border-line bg-bg-surface px-2 py-1.5 text-body-sm text-fg-ink dark:border-dark-border-line dark:bg-dark-bg-surface dark:text-dark-fg-ink"
			>
				<option value="folder">{GROUP_BY_LABELS.folder}</option>
				<option value="prefix">{GROUP_BY_LABELS.prefix}</option>
				<option value="mediaType">{GROUP_BY_LABELS.mediaType}</option>
			</select>
		</label>
	</div>

	<!-- Table -->
	<div class="overflow-hidden border border-border-line dark:border-dark-border-line">
		<table class="w-full text-sm">
			<thead>
				<tr
					class="border-b border-border-line bg-bg-surface dark:border-dark-border-line dark:bg-dark-bg-surface"
				>
					<th
						class="px-4 py-3 text-left font-label-caps text-label-caps uppercase tracking-wide text-fg-muted dark:text-dark-fg-muted"
						>Title</th
					>
					<th
						class="px-4 py-3 text-left font-label-caps text-label-caps uppercase tracking-wide text-fg-muted dark:text-dark-fg-muted"
						>Path</th
					>
					<th
						class="px-4 py-3 text-left font-label-caps text-label-caps uppercase tracking-wide text-fg-muted dark:text-dark-fg-muted"
						>Type</th
					>
					<th
						class="px-4 py-3 text-left font-label-caps text-label-caps uppercase tracking-wide text-fg-muted dark:text-dark-fg-muted"
						>Staleness</th
					>
					<th class="px-4 py-3"></th>
				</tr>
			</thead>
			{#each groups as group (group.key)}
				<tbody role="rowgroup" aria-label="{GROUP_BY_LABELS[groupBy]}: {group.key}">
					<tr class="bg-bg-elevated dark:bg-dark-bg-elevated">
						<th
							colspan="5"
							scope="rowgroup"
							role="rowheader"
							class="px-4 py-2 text-left font-label-caps text-label-caps uppercase tracking-wide text-fg-muted dark:text-dark-fg-muted"
						>
							{group.key}
							<span class="ml-2 normal-case text-fg-muted dark:text-dark-fg-muted">({group.items.length})</span>
						</th>
					</tr>
					{#each group.items as edge (edge.cursor)}
						{@const badge = stalenessBadge(edge.node.staleness)}
						<tr
							onclick={() => openArtifact(edge.node.id)}
							class="cursor-pointer border-b border-border-line last:border-0 hover:bg-bg-surface dark:border-dark-border-line dark:hover:bg-dark-bg-surface"
						>
							<td class="px-4 py-3 text-fg-ink dark:text-dark-fg-ink">
								{edge.node.title}
							</td>
							<td
								class="px-4 py-3 font-mono-code text-mono-code text-fg-muted dark:text-dark-fg-muted"
							>
								{edge.node.path}
							</td>
							<td class="px-4 py-3 font-mono-code text-mono-code text-fg-muted dark:text-dark-fg-muted">
								{edge.node.mediaType}
							</td>
							<td class="px-4 py-3">
								<span
									class="inline-block rounded-full px-2 py-0.5 font-label-caps text-label-caps uppercase {badge.cls}"
								>
									{badge.label}
								</span>
							</td>
							<td class="px-4 py-3">
								<button
									onclick={(e) => { e.stopPropagation(); viewInGraph(edge.node.id) }}
									class="rounded border border-border-line bg-bg-surface px-2 py-0.5 font-label-caps text-label-caps uppercase text-fg-muted hover:bg-bg-elevated hover:text-fg-ink dark:border-dark-border-line dark:bg-dark-bg-surface dark:text-dark-fg-muted dark:hover:bg-dark-bg-elevated dark:hover:text-dark-fg-ink"
								>
									View in Graph
								</button>
							</td>
						</tr>
					{/each}
				</tbody>
			{/each}
			{#if filtered.length === 0}
				<tbody>
					<tr>
						<td
							colspan="5"
							class="px-4 py-8 text-center text-body-sm text-fg-muted dark:text-dark-fg-muted"
						>
							No artifacts found.
						</td>
					</tr>
				</tbody>
			{/if}
		</table>
	</div>

	<!-- Load more -->
	{#if pageInfo.hasNextPage}
		<div class="flex justify-center">
			<button
				onclick={loadMore}
				disabled={loading}
				class="rounded border border-border-line bg-bg-surface px-4 py-2 text-body-sm text-fg-muted hover:bg-bg-elevated hover:text-fg-ink disabled:opacity-50 dark:border-dark-border-line dark:bg-dark-bg-surface dark:text-dark-fg-muted dark:hover:bg-dark-bg-elevated dark:hover:text-dark-fg-ink"
			>
				{loading ? 'Loading…' : 'Load more'}
			</button>
		</div>
	{/if}
</div>
