<script lang="ts">
	import type { PageData } from './$types';
	import { page } from '$app/stores';
	import { goto } from '$app/navigation';

	let { data }: { data: PageData } = $props();

	function fmtDate(iso: string): string {
		return new Date(iso).toLocaleString();
	}

	function goNext() {
		const cursor = data.commits.pageInfo.endCursor;
		if (cursor) {
			const p = $page.params as Record<string, string>;
			goto(
				`/nodes/${p['nodeId']}/projects/${p['projectId']}/commits?after=${encodeURIComponent(cursor)}`
			);
		}
	}

	function goPrev() {
		const p = $page.params as Record<string, string>;
		goto(`/nodes/${p['nodeId']}/projects/${p['projectId']}/commits`);
	}

	function openBead(beadId: string) {
		const p = $page.params as Record<string, string>;
		goto(`/nodes/${p['nodeId']}/projects/${p['projectId']}/beads/${beadId}`);
	}
</script>

<div class="space-y-4">
	<div class="flex items-center justify-between">
		<h1 class="text-xl font-semibold dark:text-white">Commits</h1>
		<span class="text-sm text-gray-500 dark:text-gray-400">
			{data.commits.totalCount} total
		</span>
	</div>

	<div class="overflow-hidden rounded-lg border border-gray-200 dark:border-gray-700">
		<table class="w-full text-sm">
			<thead>
				<tr class="border-b border-gray-200 bg-gray-50 dark:border-gray-700 dark:bg-gray-800">
					<th class="px-4 py-3 text-left font-medium text-gray-600 dark:text-gray-300">SHA</th>
					<th class="px-4 py-3 text-left font-medium text-gray-600 dark:text-gray-300">Subject</th>
					<th class="px-4 py-3 text-left font-medium text-gray-600 dark:text-gray-300">Author</th>
					<th class="px-4 py-3 text-left font-medium text-gray-600 dark:text-gray-300">Date</th>
					<th class="px-4 py-3 text-left font-medium text-gray-600 dark:text-gray-300">Beads</th>
				</tr>
			</thead>
			<tbody>
				{#each data.commits.edges as edge (edge.cursor)}
					{@const c = edge.node}
					<tr class="border-b border-gray-100 last:border-0 dark:border-gray-700">
						<td class="px-4 py-3 font-mono text-xs text-gray-500 dark:text-gray-400">
							{c.shortSha}
						</td>
						<td class="px-4 py-3 text-gray-900 dark:text-gray-100">
							<span title={c.body ?? undefined}>{c.subject}</span>
						</td>
						<td class="px-4 py-3 text-xs text-gray-500 dark:text-gray-400">
							{c.author}
						</td>
						<td class="px-4 py-3 text-xs text-gray-500 dark:text-gray-400">
							{fmtDate(c.date)}
						</td>
						<td class="px-4 py-3">
							{#if c.beadRefs && c.beadRefs.length > 0}
								<div class="flex flex-wrap gap-1">
									{#each c.beadRefs as beadId}
										<button
											onclick={(e) => {
												e.stopPropagation();
												openBead(beadId);
											}}
											class="rounded bg-blue-100 px-1.5 py-0.5 font-mono text-xs text-blue-700 hover:bg-blue-200 dark:bg-blue-900/40 dark:text-blue-300 dark:hover:bg-blue-900/60"
										>
											{beadId.slice(0, 8)}
										</button>
									{/each}
								</div>
							{:else}
								<span class="text-gray-300 dark:text-gray-600">—</span>
							{/if}
						</td>
					</tr>
				{/each}
				{#if data.commits.edges.length === 0}
					<tr>
						<td colspan="5" class="px-4 py-8 text-center text-gray-400 dark:text-gray-600">
							No commits found.
						</td>
					</tr>
				{/if}
			</tbody>
		</table>
	</div>

	<!-- Pagination -->
	<div class="flex items-center justify-between">
		<button
			onclick={goPrev}
			disabled={!data.after}
			class="rounded border border-gray-200 px-3 py-1.5 text-sm text-gray-600 hover:bg-gray-50 disabled:cursor-not-allowed disabled:opacity-40 dark:border-gray-700 dark:text-gray-300 dark:hover:bg-gray-800"
		>
			← Previous
		</button>
		<span class="text-xs text-gray-400 dark:text-gray-500">
			{data.commits.edges.length} commits shown
		</span>
		<button
			onclick={goNext}
			disabled={!data.commits.pageInfo.hasNextPage}
			class="rounded border border-gray-200 px-3 py-1.5 text-sm text-gray-600 hover:bg-gray-50 disabled:cursor-not-allowed disabled:opacity-40 dark:border-gray-700 dark:text-gray-300 dark:hover:bg-gray-800"
		>
			Next →
		</button>
	</div>
</div>
