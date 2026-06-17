<script lang="ts">
	import type { LayoutData } from './$types';
	import type { Snippet } from 'svelte';
	import { goto } from '$app/navigation';
	import { page } from '$app/stores';
	import { createClient } from '$lib/gql/client';
	import { gql } from 'graphql-request';

	let { data, children }: { data: LayoutData; children: Snippet } = $props();

	const activeWorker = $derived(($page.params as Record<string, string>)['workerId'] ?? null);
	const client = createClient();

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

	let workerEdges = $state<typeof data.workers.edges>([]);
	let startingProjectId = $state<string | null>(null);
	let stoppingWorkerId = $state<string | null>(null);
	let actionError = $state<string | null>(null);

	$effect(() => {
		workerEdges = data.workers.edges;
	});

	function openWorker(workerId: string) {
		const p = $page.params as Record<string, string>;
		goto(`/nodes/${p['nodeId']}/workers/${workerId}`);
	}

	function workerProjectName(worker: typeof data.workers.edges[0]['node']): string | null {
		// Use embedded projectName if present (test mocks / extended API responses)
		if (worker.projectName) return worker.projectName;
		// Fall back to path-keyed lookup from separately fetched projects
		if (worker.projectRoot) return data.projectsByPath[worker.projectRoot]?.name ?? null;
		return null;
	}

	function workerNodeName(worker: typeof data.workers.edges[0]['node']): string | null {
		return worker.nodeName ?? null;
	}

	function projectNode(project: (typeof data.projects)[0]) {
		if (project.nodeId) {
			const byId = data.federationNodes.find((n) => n.nodeId === project.nodeId);
			if (byId) return byId;
		}
		if (data.federationNodes.length === 1) return data.federationNodes[0];
		return data.federationNodes.find((n) => n.nodeId !== data.nodeId) ?? null;
	}

	function projectStatus(project: (typeof data.projects)[0]): string {
		return projectNode(project)?.status ?? 'unknown';
	}

	function projectWritable(project: (typeof data.projects)[0]): boolean {
		const node = projectNode(project);
		return (node?.status ?? 'unknown') === 'active' && (node?.writeCapability ?? true);
	}

	function projectNodeName(project: (typeof data.projects)[0]): string {
		return projectNode(project)?.name ?? project.nodeId ?? 'Unknown node';
	}

	async function startProjectWorker(project: (typeof data.projects)[0]) {
		if (!projectWritable(project)) return;
		startingProjectId = project.id;
		actionError = null;
		try {
			const result = await client.request<{
				startWorker: { id: string; state: string; kind: string; nodeId?: string; projectId?: string };
			}>(START_WORKER_MUTATION, { input: { projectId: project.id } });
			const node = projectNode(project);
			workerEdges = [
				{
					cursor: result.startWorker.id,
					node: {
						id: result.startWorker.id,
						kind: result.startWorker.kind,
						state: result.startWorker.state,
						status: result.startWorker.state,
						harness: null,
						model: null,
						currentBead: null,
						attempts: null,
						successes: null,
						failures: null,
						startedAt: null,
						projectId: project.id,
						projectName: project.name,
						nodeId: node?.nodeId ?? project.nodeId ?? null,
						nodeName: node?.name ?? project.nodeId ?? null
					}
				},
				...workerEdges.filter((edge) => edge.node.id !== result.startWorker.id)
			];
		} catch (err) {
			actionError = err instanceof Error ? err.message : 'Unable to start worker';
		} finally {
			startingProjectId = null;
		}
	}

	async function stopWorker(event: MouseEvent, workerId: string) {
		event.stopPropagation();
		stoppingWorkerId = workerId;
		actionError = null;
		try {
			const result = await client.request<{ stopWorker: { id: string; state: string; kind: string } }>(
				STOP_WORKER_MUTATION,
				{ id: workerId }
			);
			workerEdges = workerEdges.map((edge) =>
				edge.node.id === workerId
					? {
							...edge,
							node: {
								...edge.node,
								state: result.stopWorker.state,
								status: result.stopWorker.state,
								kind: result.stopWorker.kind
							}
						}
					: edge
			);
		} catch (err) {
			actionError = err instanceof Error ? err.message : 'Unable to stop worker';
		} finally {
			stoppingWorkerId = null;
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

	function scopeParam(): string {
		const s = $page.url.searchParams.get('scope');
		return s ? `?scope=${encodeURIComponent(s)}` : '';
	}

	function federationHref(): string {
		const p = $page.params as Record<string, string>;
		return `/nodes/${p['nodeId']}/workers?scope=federation`;
	}

	function localHref(): string {
		const p = $page.params as Record<string, string>;
		return `/nodes/${p['nodeId']}/workers`;
	}
</script>

<div class="space-y-4">
	<div class="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
		<div>
			<h1 class="text-headline-lg font-headline-lg text-fg-ink dark:text-dark-fg-ink">Workers</h1>
			<p class="mt-1 max-w-2xl text-body-sm text-fg-muted dark:text-dark-fg-muted">
				All workers across registered projects on this node.
			</p>
		</div>
		<div class="flex items-center gap-3">
			<span class="text-body-sm text-fg-muted dark:text-dark-fg-muted">
				{data.workers.totalCount} total
			</span>
			<div class="flex items-center gap-1 border border-border-line dark:border-dark-border-line">
				<a
					href={localHref()}
					class="px-3 py-1.5 text-body-sm font-medium {data.scope === 'local'
						? 'bg-accent-lever text-white dark:bg-dark-accent-lever'
						: 'text-fg-muted hover:bg-bg-surface dark:text-dark-fg-muted dark:hover:bg-dark-bg-surface'}"
				>
					Local
				</a>
				<a
					href={federationHref()}
					class="px-3 py-1.5 text-body-sm font-medium {data.scope === 'federation'
						? 'bg-accent-lever text-white dark:bg-dark-accent-lever'
						: 'text-fg-muted hover:bg-bg-surface dark:text-dark-fg-muted dark:hover:bg-dark-bg-surface'}"
				>
					Federation
				</a>
			</div>
		</div>
	</div>

	{#if actionError}
		<div class="border border-status-failed/40 bg-status-failed/10 px-3 py-2 text-body-sm text-status-failed">
			{actionError}
		</div>
	{/if}

	{#if data.scope === 'federation' && data.projects.length > 0}
		<div class="border border-border-line dark:border-dark-border-line">
			<div class="border-b border-border-line bg-bg-surface px-4 py-2 text-label-caps font-label-caps uppercase tracking-wide text-fg-muted dark:border-dark-border-line dark:bg-dark-bg-surface dark:text-dark-fg-muted">
				Projects
			</div>
			<div class="divide-y divide-border-line dark:divide-dark-border-line">
				{#each data.projects as project (project.id)}
					{@const status = projectStatus(project)}
					<div
						data-testid="worker-project-row"
						class="flex flex-col gap-3 px-4 py-3 sm:flex-row sm:items-center sm:justify-between"
					>
						<div class="min-w-0">
							<div class="flex flex-wrap items-center gap-2">
								<span class="font-medium text-fg-ink dark:text-dark-fg-ink">{project.name}</span>
								<span
									data-testid="federation-status-badge"
									class="border border-border-line px-2 py-0.5 text-label-caps font-label-caps uppercase tracking-wide text-fg-muted dark:border-dark-border-line dark:text-dark-fg-muted"
								>
									{status}
								</span>
								<span class="text-body-sm text-fg-muted dark:text-dark-fg-muted">
									{projectNodeName(project)}
								</span>
							</div>
							<div class="truncate font-mono-code text-mono-code text-fg-muted dark:text-dark-fg-muted">
								{project.path}
							</div>
						</div>
						<button
							type="button"
							class="border border-border-line px-3 py-1.5 text-body-sm font-medium text-fg-ink hover:bg-bg-surface disabled:cursor-not-allowed disabled:opacity-50 dark:border-dark-border-line dark:text-dark-fg-ink dark:hover:bg-dark-bg-surface"
							disabled={!projectWritable(project) || startingProjectId === project.id}
							onclick={() => startProjectWorker(project)}
						>
							{startingProjectId === project.id ? 'Starting' : 'Start worker'}
						</button>
					</div>
				{/each}
			</div>
		</div>
	{/if}

	<div class="overflow-hidden border border-border-line dark:border-dark-border-line">
		<table class="w-full text-body-sm">
			<thead>
				<tr class="border-b border-border-line bg-bg-surface dark:border-dark-border-line dark:bg-dark-bg-surface">
					{#if data.scope === 'federation'}
						<th class="px-4 py-3 text-left text-label-caps font-label-caps uppercase tracking-wide text-fg-muted dark:text-dark-fg-muted">Node</th>
					{:else}
						<th class="px-4 py-3 text-left text-label-caps font-label-caps uppercase tracking-wide text-fg-muted dark:text-dark-fg-muted">Project</th>
					{/if}
					<th class="px-4 py-3 text-left text-label-caps font-label-caps uppercase tracking-wide text-fg-muted dark:text-dark-fg-muted">ID</th>
					<th class="px-4 py-3 text-left text-label-caps font-label-caps uppercase tracking-wide text-fg-muted dark:text-dark-fg-muted">Kind</th>
					<th class="px-4 py-3 text-left text-label-caps font-label-caps uppercase tracking-wide text-fg-muted dark:text-dark-fg-muted">State</th>
					<th class="px-4 py-3 text-left text-label-caps font-label-caps uppercase tracking-wide text-fg-muted dark:text-dark-fg-muted">Current Bead</th>
					<th class="px-4 py-3 text-left text-label-caps font-label-caps uppercase tracking-wide text-fg-muted dark:text-dark-fg-muted">Model</th>
					<th class="px-4 py-3 text-right text-label-caps font-label-caps uppercase tracking-wide text-fg-muted dark:text-dark-fg-muted">Attempts</th>
					<th class="px-4 py-3 text-right text-label-caps font-label-caps uppercase tracking-wide text-fg-muted dark:text-dark-fg-muted">Actions</th>
				</tr>
			</thead>
			<tbody>
				{#each workerEdges as edge (edge.cursor)}
					{@const projectName = workerProjectName(edge.node)}
					{@const nodeName = workerNodeName(edge.node)}
					<tr
						data-testid="worker-row"
						data-state={edge.node.state}
						onclick={() => openWorker(edge.node.id)}
						class="cursor-pointer border-b border-border-line last:border-0 hover:bg-bg-surface dark:border-dark-border-line dark:hover:bg-dark-bg-surface {activeWorker ===
						edge.node.id
							? 'bg-accent-lever/10 dark:bg-dark-accent-lever/10'
							: ''}"
					>
						<td class="px-4 py-3">
							{#if data.scope === 'federation' && nodeName}
								<span data-testid="worker-node-badge" class="inline-flex items-center border border-border-line px-2 py-0.5 text-label-caps font-label-caps uppercase tracking-wide text-fg-muted dark:border-dark-border-line dark:text-dark-fg-muted">
									{nodeName}
								</span>
							{:else if projectName}
								<span class="inline-flex items-center border border-border-line px-2 py-0.5 text-label-caps font-label-caps uppercase tracking-wide text-fg-muted dark:border-dark-border-line dark:text-dark-fg-muted">
									{projectName}
								</span>
							{:else}
								<span class="text-fg-muted dark:text-dark-fg-muted">—</span>
							{/if}
						</td>
						<td class="px-4 py-3 font-mono-code text-mono-code text-accent-lever dark:text-dark-accent-lever">
							{edge.node.id}
						</td>
						<td class="px-4 py-3 text-fg-ink dark:text-dark-fg-ink">
							{edge.node.kind}
						</td>
						<td class="px-4 py-3">
							<span data-testid="worker-state-badge" class="font-medium {stateClass(edge.node.state)}">
								{edge.node.state}
							</span>
						</td>
						<td class="px-4 py-3 font-mono-code text-mono-code text-fg-muted dark:text-dark-fg-muted">
							{edge.node.currentBead ?? '—'}
						</td>
						<td class="px-4 py-3 text-fg-muted dark:text-dark-fg-muted">
							{edge.node.model ?? edge.node.harness ?? '—'}
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
							<button
								type="button"
								class="border border-border-line px-2 py-1 text-body-sm text-fg-ink hover:bg-bg-surface disabled:cursor-not-allowed disabled:opacity-50 dark:border-dark-border-line dark:text-dark-fg-ink dark:hover:bg-dark-bg-surface"
								disabled={stoppingWorkerId === edge.node.id || edge.node.state === 'stopped'}
								onclick={(event) => stopWorker(event, edge.node.id)}
							>
								{stoppingWorkerId === edge.node.id ? 'Stopping' : 'Stop'}
							</button>
						</td>
					</tr>
				{/each}
				{#if workerEdges.length === 0}
					<tr>
						<td
							colspan="8"
							class="px-4 py-8 text-center text-body-sm text-fg-muted dark:text-dark-fg-muted"
						>
							No workers found on this node.
						</td>
					</tr>
				{/if}
			</tbody>
		</table>
	</div>
</div>

{@render children()}
