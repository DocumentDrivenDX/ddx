<script lang="ts">
	import type { PageData } from './$types';
	import { goto } from '$app/navigation';

	let { data }: { data: PageData } = $props();

	function docHref(path: string): string {
		return `/nodes/${data.nodeId}/projects/${data.projectId}/documents/${path
			.split('/')
			.map(encodeURIComponent)
			.join('/')}`;
	}
</script>

<svelte:head>
	<title>Documents | DDx</title>
</svelte:head>

<div class="space-y-4">
	<div class="flex items-end justify-between gap-4">
		<div>
			<h1 class="text-headline-md font-headline-md text-fg-ink dark:text-dark-fg-ink">Documents</h1>
			<p class="text-body-sm text-fg-muted dark:text-dark-fg-muted">{data.documents.totalCount} total</p>
		</div>
	</div>

	<table class="w-full border-collapse text-sm">
		<thead>
			<tr>
				<th class="border-b border-border-line px-3 py-2 text-left">Title</th>
				<th class="border-b border-border-line px-3 py-2 text-left">Path</th>
			</tr>
		</thead>
		<tbody>
			{#if data.documents.edges.length === 0}
				<tr>
					<td colspan="2" class="border-b border-border-line px-3 py-6 text-center text-fg-muted dark:border-dark-border-line dark:text-dark-fg-muted">
						No documents found.
					</td>
				</tr>
			{:else}
				{#each data.documents.edges as edge (edge.node.id)}
					<tr class="hover:bg-bg-elevated dark:hover:bg-dark-bg-elevated">
						<td
							class="cursor-pointer border-b border-border-line px-3 py-2 font-medium text-accent-lever dark:border-dark-border-line dark:text-dark-accent-lever"
							onclick={() => goto(docHref(edge.node.path))}
						>
							{edge.node.title}
						</td>
						<td class="border-b border-border-line px-3 py-2 font-mono-code dark:border-dark-border-line">
							{edge.node.path}
						</td>
					</tr>
				{/each}
			{/if}
		</tbody>
	</table>
</div>
