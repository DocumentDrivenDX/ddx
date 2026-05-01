<script lang="ts">
	import type { PageData } from './$types';
	import { goto } from '$app/navigation';
	import { resolve } from '$app/paths';
	import { page } from '$app/stores';
	import D3Graph from '$lib/components/D3Graph.svelte';
	import IntegrityPanel from '$lib/components/IntegrityPanel.svelte';

	let { data }: { data: PageData } = $props();

	const links = $derived(
		data.graph.documents.flatMap((doc) =>
			doc.dependsOn.map((depId) => ({ source: doc.id, target: depId }))
		)
	);

	const issues = $derived(data.graph.issues ?? []);

	function handleNodeClick(node: { path: string }) {
		const p = $page.params as Record<string, string>;
		goto(
			resolve(
				`/nodes/${p['nodeId']}/projects/${p['projectId']}/documents/${node.path.split('/').map(encodeURIComponent).join('/')}`
			)
		);
	}
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
		<span class="text-sm text-gray-700 dark:text-gray-300">
			{data.graph.documents.length} nodes &middot; {links.length} edges
		</span>
	</div>

	{#if issues.length > 0}
		<IntegrityPanel {issues} />
	{/if}

	{#if data.graph.documents.length === 0}
		<div class="flex flex-1 items-center justify-center text-gray-700 dark:text-gray-300">
			No documents in graph.
		</div>
	{:else}
		<div class="min-h-0 flex-1 overflow-hidden rounded-lg border border-gray-200 dark:border-gray-700">
			<D3Graph nodes={data.graph.documents} {links} onNodeClick={handleNodeClick} />
		</div>
	{/if}
</div>
