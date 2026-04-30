<script lang="ts">
	import type { PageData } from './$types';
	import { goto } from '$app/navigation';
	import { page } from '$app/stores';
	import { FileText } from 'lucide-svelte';

	let { data }: { data: PageData } = $props();

	function openDoc(docPath: string) {
		const p = $page.params as Record<string, string>;
		goto(`/nodes/${p['nodeId']}/projects/${p['projectId']}/documents/${docPath}`);
	}
</script>

<div class="space-y-4">
	<div class="flex items-center justify-between">
		<h1 class="text-headline-md font-headline-md text-fg-ink dark:text-dark-fg-ink">Documents</h1>
		<span class="text-body-sm text-fg-muted dark:text-dark-fg-muted">
			{data.docs.totalCount} total
		</span>
	</div>

	<div class="overflow-hidden border border-border-line dark:border-dark-border-line">
		<table class="w-full text-sm">
			<thead>
				<tr class="border-b border-border-line bg-bg-surface dark:border-dark-border-line dark:bg-dark-bg-surface">
					<th class="px-4 py-3 text-left text-label-caps font-label-caps uppercase tracking-wide text-fg-muted dark:text-dark-fg-muted">Title</th>
					<th class="px-4 py-3 text-left text-label-caps font-label-caps uppercase tracking-wide text-fg-muted dark:text-dark-fg-muted">Path</th>
				</tr>
			</thead>
			<tbody>
				{#each data.docs.edges as edge (edge.cursor)}
					<tr
						onclick={() => openDoc(edge.node.path)}
						class="cursor-pointer border-b border-border-line last:border-0 hover:bg-bg-surface dark:border-dark-border-line dark:hover:bg-dark-bg-surface"
					>
						<td class="px-4 py-3 text-fg-ink dark:text-dark-fg-ink">
							<div class="flex items-center gap-2">
								<FileText class="h-4 w-4 shrink-0 text-fg-muted dark:text-dark-fg-muted" />
								{edge.node.title}
							</div>
						</td>
						<td class="px-4 py-3 font-mono-code text-mono-code text-fg-muted dark:text-dark-fg-muted">
							{edge.node.path}
						</td>
					</tr>
				{/each}
				{#if data.docs.edges.length === 0}
					<tr>
						<td colspan="2" class="px-4 py-8 text-center text-body-sm text-fg-muted dark:text-dark-fg-muted">
							No documents found.
						</td>
					</tr>
				{/if}
			</tbody>
		</table>
	</div>
</div>
