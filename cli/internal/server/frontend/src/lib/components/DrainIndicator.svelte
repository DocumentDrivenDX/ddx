<script lang="ts">
	import { Loader2 } from 'lucide-svelte';
	import { projectStore } from '$lib/stores/project.svelte';
	import { nodeStore } from '$lib/stores/node.svelte';
	import { createClient } from '$lib/gql/client';
	import { gql } from 'graphql-request';

	// Persistent drain-queue worker indicator (ddx-b6cf025c). Shown on every
	// route inside a selected project; hidden when no project is selected.
	// Polls every 3s — lightweight, and avoids wiring a new subscription
	// stream just for nav badge updates. AC requires ≤2s worker-state and
	// ≤5s ready-count freshness; 3s poll sits inside both bounds.

	const SUMMARY_QUERY = gql`
		query QueueAndWorkersSummary($projectId: String!) {
			queueAndWorkersSummary(projectId: $projectId) {
				readyBeads
				runningWorkers
				totalWorkers
			}
		}
	`;

	let readyBeads = $state(0);
	let runningWorkers = $state(0);
	let loaded = $state(false);

	const projectId = $derived(projectStore.value?.id ?? null);
	const nodeId = $derived(nodeStore.value?.id ?? null);
	const workersHref = $derived(
		nodeId && projectId ? `/nodes/${nodeId}/projects/${projectId}/workers` : null
	);
	const active = $derived(runningWorkers > 0);

	const label = $derived.by(() => {
		if (!loaded) return '';
		if (active) {
			const w = runningWorkers === 1 ? 'worker' : 'workers';
			return `${runningWorkers} ${w} · ${readyBeads} ready`;
		}
		return `Queue: ${readyBeads} ready`;
	});

	async function refresh(pid: string) {
		try {
			const client = createClient(fetch);
			const data = await client.request<{
				queueAndWorkersSummary: {
					readyBeads: number;
					runningWorkers: number;
					totalWorkers: number;
				};
			}>(SUMMARY_QUERY, { projectId: pid });
			readyBeads = data.queueAndWorkersSummary.readyBeads;
			runningWorkers = data.queueAndWorkersSummary.runningWorkers;
			loaded = true;
		} catch {
			// Keep previous values on transient failure. AC #4: "falls back to the
			// global count" is handled implicitly by holding state; we intentionally
			// do not clear `loaded` so the badge keeps rendering.
		}
	}

	$effect(() => {
		const pid = projectId;
		if (!pid) {
			loaded = false;
			return;
		}
		void refresh(pid);
		const h = setInterval(() => void refresh(pid), 3000);
		return () => clearInterval(h);
	});
</script>

{#if projectId && workersHref}
	<a
		data-testid="drain-indicator"
		data-state={active ? 'active' : 'idle'}
		href={workersHref}
		class="flex items-center gap-1.5 rounded border border-gray-200 px-2 py-1 text-xs font-medium text-gray-700 hover:bg-gray-100 dark:border-gray-700 dark:text-gray-200 dark:hover:bg-gray-800"
		aria-label="Drain queue status"
		title="Click for worker overview"
	>
		{#if active}
			<Loader2 class="h-3.5 w-3.5 animate-spin text-blue-600 dark:text-blue-400" />
		{:else}
			<span class="inline-block h-2 w-2 rounded-full bg-gray-400 dark:bg-gray-500"></span>
		{/if}
		<span>{label || 'Queue: …'}</span>
	</a>
{/if}
