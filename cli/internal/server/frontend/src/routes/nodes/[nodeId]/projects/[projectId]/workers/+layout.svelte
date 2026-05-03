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
		return `/nodes/${p['nodeId']}/projects/${p['projectId']}/runs?layer=run`;
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
				return 'text-status-running';
			case 'idle':
				return 'text-status-open';
			case 'stopped':
				return 'text-fg-muted dark:text-dark-fg-muted';
			case 'error':
				return 'text-status-failed';
			default:
				return 'text-fg-muted dark:text-dark-fg-muted';
		}
	}
</script>

<div class="space-y-4">
	<!-- Drain-worker count control (ddx-b6cf025c). Dispatches a default-spec
	     worker; server enforces workers.default_spec + workers.max_count. -->
	<div
		data-testid="drain-count-panel"
		class="flex flex-col gap-3 border border-border-line bg-bg-surface p-4 text-body-sm dark:border-dark-border-line dark:bg-dark-bg-surface sm:flex-row sm:items-center sm:justify-between"
	>
		<div>
			<div class="text-label-caps font-label-caps uppercase tracking-wide text-fg-muted dark:text-dark-fg-muted">
				Drain workers
			</div>
			<div
				data-testid="drain-worker-count"
				class="mt-1 text-headline-lg font-headline-lg text-fg-ink dark:text-dark-fg-ink"
			>
				{runningDrainCount}
			</div>
			<p class="mt-1 text-body-sm text-fg-muted dark:text-dark-fg-muted">
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
				class="border border-accent-lever bg-accent-lever px-3 py-1.5 text-body-sm font-medium text-white hover:opacity-90 disabled:cursor-not-allowed disabled:opacity-60 dark:border-dark-accent-lever dark:bg-dark-accent-lever"
				aria-label="Add worker"
			>
				{adding ? '…' : '+ Add worker'}
			</button>
			<button
				type="button"
				data-testid="remove-drain-worker"
				onclick={() => void removeDrainWorker()}
				disabled={removing || runningDrainCount === 0}
				class="border border-border-line px-3 py-1.5 text-body-sm font-medium text-error hover:bg-bg-canvas disabled:cursor-not-allowed disabled:opacity-60 dark:border-dark-border-line dark:text-dark-error dark:hover:bg-dark-bg-surface"
				aria-label="Remove worker"
			>
				{removing ? '…' : '− Remove worker'}
			</button>
		</div>
	</div>

	<div class="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
		<div>
			<h1 class="text-headline-lg font-headline-lg text-fg-ink dark:text-dark-fg-ink">Workers</h1>
			<p class="mt-1 max-w-2xl text-body-sm text-fg-muted dark:text-dark-fg-muted">
				Workers drain the bead queue as long-lived processes; Sessions are the history of
				what they ran.
			</p>
			<a class="mt-2 inline-flex text-body-sm text-accent-lever hover:underline dark:text-dark-accent-lever" href={sessionsHref()}>
				Recent sessions →
			</a>
		</div>
		<div class="flex items-center gap-3">
			<span class="text-body-sm text-fg-muted dark:text-dark-fg-muted">
				{data.workers.totalCount} total
			</span>
			<button
				type="button"
				onclick={() => {
					actionError = null;
					showStartForm = !showStartForm;
				}}
				class="border border-accent-lever bg-accent-lever px-3 py-1.5 text-body-sm font-medium text-white hover:opacity-90 disabled:cursor-not-allowed disabled:opacity-60 dark:border-dark-accent-lever dark:bg-dark-accent-lever"
			>
				Start worker
			</button>
		</div>
	</div>

	{#if showStartForm}
		<form
			class="grid gap-3 border border-border-line bg-bg-surface p-4 text-body-sm dark:border-dark-border-line dark:bg-dark-bg-surface sm:grid-cols-4"
			onsubmit={(event) => {
				event.preventDefault();
				void startWorker();
			}}
		>
			<label class="space-y-1">
				<span class="text-label-caps font-label-caps uppercase tracking-wide text-fg-muted dark:text-dark-fg-muted">Harness</span>
				<input
					bind:value={harness}
					placeholder="auto"
					class="w-full border border-border-line bg-bg-elevated px-2 py-1.5 text-fg-ink dark:border-dark-border-line dark:bg-dark-bg-elevated dark:text-dark-fg-ink"
				/>
			</label>
			<label class="space-y-1">
				<span class="text-label-caps font-label-caps uppercase tracking-wide text-fg-muted dark:text-dark-fg-muted">Profile</span>
				<select
					bind:value={profile}
					required
					class="w-full border border-border-line bg-bg-elevated px-2 py-1.5 text-fg-ink dark:border-dark-border-line dark:bg-dark-bg-elevated dark:text-dark-fg-ink"
				>
					<option value="cheap">cheap</option>
					<option value="fast">fast</option>
					<option value="smart">smart</option>
				</select>
			</label>
			<label class="space-y-1">
				<span class="text-label-caps font-label-caps uppercase tracking-wide text-fg-muted dark:text-dark-fg-muted">Effort</span>
				<select
					bind:value={effort}
					required
					class="w-full border border-border-line bg-bg-elevated px-2 py-1.5 text-fg-ink dark:border-dark-border-line dark:bg-dark-bg-elevated dark:text-dark-fg-ink"
				>
					<option value="low">low</option>
					<option value="medium">medium</option>
					<option value="high">high</option>
				</select>
			</label>
			<label class="space-y-1">
				<span class="text-label-caps font-label-caps uppercase tracking-wide text-fg-muted dark:text-dark-fg-muted">Label filter</span>
				<input
					bind:value={labelFilter}
					placeholder="optional"
					class="w-full border border-border-line bg-bg-elevated px-2 py-1.5 text-fg-ink dark:border-dark-border-line dark:bg-dark-bg-elevated dark:text-dark-fg-ink"
				/>
			</label>
			<div class="flex items-end gap-2 sm:col-span-4">
				<button
					type="submit"
					disabled={starting || !profile.trim() || !effort.trim()}
					class="border border-fg-ink bg-fg-ink px-3 py-1.5 text-body-sm font-medium text-bg-canvas hover:opacity-90 disabled:cursor-not-allowed disabled:opacity-60 dark:border-dark-fg-ink dark:bg-dark-fg-ink dark:text-dark-bg-canvas"
				>
					{starting ? 'Starting…' : 'Start'}
				</button>
				<button
					type="button"
					onclick={() => (showStartForm = false)}
					class="px-3 py-1.5 text-body-sm text-fg-muted hover:bg-bg-canvas dark:text-dark-fg-muted dark:hover:bg-dark-bg-surface"
				>
					Cancel
				</button>
			</div>
		</form>
	{/if}

	{#if actionError}
		<div class="border border-border-line bg-bg-surface px-3 py-2 text-body-sm text-status-failed dark:border-dark-border-line dark:bg-dark-bg-surface">
			{actionError}
		</div>
	{/if}

	<div class="overflow-hidden border border-border-line dark:border-dark-border-line">
		<table class="w-full text-body-sm">
			<thead>
				<tr class="border-b border-border-line bg-bg-surface dark:border-dark-border-line dark:bg-dark-bg-surface">
					<th class="px-4 py-3 text-left text-label-caps font-label-caps uppercase tracking-wide text-fg-muted dark:text-dark-fg-muted">ID</th>
					<th class="px-4 py-3 text-left text-label-caps font-label-caps uppercase tracking-wide text-fg-muted dark:text-dark-fg-muted">Kind</th>
					<th class="px-4 py-3 text-left text-label-caps font-label-caps uppercase tracking-wide text-fg-muted dark:text-dark-fg-muted"
						>State / Phase</th
					>
					<th class="px-4 py-3 text-left text-label-caps font-label-caps uppercase tracking-wide text-fg-muted dark:text-dark-fg-muted"
						>Current Bead</th
					>
					<th class="px-4 py-3 text-right text-label-caps font-label-caps uppercase tracking-wide text-fg-muted dark:text-dark-fg-muted"
						>Attempts</th
					>
					<th class="px-4 py-3 text-right text-label-caps font-label-caps uppercase tracking-wide text-fg-muted dark:text-dark-fg-muted">Actions</th>
				</tr>
			</thead>
			<tbody>
				{#each data.workers.edges as edge (edge.cursor)}
					<tr
						onclick={() => openWorker(edge.node.id)}
						class="cursor-pointer border-b border-border-line last:border-0 hover:bg-bg-surface dark:border-dark-border-line dark:hover:bg-dark-bg-surface {activeWorker ===
						edge.node.id
							? 'bg-accent-lever/10 dark:bg-dark-accent-lever/10'
							: ''}"
					>
						<td class="px-4 py-3 font-mono-code text-mono-code text-accent-lever dark:text-dark-accent-lever">
							{edge.node.id.slice(0, 8)}
						</td>
						<td class="px-4 py-3 text-fg-ink dark:text-dark-fg-ink">
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
						<td class="px-4 py-3 font-mono-code text-mono-code text-fg-muted dark:text-dark-fg-muted">
							{edge.node.currentBead ?? '—'}
						</td>
						<td class="px-4 py-3 text-right text-fg-muted dark:text-dark-fg-muted">
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
									class="border border-border-line px-2 py-1 text-xs font-medium text-status-failed hover:bg-bg-canvas disabled:cursor-not-allowed disabled:opacity-60 dark:border-dark-border-line dark:hover:bg-dark-bg-surface"
								>
									{stoppingId === edge.node.id ? 'Stopping…' : 'Stop'}
								</button>
							{:else}
								<span class="text-mono-code text-fg-muted dark:text-dark-fg-muted">—</span>
							{/if}
						</td>
					</tr>
				{/each}
				{#if data.workers.edges.length === 0}
					<tr>
						<td colspan="6" class="px-4 py-8 text-center text-body-sm text-fg-muted dark:text-dark-fg-muted">
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
