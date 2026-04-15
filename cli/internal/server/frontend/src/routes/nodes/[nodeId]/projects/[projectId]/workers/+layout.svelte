<script lang="ts">
	import type { LayoutData } from './$types';
	import type { Snippet } from 'svelte';
	import { goto } from '$app/navigation';
	import { page } from '$app/stores';
	import { subscribeWorkerProgress } from '$lib/gql/subscriptions';

	let { data, children }: { data: LayoutData; children: Snippet } = $props();

	// Live phase overrides from workerProgress subscription (workerID -> phase)
	let livePhaseOverrides = $state<Map<string, string>>(new Map());

	// Subscribe to progress events for all running workers
	$effect(() => {
		const runningIds = data.workers.edges
			.filter((e) => e.node.state === 'running')
			.map((e) => e.node.id);

		livePhaseOverrides = new Map();

		const disposes = runningIds.map((workerID) =>
			subscribeWorkerProgress(workerID, (evt) => {
				const next = new Map(livePhaseOverrides);
				next.set(evt.workerID, evt.phase);
				livePhaseOverrides = next;
			})
		);

		return () => disposes.forEach((d) => d());
	});

	// The currently open worker (from child route params)
	let activeWorker = $derived(($page.params as Record<string, string>)['workerId'] ?? null);

	function openWorker(workerId: string) {
		const p = $page.params as Record<string, string>;
		goto(`/nodes/${p['nodeId']}/projects/${p['projectId']}/workers/${workerId}`);
	}

	function stateClass(state: string): string {
		switch (state) {
			case 'running':
				return 'text-green-600 dark:text-green-400';
			case 'idle':
				return 'text-blue-600 dark:text-blue-400';
			case 'stopped':
				return 'text-gray-500 dark:text-gray-400';
			case 'error':
				return 'text-red-600 dark:text-red-400';
			default:
				return 'text-gray-500 dark:text-gray-400';
		}
	}
</script>

<div class="space-y-4">
	<div class="flex items-center justify-between">
		<h1 class="text-xl font-semibold dark:text-white">Workers</h1>
		<span class="text-sm text-gray-500 dark:text-gray-400">
			{data.workers.totalCount} total
		</span>
	</div>

	<div class="overflow-hidden rounded-lg border border-gray-200 dark:border-gray-700">
		<table class="w-full text-sm">
			<thead>
				<tr class="border-b border-gray-200 bg-gray-50 dark:border-gray-700 dark:bg-gray-800">
					<th class="px-4 py-3 text-left font-medium text-gray-600 dark:text-gray-300">ID</th>
					<th class="px-4 py-3 text-left font-medium text-gray-600 dark:text-gray-300">Kind</th>
					<th class="px-4 py-3 text-left font-medium text-gray-600 dark:text-gray-300"
						>State / Phase</th
					>
					<th class="px-4 py-3 text-left font-medium text-gray-600 dark:text-gray-300"
						>Current Bead</th
					>
					<th class="px-4 py-3 text-right font-medium text-gray-600 dark:text-gray-300"
						>Attempts</th
					>
				</tr>
			</thead>
			<tbody>
				{#each data.workers.edges as edge (edge.cursor)}
					<tr
						onclick={() => openWorker(edge.node.id)}
						class="cursor-pointer border-b border-gray-100 last:border-0 hover:bg-gray-50 dark:border-gray-700 dark:hover:bg-gray-800 {activeWorker ===
						edge.node.id
							? 'bg-blue-50 dark:bg-blue-900/20'
							: ''}"
					>
						<td class="px-4 py-3 font-mono text-xs text-gray-500 dark:text-gray-400">
							{edge.node.id.slice(0, 8)}
						</td>
						<td class="px-4 py-3 text-gray-900 dark:text-gray-100">
							{edge.node.kind}
						</td>
						<td class="px-4 py-3">
							<span
								class="font-medium {stateClass(
									livePhaseOverrides.get(edge.node.id) ?? edge.node.state
								)}"
							>
								{livePhaseOverrides.get(edge.node.id) ?? edge.node.state}
							</span>
						</td>
						<td class="px-4 py-3 font-mono text-xs text-gray-500 dark:text-gray-400">
							{edge.node.currentBead ?? '—'}
						</td>
						<td class="px-4 py-3 text-right text-gray-600 dark:text-gray-300">
							{#if edge.node.attempts != null}
								<span title="{edge.node.successes ?? 0}✓ / {edge.node.failures ?? 0}✗">
									{edge.node.attempts}
								</span>
							{:else}
								—
							{/if}
						</td>
					</tr>
				{/each}
				{#if data.workers.edges.length === 0}
					<tr>
						<td colspan="5" class="px-4 py-8 text-center text-gray-400 dark:text-gray-600">
							No workers found.
						</td>
					</tr>
				{/if}
			</tbody>
		</table>
	</div>
</div>

{@render children()}
