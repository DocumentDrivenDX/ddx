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
		<h1 class="text-xl font-semibold dark:text-white">Documents</h1>
		<span class="text-sm text-gray-500 dark:text-gray-400">
			{data.docs.totalCount} total
		</span>
	</div>

	<div class="overflow-hidden rounded-lg border border-gray-200 dark:border-gray-700">
		<table class="w-full text-sm">
			<thead>
				<tr class="border-b border-gray-200 bg-gray-50 dark:border-gray-700 dark:bg-gray-800">
					<th class="px-4 py-3 text-left font-medium text-gray-600 dark:text-gray-300">Title</th>
					<th class="px-4 py-3 text-left font-medium text-gray-600 dark:text-gray-300">Path</th>
				</tr>
			</thead>
			<tbody>
				{#each data.docs.edges as edge (edge.cursor)}
					<tr
						onclick={() => openDoc(edge.node.path)}
						class="cursor-pointer border-b border-gray-100 last:border-0 hover:bg-gray-50 dark:border-gray-700 dark:hover:bg-gray-800"
					>
						<td class="px-4 py-3 text-gray-900 dark:text-gray-100">
							<div class="flex items-center gap-2">
								<FileText class="h-4 w-4 shrink-0 text-gray-400 dark:text-gray-500" />
								{edge.node.title}
							</div>
						</td>
						<td class="px-4 py-3 font-mono text-xs text-gray-500 dark:text-gray-400">
							{edge.node.path}
						</td>
					</tr>
				{/each}
				{#if data.docs.edges.length === 0}
					<tr>
						<td colspan="2" class="px-4 py-8 text-center text-gray-400 dark:text-gray-600">
							No documents found.
						</td>
					</tr>
				{/if}
			</tbody>
		</table>
	</div>
</div>
