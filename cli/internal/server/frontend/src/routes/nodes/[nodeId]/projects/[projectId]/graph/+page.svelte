<script lang="ts">
	import type { PageData } from './$types';
	import { goto } from '$app/navigation';
	import { resolve } from '$app/paths';
	import { page } from '$app/stores';
	import D3Graph from '$lib/components/D3Graph.svelte';
	import IntegrityPanel from '$lib/components/IntegrityPanel.svelte';
	import type { GraphDocument } from './+page';

	let { data }: { data: PageData } = $props();

	const issues = $derived(data.graph.issues ?? []);

	// Filter state — derived from URL search params
	let activeMediaTypes = $state<string[]>([]);
	let activeStaleness = $state<string[]>([]);

	// Sync filter state from URL on init
	$effect(() => {
		const urlMediaType = $page.url.searchParams.get('mediaType');
		const urlStaleness = $page.url.searchParams.get('staleness');
		activeMediaTypes = urlMediaType ? urlMediaType.split(',').filter(Boolean) : [];
		activeStaleness = urlStaleness ? urlStaleness.split(',').filter(Boolean) : [];
	});

	// Filtered nodes: apply active filter chips
	const visibleNodes = $derived.by(() => {
		let docs: GraphDocument[] = data.graph.documents;
		if (activeMediaTypes.length > 0) {
			docs = docs.filter((d) => activeMediaTypes.includes(d.mediaType));
		}
		if (activeStaleness.length > 0) {
			docs = docs.filter((d) => activeStaleness.includes(d.staleness));
		}
		return docs;
	});

	const visibleIds = $derived(new Set(visibleNodes.map((d) => d.id)));

	// Only include links between visible nodes
	const visibleLinks = $derived(
		data.graph.documents
			.flatMap((doc) => doc.dependsOn.map((depId) => ({ source: doc.id, target: depId })))
			.filter((l) => visibleIds.has(l.source) && visibleIds.has(l.target))
	);

	function updateFilters(mediaTypes: string[], staleness: string[]) {
		const url = new URL($page.url);
		if (mediaTypes.length > 0) url.searchParams.set('mediaType', mediaTypes.join(','));
		else url.searchParams.delete('mediaType');
		if (staleness.length > 0) url.searchParams.set('staleness', staleness.join(','));
		else url.searchParams.delete('staleness');
		goto(url.pathname + url.search, { replaceState: true, keepFocus: true });
	}

	function toggleMediaType(mt: string) {
		const next = activeMediaTypes.includes(mt)
			? activeMediaTypes.filter((m) => m !== mt)
			: [...activeMediaTypes, mt];
		updateFilters(next, activeStaleness);
	}

	function toggleStaleness(s: string) {
		const next = activeStaleness.includes(s)
			? activeStaleness.filter((v) => v !== s)
			: [...activeStaleness, s];
		updateFilters(activeMediaTypes, next);
	}

	function handleNodeClick(node: { path: string; mediaType?: string; id: string }) {
		const p = $page.params as Record<string, string>;
		if (node.mediaType === 'text/markdown' || !node.mediaType) {
			goto(
				resolve(
					`/nodes/${p['nodeId']}/projects/${p['projectId']}/documents/${node.path.split('/').map(encodeURIComponent).join('/')}`
				)
			);
		} else {
			goto(
				resolve(
					`/nodes/${p['nodeId']}/projects/${p['projectId']}/artifacts/${encodeURIComponent('doc:' + node.id)}`
				)
			);
		}
	}

	function handleTransformChange(t: { x: number; y: number; k: number }) {
		const url = new URL($page.url);
		url.searchParams.set('zoom', t.k.toFixed(3));
		url.searchParams.set('pan', `${t.x.toFixed(1)},${t.y.toFixed(1)}`);
		history.replaceState(null, '', url.toString());
	}

	const MEDIA_TYPE_CHIPS = [
		{ label: 'Markdown', value: 'text/markdown' },
		{ label: 'Image', value: 'image/*' },
		{ label: 'Unknown', value: 'unknown' }
	];

	const STALENESS_CHIPS = [
		{ label: 'Fresh', value: 'fresh' },
		{ label: 'Stale', value: 'stale' },
		{ label: 'Missing', value: 'missing' }
	];
</script>

<div class="flex flex-col gap-4" style="height: calc(100dvh - 40px - 3rem)">
	<div class="flex shrink-0 items-center justify-between">
		<div class="flex items-center gap-3">
			<h1 class="text-xl font-semibold dark:text-white">Document Graph</h1>
			{#if issues.length > 0}
				<span
					data-testid="integrity-badge"
					class="rounded-full bg-amber-200 px-2 py-0.5 text-xs font-medium text-amber-900 dark:bg-amber-800 dark:text-amber-100"
				>
					{issues.length}
					{issues.length === 1 ? 'issue' : 'issues'}
				</span>
			{/if}
		</div>
		<div class="flex items-center gap-4">
			<a
				href={`/nodes/${$page.params['nodeId']}/projects/${$page.params['projectId']}/artifacts?mediaType=text%2Fmarkdown`}
				class="text-sm text-accent-lever hover:underline dark:text-dark-accent-lever"
			>
				Back to documents
			</a>
			<span class="text-sm text-gray-700 dark:text-gray-300">
				{visibleNodes.length} nodes &middot; {visibleLinks.length} edges
			</span>
		</div>
	</div>

	<!-- Filter chips -->
	<div class="flex shrink-0 flex-wrap gap-2">
		<span class="self-center text-xs text-fg-muted dark:text-dark-fg-muted">Type:</span>
		{#each MEDIA_TYPE_CHIPS as chip}
			{@const active = activeMediaTypes.includes(chip.value)}
			<button
				onclick={() => toggleMediaType(chip.value)}
				data-testid={`filter-mediatype-${chip.value.replace(/[^a-z]/gi, '-')}`}
				class="rounded-full px-3 py-0.5 font-label-caps text-label-caps uppercase transition-colors {active
					? 'bg-accent-lever text-white dark:bg-dark-accent-lever'
					: 'bg-bg-surface text-fg-muted hover:bg-bg-elevated hover:text-fg-ink dark:bg-dark-bg-surface dark:text-dark-fg-muted dark:hover:bg-dark-bg-elevated dark:hover:text-dark-fg-ink'}"
			>
				{chip.label}
			</button>
		{/each}
		<span class="self-center text-xs text-fg-muted dark:text-dark-fg-muted">Staleness:</span>
		{#each STALENESS_CHIPS as chip}
			{@const active = activeStaleness.includes(chip.value)}
			<button
				onclick={() => toggleStaleness(chip.value)}
				data-testid={`filter-staleness-${chip.value}`}
				class="rounded-full px-3 py-0.5 font-label-caps text-label-caps uppercase transition-colors {active
					? 'bg-accent-lever text-white dark:bg-dark-accent-lever'
					: 'bg-bg-surface text-fg-muted hover:bg-bg-elevated hover:text-fg-ink dark:bg-dark-bg-surface dark:text-dark-fg-muted dark:hover:bg-dark-bg-elevated dark:hover:text-dark-fg-ink'}"
			>
				{chip.label}
			</button>
		{/each}
	</div>

	{#if issues.length > 0}
		<IntegrityPanel {issues} pathToDocId={data.graph.pathToDocId} />
	{/if}

	{#if data.graph.documents.length === 0}
		<div class="flex flex-1 items-center justify-center text-gray-700 dark:text-gray-300">
			No documents in graph.
		</div>
	{:else}
		<div class="min-h-0 flex-1 overflow-hidden rounded-lg border border-gray-200 dark:border-gray-700">
			<D3Graph
				nodes={visibleNodes}
				links={visibleLinks}
				onNodeClick={handleNodeClick}
				initialTransform={data.initialTransform}
				onTransformChange={handleTransformChange}
				highlightNodeId={data.highlightNodeId}
			/>
		</div>
	{/if}
</div>
