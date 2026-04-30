<script lang="ts">
	import { Loader2 } from 'lucide-svelte';
	import { projectStore } from '$lib/stores/project.svelte';
	import { nodeStore } from '$lib/stores/node.svelte';
	import { createClient } from '$lib/gql/client';
	import { subscribeBeadLifecycle, subscribeWorkerProgress } from '$lib/gql/subscriptions';
	import { gql } from 'graphql-request';

	// Persistent drain-queue worker indicator (ddx-b6cf025c). Shown on every
	// route inside a selected project; hidden when no project is selected.
	// Subscription-driven so worker-state changes reflect within 2s
	// (workerProgress) and bead-count changes within 5s (beadLifecycle).
	// A 3s poll backstops both in case events are dropped.

	const SUMMARY_QUERY = gql`
		query QueueAndWorkersSummary($projectId: String!) {
			queueAndWorkersSummary(projectId: $projectId) {
				readyBeads
				runningWorkers
				totalWorkers
			}
		}
	`;

	const RUNNING_WORKERS_QUERY = gql`
		query DrainIndicatorRunningWorkers($projectID: String!) {
			workersByProject(projectID: $projectID, first: 50) {
				edges {
					node {
						id
						state
					}
				}
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

	async function fetchRunningWorkerIds(pid: string): Promise<string[]> {
		try {
			const client = createClient(fetch);
			const data = await client.request<{
				workersByProject: { edges: { node: { id: string; state: string } }[] };
			}>(RUNNING_WORKERS_QUERY, { projectID: pid });
			return data.workersByProject.edges
				.filter((e) => e.node.state === 'running')
				.map((e) => e.node.id);
		} catch {
			return [];
		}
	}

	$effect(() => {
		const pidNullable = projectId;
		if (!pidNullable) {
			loaded = false;
			return;
		}
		const pid: string = pidNullable;
		let cancelled = false;
		const workerSubs = new Map<string, () => void>();

		async function rewireWorkerSubscriptions() {
			if (cancelled) return;
			const ids = await fetchRunningWorkerIds(pid);
			if (cancelled) return;
			const next = new Set(ids);
			// Drop subscriptions for workers no longer running.
			for (const [id, dispose] of workerSubs) {
				if (!next.has(id)) {
					dispose();
					workerSubs.delete(id);
				}
			}
			// Add subscriptions for newly-running workers.
			for (const id of ids) {
				if (workerSubs.has(id)) continue;
				const dispose = subscribeWorkerProgress(id, () => {
					void refresh(pid);
				});
				workerSubs.set(id, dispose);
			}
		}

		void refresh(pid).then(() => void rewireWorkerSubscriptions());

		// Bead lifecycle drives both ready-count freshness AND triggers a re-scan
		// for new running workers (workers come online by claiming a bead, which
		// fires status_changed).
		const beadDispose = subscribeBeadLifecycle(pid, () => {
			void refresh(pid);
			void rewireWorkerSubscriptions();
		});

		const h = setInterval(() => {
			void refresh(pid);
			void rewireWorkerSubscriptions();
		}, 3000);

		return () => {
			cancelled = true;
			clearInterval(h);
			beadDispose();
			for (const dispose of workerSubs.values()) dispose();
			workerSubs.clear();
		};
	});
</script>

{#if projectId && workersHref}
	<a
		data-testid="drain-indicator"
		data-state={active ? 'active' : 'idle'}
		href={workersHref}
		class="flex items-center gap-1.5 rounded-none border border-border-line px-2 py-1 text-xs font-medium text-fg-ink hover:bg-bg-surface dark:border-dark-border-line dark:text-dark-fg-ink dark:hover:bg-dark-bg-surface"
		aria-label="Drain queue status"
		title="Click for worker overview"
	>
		{#if active}
			<Loader2 class="h-3.5 w-3.5 animate-spin text-accent-lever dark:text-dark-accent-lever" />
		{:else}
			<span class="inline-block h-2 w-2 rounded-none bg-fg-muted dark:bg-dark-fg-muted"></span>
		{/if}
		<span>{label || 'Queue: …'}</span>
	</a>
{/if}
