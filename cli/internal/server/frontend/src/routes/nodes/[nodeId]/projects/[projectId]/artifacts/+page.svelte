<script lang="ts">
	import type { PageData } from './$types';
	import { goto } from '$app/navigation';
	import { resolve } from '$app/paths';
	import { Search } from 'lucide-svelte';
	import { createClient } from '$lib/gql/client';
	import FilterChip from '$lib/components/FilterChip.svelte';
	import {
		ARTIFACTS_QUERY,
		PAGE_SIZE,
		SORT_OPTIONS,
		STALENESS_OPTIONS,
		DEFAULT_SORT,
		type ArtifactSort,
		type Staleness
	} from './+page';
	import type { ArtifactEdge, ArtifactConnection, PageInfo } from './+page';
	import { writeState } from '$lib/urlState';
	import {
		GROUP_BY_OPTIONS,
		groupItems,
		groupCountLabel,
		axisAvailable,
		GROUP_BY_LABELS,
		type GroupBy
	} from './grouping';
	import { readPersistedGroupBy, writePersistedGroupBy } from './persistence';
	import { createRequestSequence, runLatest } from './searchFetcher';

	let { data }: { data: PageData } = $props();

	let allEdges = $state<ArtifactEdge[]>(data.artifacts.edges);
	let pageInfo = $state<PageInfo>(data.artifacts.pageInfo);
	let loading = $state(false);

	$effect(() => {
		allEdges = data.artifacts.edges;
		pageInfo = data.artifacts.pageInfo;
	});

	let q = $state(data.q);
	let groupBy = $state<GroupBy>(data.groupBy);
	let pendingStoredGroupBy: GroupBy | null | undefined = undefined;

	// sessionStorage persistence with stable keys (per project).
	const SS_KEY_MEDIA_TYPE = `artifacts:mediaType:${data.projectId}`;

	$effect(() => {
		if (typeof sessionStorage === 'undefined') return;
		try {
			writePersistedGroupBy(sessionStorage, data.projectId, groupBy);
			if (data.mediaType) sessionStorage.setItem(SS_KEY_MEDIA_TYPE, data.mediaType);
			else sessionStorage.removeItem(SS_KEY_MEDIA_TYPE);
		} catch {
			// sessionStorage may be unavailable (private mode, SSR); ignore.
		}
	});

	$effect(() => {
		if (data.hasGroupByParam || pendingStoredGroupBy === null) return;
		if (pendingStoredGroupBy === undefined) {
			if (typeof sessionStorage === 'undefined') {
				pendingStoredGroupBy = null;
				return;
			}
			try {
				pendingStoredGroupBy = readPersistedGroupBy(sessionStorage, data.projectId);
			} catch {
				pendingStoredGroupBy = null;
				return;
			}
		}
		if (pendingStoredGroupBy === null) return;
		if (pendingStoredGroupBy === 'workflowStage' && !workflowStageAxisAvailable) return;
		if (pendingStoredGroupBy !== groupBy) {
			groupBy = pendingStoredGroupBy;
			navigateWith({ groupBy: pendingStoredGroupBy }, { replace: true });
		}
		pendingStoredGroupBy = null;
	});

	// Server-side search: q is sent to the backend so results are correct
	// across all pages, not just the loaded edges.
	const filtered = $derived(allEdges);
	const workflowStageAxisAvailable = $derived(axisAvailable(filtered.map((e) => e.node.path)));
	const effectiveGroupBy = $derived(
		groupBy === 'workflowStage' && !workflowStageAxisAvailable ? 'folder' : groupBy
	);
	const visibleGroupByOptions = $derived(
		GROUP_BY_OPTIONS.filter((opt) => opt.value !== 'workflowStage' || workflowStageAxisAvailable)
	);

	const groups = $derived(
		groupItems(
			filtered.map((e) => ({ ...e, path: e.node.path, mediaType: e.node.mediaType })),
			effectiveGroupBy
		)
	);

	let searchDebounce: ReturnType<typeof setTimeout> | undefined;
	const fetchSeq = createRequestSequence();

	function navigateWith(patch: Parameters<typeof writeState>[1], opts?: { replace?: boolean }) {
		const url = new URL(window.location.href);
		const params = writeState(url.searchParams, patch);
		const search = params.toString();
		// goto() re-runs load(), which resets allEdges/pageInfo (cursor) via the
		// $effect bound to `data` — satisfying the "param change resets cursor" AC.
		void goto(
			resolve(
				`/nodes/${data.nodeId}/projects/${data.projectId}/artifacts${search ? `?${search}` : ''}`
			),
			{ replaceState: opts?.replace ?? false, keepFocus: true, noScroll: true }
		);
	}

	function onSearchInput(e: Event) {
		q = (e.target as HTMLInputElement).value;
		clearTimeout(searchDebounce);
		// Invalidate any in-flight loadMore so its (stale) results don't get
		// appended on top of fresh search results when the goto() reload lands.
		fetchSeq.invalidate();
		searchDebounce = setTimeout(() => {
			navigateWith({ q }, { replace: true });
		}, 200);
	}

	function selectGroupBy(next: GroupBy) {
		pendingStoredGroupBy = null;
		groupBy = next;
		navigateWith({ groupBy: next });
	}

	function selectSort(next: ArtifactSort) {
		navigateWith({ sort: next === DEFAULT_SORT ? null : next });
	}

	function selectStaleness(next: Staleness | null) {
		// Toggle off if user re-clicks the active chip.
		const value = next && next === data.staleness ? null : next;
		navigateWith({ staleness: value });
	}

	// Phase axis: HELIX phases sourced from path prefix docs/helix/NN-*/.
	// The PHASE_OPTIONS list mirrors the canonical numbered phases the project uses.
	const PHASE_OPTIONS: { label: string; value: string }[] = [
		{ label: '01 Frame', value: '01-frame' },
		{ label: '02 Design', value: '02-design' },
		{ label: '03 Test', value: '03-test' },
		{ label: '04 Build', value: '04-build' },
		{ label: '05 Deploy', value: '05-deploy' },
		{ label: '06 Iterate', value: '06-iterate' }
	];

	function selectPhase(next: string | null) {
		const value = next && next === data.phase ? null : next;
		navigateWith({ phase: value });
	}

	// Prefix axis: id-prefix segment (ADR|SD|FEAT|US|RSCH|PRD).
	// Multi-select OR semantics — clicking toggles membership.
	const PREFIX_OPTIONS = ['ADR', 'SD', 'FEAT', 'US', 'RSCH', 'PRD'] as const;

	function togglePrefix(p: string) {
		const cur = data.prefix ?? [];
		const next = cur.includes(p) ? cur.filter((x) => x !== p) : [...cur, p];
		navigateWith({ prefix: next });
	}

	const MEDIA_TYPES: { label: string; value: string | null }[] = [
		{ label: 'All', value: null },
		{ label: 'Markdown', value: 'text/markdown' },
		{ label: 'SVG', value: 'image/svg+xml' },
		{ label: 'Image', value: 'image/*' },
		{ label: 'PDF', value: 'application/pdf' },
		{ label: 'Excalidraw', value: 'application/vnd.excalidraw+json' },
		{ label: 'Unknown', value: 'unknown' }
	];

	function selectMediaType(mediaType: string | null) {
		navigateWith({ mediaType, q });
	}

	function openGraph() {
		void goto(resolve(`/nodes/${data.nodeId}/projects/${data.projectId}/graph`));
	}

	function viewInGraph(artifactId: string) {
		const docId = artifactId.replace(/^doc:/, '');
		void goto(
			resolve(
				`/nodes/${data.nodeId}/projects/${data.projectId}/graph?highlight=${encodeURIComponent(docId)}`
			)
		);
	}

	function openArtifact(id: string) {
		// Pass current filter/search state as a "back" param so the detail page
		// can return to the same filtered list state.
		const listUrl = new URL(window.location.href);
		const backHref = listUrl.pathname + listUrl.search;
		const backParam =
			backHref !== `/nodes/${data.nodeId}/projects/${data.projectId}/artifacts`
				? `?back=${encodeURIComponent(backHref)}`
				: '';
		void goto(
			resolve(
				`/nodes/${data.nodeId}/projects/${data.projectId}/artifacts/${encodeURIComponent(id)}${backParam}`
			)
		);
	}

	async function loadMore() {
		if (!pageInfo.hasNextPage || loading) return;
		loading = true;
		try {
			const client = createClient();
			const outcome = await runLatest(fetchSeq, () =>
				client.request<{ artifacts: ArtifactConnection }>(ARTIFACTS_QUERY, {
					projectID: data.projectId,
					first: PAGE_SIZE,
					after: pageInfo.endCursor,
					mediaType: data.mediaType ?? undefined,
					search: q ? q : undefined,
					sort: data.sort,
					staleness: data.staleness ?? undefined,
					phase: data.phase ?? undefined,
					prefix: data.prefix && data.prefix.length > 0 ? data.prefix : undefined
				})
			);
			if (outcome.stale) return;
			allEdges = [...allEdges, ...outcome.value.artifacts.edges];
			pageInfo = outcome.value.artifacts.pageInfo;
		} finally {
			loading = false;
		}
	}

	// Render a server-supplied snippet to HTML. The resolver wraps the matched
	// substring in markdown emphasis ("**…**"); we escape the rest and convert
	// the marker pairs to <mark> for visual highlighting. Only the first
	// matching pair is highlighted (resolver emits a single match per snippet).
	function escapeHtml(s: string): string {
		return s
			.replace(/&/g, '&amp;')
			.replace(/</g, '&lt;')
			.replace(/>/g, '&gt;')
			.replace(/"/g, '&quot;')
			.replace(/'/g, '&#39;');
	}
	function renderSnippet(snippet: string): string {
		const escaped = escapeHtml(snippet);
		return escaped.replace(
			/\*\*([\s\S]+?)\*\*/,
			'<mark class="rounded bg-yellow-200 px-0.5 text-fg-ink dark:bg-yellow-500/40 dark:text-dark-fg-ink">$1</mark>'
		);
	}

	function stalenessBadge(staleness: string): { label: string; cls: string } {
		switch (staleness) {
			case 'fresh':
				return {
					label: 'fresh',
					cls: 'bg-green-100 text-green-800 dark:bg-green-900/30 dark:text-green-400'
				};
			case 'stale':
				return {
					label: 'stale',
					cls: 'bg-amber-100 text-amber-800 dark:bg-amber-900/30 dark:text-amber-400'
				};
			case 'missing':
				return {
					label: 'missing',
					cls: 'bg-red-100 text-red-800 dark:bg-red-900/30 dark:text-red-400'
				};
			default:
				return {
					label: staleness,
					cls: 'bg-bg-surface text-fg-muted dark:bg-dark-bg-surface dark:text-dark-fg-muted'
				};
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
				class="border-border-line bg-bg-surface text-body-sm text-fg-muted hover:bg-bg-elevated hover:text-fg-ink dark:border-dark-border-line dark:bg-dark-bg-surface dark:text-dark-fg-muted dark:hover:bg-dark-bg-elevated dark:hover:text-dark-fg-ink rounded border px-3 py-1.5"
			>
				Open Graph
			</button>
		</div>
	</div>

	<!-- Filter chips -->
	<div class="flex flex-wrap gap-2" data-testid="media-type-chips">
		{#each MEDIA_TYPES as chip (chip.label)}
			<FilterChip
				label={chip.label}
				active={chip.value === data.mediaType}
				onclick={() => selectMediaType(chip.value)}
			/>
		{/each}
	</div>

	<!-- Staleness filter chips -->
	<div class="flex flex-wrap gap-2" data-testid="staleness-chips">
		<span
			class="font-label-caps text-label-caps text-fg-muted dark:text-dark-fg-muted self-center uppercase"
			>Staleness</span
		>
		{#each STALENESS_OPTIONS as value (value)}
			{@const active = data.staleness === value}
			<FilterChip
				label={value}
				testid="staleness-chip-{value}"
				{active}
				ariaPressed={active}
				onclick={() => selectStaleness(value)}
			/>
		{/each}
		{#if data.staleness}
			<FilterChip
				label="Clear"
				testid="staleness-chip-clear"
				clear
				onclick={() => selectStaleness(null)}
			/>
		{/if}
	</div>

	<!-- Phase filter chips (HELIX phase, sourced from docs/helix/NN-*/ path prefix) -->
	<div class="flex flex-wrap gap-2" data-testid="phase-chips">
		<span
			class="font-label-caps text-label-caps text-fg-muted dark:text-dark-fg-muted self-center uppercase"
			>Phase</span
		>
		{#each PHASE_OPTIONS as opt (opt.value)}
			{@const active = data.phase === opt.value}
			<FilterChip
				label={opt.label}
				testid="phase-chip-{opt.value}"
				{active}
				ariaPressed={active}
				onclick={() => selectPhase(opt.value)}
			/>
		{/each}
		{#if data.phase}
			<FilterChip label="Clear" testid="phase-chip-clear" clear onclick={() => selectPhase(null)} />
		{/if}
	</div>

	<!-- Prefix filter chips (id-prefix segment, multi-select OR) -->
	<div class="flex flex-wrap gap-2" data-testid="prefix-chips">
		<span
			class="font-label-caps text-label-caps text-fg-muted dark:text-dark-fg-muted self-center uppercase"
			>Prefix</span
		>
		{#each PREFIX_OPTIONS as p (p)}
			{@const active = (data.prefix ?? []).includes(p)}
			<FilterChip
				label={p}
				testid="prefix-chip-{p}"
				{active}
				ariaPressed={active}
				onclick={() => togglePrefix(p)}
			/>
		{/each}
		{#if data.prefix && data.prefix.length > 0}
			<FilterChip
				label="Clear"
				testid="prefix-chip-clear"
				clear
				onclick={() => navigateWith({ prefix: [] })}
			/>
		{/if}
	</div>

	<!-- Search bar + group-by selector -->
	<div class="flex items-center gap-3">
		<div class="relative flex-1">
			<Search
				class="text-fg-muted dark:text-dark-fg-muted pointer-events-none absolute top-1/2 left-3 h-4 w-4 -translate-y-1/2"
			/>
			<input
				type="search"
				placeholder="Search title or path…"
				value={q}
				oninput={onSearchInput}
				class="border-border-line bg-bg-surface text-body-sm text-fg-ink placeholder:text-fg-muted focus:ring-accent-lever dark:border-dark-border-line dark:bg-dark-bg-surface dark:text-dark-fg-ink dark:placeholder:text-dark-fg-muted dark:focus:ring-dark-accent-lever w-full rounded border py-2 pr-3 pl-9 focus:ring-1 focus:outline-none"
			/>
		</div>
		<label class="text-body-sm text-fg-muted dark:text-dark-fg-muted flex items-center gap-2">
			<span>Sort by</span>
			<select
				aria-label="Sort by"
				data-testid="sort-select"
				value={data.sort}
				onchange={(e) => selectSort((e.target as HTMLSelectElement).value as ArtifactSort)}
				class="border-border-line bg-bg-surface text-body-sm text-fg-ink dark:border-dark-border-line dark:bg-dark-bg-surface dark:text-dark-fg-ink rounded border px-2 py-1.5"
			>
				{#each SORT_OPTIONS as opt (opt.value)}
					<option value={opt.value}>{opt.label}</option>
				{/each}
			</select>
		</label>
		<label class="text-body-sm text-fg-muted dark:text-dark-fg-muted flex items-center gap-2">
			<span>Group by</span>
			<select
				aria-label="Group by"
				value={effectiveGroupBy}
				onchange={(e) => selectGroupBy((e.target as HTMLSelectElement).value as GroupBy)}
				class="border-border-line bg-bg-surface text-body-sm text-fg-ink dark:border-dark-border-line dark:bg-dark-bg-surface dark:text-dark-fg-ink rounded border px-2 py-1.5"
			>
				{#each visibleGroupByOptions as opt (opt.value)}
					<option value={opt.value}>{opt.label}</option>
				{/each}
			</select>
		</label>
	</div>

	<!-- Table -->
	<div class="border-border-line dark:border-dark-border-line overflow-hidden border">
		<table class="w-full text-sm">
			<thead>
				<tr
					class="border-border-line bg-bg-surface dark:border-dark-border-line dark:bg-dark-bg-surface border-b"
				>
					<th
						class="font-label-caps text-label-caps text-fg-muted dark:text-dark-fg-muted px-4 py-3 text-left tracking-wide uppercase"
						>Title</th
					>
					<th
						class="font-label-caps text-label-caps text-fg-muted dark:text-dark-fg-muted px-4 py-3 text-left tracking-wide uppercase"
						>Path</th
					>
					<th
						class="font-label-caps text-label-caps text-fg-muted dark:text-dark-fg-muted px-4 py-3 text-left tracking-wide uppercase"
						>Type</th
					>
					<th
						class="font-label-caps text-label-caps text-fg-muted dark:text-dark-fg-muted px-4 py-3 text-left tracking-wide uppercase"
						>Staleness</th
					>
					<th class="px-4 py-3"></th>
				</tr>
			</thead>
			{#each groups as group (group.key)}
				<tbody role="rowgroup" aria-label="{GROUP_BY_LABELS[effectiveGroupBy]}: {group.key}">
					<tr class="bg-bg-elevated dark:bg-dark-bg-elevated">
						<th
							colspan="5"
							scope="rowgroup"
							role="rowheader"
							class="font-label-caps text-label-caps text-fg-muted dark:text-dark-fg-muted px-4 py-2 text-left tracking-wide uppercase"
						>
							{group.key}
							<span class="text-fg-muted dark:text-dark-fg-muted ml-2 normal-case">
								({groupCountLabel(group.items.length, filtered.length, pageInfo.hasNextPage)})
							</span>
						</th>
					</tr>
					{#each group.items as edge (edge.cursor)}
						{@const badge = stalenessBadge(edge.node.staleness)}
						<tr
							onclick={() => openArtifact(edge.node.id)}
							class="border-border-line hover:bg-bg-surface dark:border-dark-border-line dark:hover:bg-dark-bg-surface cursor-pointer border-b last:border-0"
						>
							<td class="text-fg-ink dark:text-dark-fg-ink px-4 py-3">
								{edge.node.title}
							</td>
							<td
								class="font-mono-code text-mono-code text-fg-muted dark:text-dark-fg-muted px-4 py-3"
							>
								{edge.node.path}
							</td>
							<td
								class="font-mono-code text-mono-code text-fg-muted dark:text-dark-fg-muted px-4 py-3"
							>
								{edge.node.mediaType}
							</td>
							<td class="px-4 py-3">
								<span
									class="font-label-caps text-label-caps inline-block rounded-full px-2 py-0.5 uppercase {badge.cls}"
								>
									{badge.label}
								</span>
							</td>
							<td class="px-4 py-3">
								<button
									onclick={(e) => {
										e.stopPropagation();
										viewInGraph(edge.node.id);
									}}
									class="border-border-line bg-bg-surface font-label-caps text-label-caps text-fg-muted hover:bg-bg-elevated hover:text-fg-ink dark:border-dark-border-line dark:bg-dark-bg-surface dark:text-dark-fg-muted dark:hover:bg-dark-bg-elevated dark:hover:text-dark-fg-ink rounded border px-2 py-0.5 uppercase"
								>
									View in Graph
								</button>
							</td>
						</tr>
						{#if edge.snippet}
							<tr
								data-testid="artifact-snippet-{edge.node.id}"
								onclick={() => openArtifact(edge.node.id)}
								class="border-border-line hover:bg-bg-surface dark:border-dark-border-line dark:hover:bg-dark-bg-surface cursor-pointer border-b last:border-0"
							>
								<td
									colspan="5"
									class="text-body-sm text-fg-muted dark:text-dark-fg-muted px-4 pb-3"
								>
									<!-- eslint-disable-next-line svelte/no-at-html-tags -->
									<span class="block leading-snug">{@html renderSnippet(edge.snippet)}</span>
								</td>
							</tr>
						{/if}
					{/each}
				</tbody>
			{/each}
			{#if filtered.length === 0}
				<tbody>
					<tr>
						<td
							colspan="5"
							class="text-body-sm text-fg-muted dark:text-dark-fg-muted px-4 py-8 text-center"
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
				class="border-border-line bg-bg-surface text-body-sm text-fg-muted hover:bg-bg-elevated hover:text-fg-ink dark:border-dark-border-line dark:bg-dark-bg-surface dark:text-dark-fg-muted dark:hover:bg-dark-bg-elevated dark:hover:text-dark-fg-ink rounded border px-4 py-2 disabled:opacity-50"
			>
				{loading ? 'Loading…' : 'Load more'}
			</button>
		</div>
	{/if}
</div>
