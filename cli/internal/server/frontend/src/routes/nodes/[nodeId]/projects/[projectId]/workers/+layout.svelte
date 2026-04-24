<script lang="ts">
	import type { LayoutData } from './$types';
	import type { Snippet } from 'svelte';
	import { goto, invalidateAll } from '$app/navigation';
	import { page } from '$app/stores';
	import { createClient } from '$lib/gql/client';
	import { subscribeWorkerProgress } from '$lib/gql/subscriptions';
	import { gql } from 'graphql-request';

	let { data, children }: { data: LayoutData; children: Snippet } = $props();

	const START_WORKER_MUTATION = gql`
		mutation StartWorker($input: StartWorkerInput!) {
			startWorker(input: $input) {
				id
				state
				kind
			}
		}
	`;

	const STOP_WORKER_MUTATION = gql`
		mutation StopWorker($id: ID!) {
			stopWorker(id: $id) {
				id
				state
				kind
			}
		}
	`;

	// + Add worker dispatches a default-spec drain worker (ddx-b6cf025c). The
	// server honours .ddx/config.yaml workers.default_spec + workers.max_count.
	const ADD_WORKER_MUTATION = gql`
		mutation AddDrainWorker($projectId: String!) {
			workerDispatch(kind: "execute-loop", projectId: $projectId) {
				id
				state
				kind
			}
		}
	`;

	// Live phase overrides from workerProgress subscription (workerID -> phase)
	let livePhaseOverrides = $state<Map<string, string>>(new Map());
	let showStartForm = $state(false);
	let starting = $state(false);
	let stoppingId = $state<string | null>(null);
	let actionError = $state<string | null>(null);
	let harness = $state('');
	let profile = $state('smart');
	let effort = $state('medium');
	let labelFilter = $state('');
	let adding = $state(false);
	let removing = $state(false);

	// Drain workers: count of running execute-loop workers.
	const runningDrainCount = $derived(
		data.workers.edges.filter(
			(e) => e.node.state === 'running' && e.node.kind === 'execute-loop'
		).length
	);

	// workers.max_count safety rail (ddx-b6cf025c). Null = no cap configured.
	const maxCount = $derived(data.maxCount);
	const atMaxCount = $derived(
		typeof maxCount === 'number' && maxCount >= 0 && runningDrainCount >= maxCount
	);
	const addDisabled = $derived(adding || atMaxCount);
	const addTooltip = $derived(
		atMaxCount ? 'at workers.max_count limit' : 'Add a general-purpose drain worker'
	);

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

	function sessionsHref(): string {
		const p = $page.params as Record<string, string>;
		return `/nodes/${p['nodeId']}/projects/${p['projectId']}/sessions`;
	}

	function errorText(err: unknown): string {
		return err instanceof Error ? err.message : 'Worker action failed.';
	}

	async function startWorker() {
		actionError = null;
		if (!profile.trim() || !effort.trim()) {
			actionError = 'Profile and effort are required.';
			return;
		}
		starting = true;
		try {
			const client = createClient(fetch);
			await client.request(START_WORKER_MUTATION, {
				input: {
					projectId: data.projectId,
					harness: harness.trim() || null,
					profile: profile.trim(),
					effort: effort.trim(),
					labelFilter: labelFilter.trim() || null
				}
			});
			showStartForm = false;
			harness = '';
			labelFilter = '';
			await invalidateAll();
		} catch (err) {
			actionError = errorText(err);
		} finally {
			starting = false;
		}
	}

	async function addDrainWorker() {
		actionError = null;
		adding = true;
		try {
			const client = createClient(fetch);
			await client.request(ADD_WORKER_MUTATION, { projectId: data.projectId });
			await invalidateAll();
		} catch (err) {
			actionError = errorText(err);
		} finally {
			adding = false;
		}
	}

	async function removeDrainWorker() {
		actionError = null;
		// Find the oldest running execute-loop worker (AC #4: "stops the oldest-
		// running drain worker"). data.workers is sorted newest-first, so the
		// last matching edge is oldest.
		const runningDrain = data.workers.edges
			.filter((e) => e.node.state === 'running' && e.node.kind === 'execute-loop')
			.map((e) => e.node);
		const target = runningDrain[runningDrain.length - 1];
		if (!target) return;
		if (!window.confirm(`Stop worker ${target.id}?`)) return;
		removing = true;
		try {
			const client = createClient(fetch);
			await client.request(STOP_WORKER_MUTATION, { id: target.id });
			await invalidateAll();
		} catch (err) {
			actionError = errorText(err);
		} finally {
			removing = false;
		}
	}

	async function stopWorker(event: MouseEvent, workerId: string) {
		event.stopPropagation();
		actionError = null;
		if (!window.confirm(`Stop worker ${workerId}?`)) return;
		stoppingId = workerId;
		try {
			const client = createClient(fetch);
			await client.request(STOP_WORKER_MUTATION, { id: workerId });
			await invalidateAll();
		} catch (err) {
			actionError = errorText(err);
		} finally {
			stoppingId = null;
		}
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
	<!-- Drain-worker count control (ddx-b6cf025c). Dispatches a default-spec
	     worker; server enforces workers.default_spec + workers.max_count. -->
	<div
		data-testid="drain-count-panel"
		class="flex flex-col gap-3 rounded-lg border border-blue-200 bg-blue-50 p-4 text-sm dark:border-blue-900 dark:bg-blue-950/30 sm:flex-row sm:items-center sm:justify-between"
	>
		<div>
			<div class="text-xs font-medium uppercase tracking-wide text-blue-700 dark:text-blue-300">
				Drain workers
			</div>
			<div
				data-testid="drain-worker-count"
				class="text-3xl font-semibold text-blue-900 dark:text-blue-100"
			>
				{runningDrainCount}
			</div>
			<p class="mt-1 text-xs text-blue-800/80 dark:text-blue-200/80">
				Adds a general-purpose drain worker. Use the per-harness picker below for custom specs.
			</p>
		</div>
		<div class="flex items-center gap-2">
			<button
				type="button"
				data-testid="add-drain-worker"
				onclick={() => void addDrainWorker()}
				disabled={addDisabled}
				title={addTooltip}
				class="rounded border border-blue-700 bg-blue-600 px-3 py-1.5 text-sm font-medium text-white hover:bg-blue-700 disabled:cursor-not-allowed disabled:opacity-60 dark:border-blue-500 dark:bg-blue-500 dark:hover:bg-blue-600"
				aria-label="Add worker"
			>
				{adding ? '…' : '+ Add worker'}
			</button>
			<button
				type="button"
				data-testid="remove-drain-worker"
				onclick={() => void removeDrainWorker()}
				disabled={removing || runningDrainCount === 0}
				class="rounded border border-red-300 px-3 py-1.5 text-sm font-medium text-red-700 hover:bg-red-50 disabled:cursor-not-allowed disabled:opacity-60 dark:border-red-800 dark:text-red-300 dark:hover:bg-red-950/30"
				aria-label="Remove worker"
			>
				{removing ? '…' : '− Remove worker'}
			</button>
		</div>
	</div>

	<div class="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
		<div>
			<h1 class="text-xl font-semibold dark:text-white">Workers</h1>
			<p class="mt-1 max-w-2xl text-sm text-gray-600 dark:text-gray-300">
				Workers drain the bead queue as long-lived processes; Sessions are the history of
				what they ran.
			</p>
			<a class="mt-2 inline-flex text-sm text-blue-600 hover:underline dark:text-blue-400" href={sessionsHref()}>
				Recent sessions →
			</a>
		</div>
		<div class="flex items-center gap-3">
			<span class="text-sm text-gray-500 dark:text-gray-400">
				{data.workers.totalCount} total
			</span>
			<button
				type="button"
				onclick={() => {
					actionError = null;
					showStartForm = !showStartForm;
				}}
				class="rounded border border-blue-600 bg-blue-600 px-3 py-1.5 text-sm font-medium text-white hover:bg-blue-700 disabled:cursor-not-allowed disabled:opacity-60 dark:border-blue-500 dark:bg-blue-500 dark:hover:bg-blue-600"
			>
				Start worker
			</button>
		</div>
	</div>

	{#if showStartForm}
		<form
			class="grid gap-3 rounded-lg border border-gray-200 bg-gray-50 p-4 text-sm dark:border-gray-700 dark:bg-gray-800 sm:grid-cols-4"
			onsubmit={(event) => {
				event.preventDefault();
				void startWorker();
			}}
		>
			<label class="space-y-1">
				<span class="text-xs font-medium text-gray-600 dark:text-gray-300">Harness</span>
				<input
					bind:value={harness}
					placeholder="auto"
					class="w-full rounded border border-gray-300 bg-white px-2 py-1.5 dark:border-gray-600 dark:bg-gray-900 dark:text-gray-100"
				/>
			</label>
			<label class="space-y-1">
				<span class="text-xs font-medium text-gray-600 dark:text-gray-300">Profile</span>
				<select
					bind:value={profile}
					required
					class="w-full rounded border border-gray-300 bg-white px-2 py-1.5 dark:border-gray-600 dark:bg-gray-900 dark:text-gray-100"
				>
					<option value="cheap">cheap</option>
					<option value="fast">fast</option>
					<option value="smart">smart</option>
				</select>
			</label>
			<label class="space-y-1">
				<span class="text-xs font-medium text-gray-600 dark:text-gray-300">Effort</span>
				<select
					bind:value={effort}
					required
					class="w-full rounded border border-gray-300 bg-white px-2 py-1.5 dark:border-gray-600 dark:bg-gray-900 dark:text-gray-100"
				>
					<option value="low">low</option>
					<option value="medium">medium</option>
					<option value="high">high</option>
				</select>
			</label>
			<label class="space-y-1">
				<span class="text-xs font-medium text-gray-600 dark:text-gray-300">Label filter</span>
				<input
					bind:value={labelFilter}
					placeholder="optional"
					class="w-full rounded border border-gray-300 bg-white px-2 py-1.5 dark:border-gray-600 dark:bg-gray-900 dark:text-gray-100"
				/>
			</label>
			<div class="flex items-end gap-2 sm:col-span-4">
				<button
					type="submit"
					disabled={starting || !profile.trim() || !effort.trim()}
					class="rounded bg-gray-900 px-3 py-1.5 text-sm font-medium text-white hover:bg-black disabled:cursor-not-allowed disabled:opacity-60 dark:bg-gray-100 dark:text-gray-900"
				>
					{starting ? 'Starting…' : 'Start'}
				</button>
				<button
					type="button"
					onclick={() => (showStartForm = false)}
					class="rounded px-3 py-1.5 text-sm text-gray-600 hover:bg-gray-100 dark:text-gray-300 dark:hover:bg-gray-700"
				>
					Cancel
				</button>
			</div>
		</form>
	{/if}

	{#if actionError}
		<div class="rounded border border-red-200 bg-red-50 px-3 py-2 text-sm text-red-700 dark:border-red-900 dark:bg-red-950/40 dark:text-red-200">
			{actionError}
		</div>
	{/if}

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
					<th class="px-4 py-3 text-right font-medium text-gray-600 dark:text-gray-300">Actions</th>
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
						<td class="px-4 py-3 text-right">
							{#if edge.node.state === 'running'}
								<button
									type="button"
									onclick={(event) => stopWorker(event, edge.node.id)}
									disabled={stoppingId === edge.node.id}
									class="rounded border border-red-300 px-2 py-1 text-xs font-medium text-red-700 hover:bg-red-50 disabled:cursor-not-allowed disabled:opacity-60 dark:border-red-800 dark:text-red-300 dark:hover:bg-red-950/30"
								>
									{stoppingId === edge.node.id ? 'Stopping…' : 'Stop'}
								</button>
							{:else}
								<span class="text-xs text-gray-400 dark:text-gray-600">—</span>
							{/if}
						</td>
					</tr>
				{/each}
				{#if data.workers.edges.length === 0}
					<tr>
						<td colspan="6" class="px-4 py-8 text-center text-gray-500 dark:text-gray-400">
							No workers found. Nothing is draining this queue right now; start a worker here
							or run ddx work from a terminal.
						</td>
					</tr>
				{/if}
			</tbody>
		</table>
	</div>
</div>

{@render children()}
