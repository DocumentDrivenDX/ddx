<script lang="ts">
	import type { PageData } from './$types'
	import D3Graph from '$lib/components/D3Graph.svelte'

	let { data }: { data: PageData } = $props()

	const links = $derived(
		data.graph.documents.flatMap((doc) => doc.dependsOn.map((depId) => ({ source: doc.id, target: depId })))
	)
</script>

<div class="flex h-full flex-col gap-4">
	<div class="flex shrink-0 items-center justify-between">
		<h1 class="text-xl font-semibold dark:text-white">Document Graph</h1>
		<span class="text-sm text-gray-500 dark:text-gray-400">
			{data.graph.documents.length} nodes &middot; {links.length} edges
		</span>
	</div>

	{#if data.graph.warnings.length > 0}
		<div
			class="shrink-0 rounded border border-amber-200 bg-amber-50 px-4 py-3 text-sm text-amber-800 dark:border-amber-800 dark:bg-amber-950 dark:text-amber-200"
		>
			{#each data.graph.warnings as warning}
				<div>{warning}</div>
			{/each}
		</div>
	{/if}

	{#if data.graph.documents.length === 0}
		<div class="flex flex-1 items-center justify-center text-gray-400 dark:text-gray-600">
			No documents in graph.
		</div>
	{:else}
		<div class="min-h-0 flex-1 overflow-hidden rounded-lg border border-gray-200 dark:border-gray-700">
			<D3Graph nodes={data.graph.documents} {links} />
		</div>
	{/if}
</div>
