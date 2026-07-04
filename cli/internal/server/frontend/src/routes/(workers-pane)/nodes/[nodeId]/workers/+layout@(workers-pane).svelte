<script lang="ts">
	import type { LayoutData } from './$types';
	import type { Snippet } from 'svelte';
	import { goto } from '$app/navigation';
	import { page } from '$app/stores';
	import { createClient } from '$lib/gql/client';
	import { gql } from 'graphql-request';

	let { data, children }: { data: LayoutData; children: Snippet } = $props();

	type WorkerNode = (typeof data.workers.edges)[number]['node'] & {
		projectId?: string | null;
		projectName?: string | null;
		nodeId?: string | null;
		nodeName?: string | null;
	};

	type WorkerEdge = {
		node: WorkerNode;
		cursor: string;
	};

	type FederationNode = {
		nodeId: string;
		name: string;
		status: string;
	};

	type FederationProject = {
		id: string;
		name: string;
		path: string;
		nodeId: string | null;
	};
	type FederatedLayoutData = LayoutData & {
		projectsById?: Record<string, FederationProject>;
		federationNodesById?: Record<string, FederationNode>;
	};

	const federationData = data as FederatedLayoutData;

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

	const activeWorker = $derived(($page.params as Record<string, string>)['workerId'] ?? null);
	let workerEdges = $state<WorkerEdge[]>(data.workers.edges as WorkerEdge[]);
	let actionError = $state<string | null>(null);
	let startingByProject = $state<Set<string>>(new Set());
	let stoppingByWorker = $state<Set<string>>(new Set());

	$effect(() => {
		workerEdges = data.workers.edges as WorkerEdge[];
	});

	function openWorker(workerId: string) {
		const p = $page.params as Record<string, string>;
		goto(`/nodes/${p['nodeId']}/workers/${workerId}`);
	}

	function workerProjectName(worker: WorkerNode): string | null {
		// Use embedded projectName if present (test mocks / extended API responses)
		if (worker.projectName) return worker.projectName;
		// Fall back to path-keyed lookup from separately fetched projects
		if (worker.projectRoot) return federationData.projectsByPath?.[worker.projectRoot]?.name ?? null;
		return null;
	}

	function workerNodeName(worker: WorkerNode): string | null {
		if (worker.nodeName) return worker.nodeName;
		if (worker.nodeId) {
			return federationData.federationNodesById?.[worker.nodeId]?.name ?? null;
		}
		return null;
	}

	function workerNodeStatus(worker: WorkerNode): string | null {
		if (worker.nodeId) {
			return federationData.federationNodesById?.[worker.nodeId]?.status ?? null;
		}
		return null;
	}

	function projectGroupRows(): Array<{
		project: FederationProject;
		nodeId: string | null;
		nodeName: string | null;
		nodeStatus: string | null;
		workers: WorkerNode[];
	}> {
		if (data.scope !== 'federation') return [];
		const byProject = new Map<string, WorkerNode[]>();
		for (const edge of workerEdges) {
			const worker = edge.node;
			const projectId = worker.projectId ?? worker.projectRoot ?? 'unknown';
			const list = byProject.get(projectId) ?? [];
			list.push(worker);
			byProject.set(projectId, list);
		}
		const projectEntries = Object.values(federationData.projectsById ?? {});
		const orderedProjects =
			projectEntries.length > 0
				? projectEntries
				: Array.from(byProject.entries()).map(([projectId]) => ({
						id: projectId,
						name: projectId,
						path: '',
						nodeId: null
					}));
		return orderedProjects.map((project) => {
			const workers = byProject.get(project.id) ?? [];
			const nodeId = project.nodeId ?? workers[0]?.nodeId ?? null;
			const nodeName = nodeId
				? federationData.federationNodesById?.[nodeId]?.name ?? workers[0]?.nodeName ?? null
				: workers[0]?.nodeName ?? null;
			const nodeStatus = nodeId ? federationData.federationNodesById?.[nodeId]?.status ?? null : workers[0]?.nodeId ? workerNodeStatus(workers[0]) : null;
			return {
				project: {
					id: project.id,
					name: project.name,
					path: project.path,
					nodeId: project.nodeId ?? null
				},
				nodeId,
				nodeName,
				nodeStatus,
				workers
			};
		});
	}

	function mutationError(err: unknown): string {
		return err instanceof Error ? err.message : 'Worker action failed.';
	}

	function replaceWorker(nextWorker: WorkerNode) {
		const idx = workerEdges.findIndex((edge) => edge.node.id === nextWorker.id);
		if (idx >= 0) {
			const next = workerEdges.slice();
			next[idx] = { ...next[idx], node: { ...next[idx].node, ...nextWorker } };
			workerEdges = next;
			return;
		}
		workerEdges = [{ node: nextWorker, cursor: nextWorker.id }, ...workerEdges];
	}

	async function startWorker(projectId: string) {
		startingByProject = new Set(startingByProject).add(projectId);
		actionError = null;
		try {
			const client = createClient(fetch);
			const result = await client.request<{ startWorker: WorkerNode }>(START_WORKER_MUTATION, {
				input: { projectId }
			});
			replaceWorker(result.startWorker);
		} catch (err) {
			actionError = mutationError(err);
		} finally {
			const next = new Set(startingByProject);
			next.delete(projectId);
			startingByProject = next;
		}
	}

	async function stopWorker(workerId: string) {
		stoppingByWorker = new Set(stoppingByWorker).add(workerId);
		try {
			const client = createClient(fetch);
			const result = await client.request<{ stopWorker: WorkerNode }>(STOP_WORKER_MUTATION, {
				id: workerId
			});
			replaceWorker(result.stopWorker);
		} catch (err) {
			actionError = mutationError(err);
		} finally {
			const next = new Set(stoppingByWorker);
			next.delete(workerId);
			stoppingByWorker = next;
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

	{#if data.scope === 'federation'}
		<section class="space-y-3 border border-border-line bg-bg-surface p-4 dark:border-dark-border-line dark:bg-dark-bg-surface">
			<div class="flex items-center justify-between gap-3">
				<div>
					<h2 class="text-headline-md font-headline-md text-fg-ink dark:text-dark-fg-ink">Federated projects</h2>
					<p class="text-body-sm text-fg-muted dark:text-dark-fg-muted">
						Each project row starts or stops workers on the owning node only.
					</p>
				</div>
				<span class="text-body-sm text-fg-muted dark:text-dark-fg-muted">
					{projectGroupRows().length} project{projectGroupRows().length === 1 ? '' : 's'}
				</span>
			</div>

			<div class="space-y-2">
				{#each projectGroupRows() as group (group.project.id)}
					<div
						data-testid="worker-project-row"
						class="flex flex-col gap-3 border border-border-line px-4 py-3 dark:border-dark-border-line md:flex-row md:items-center md:justify-between"
					>
						<div class="space-y-1">
							<div class="text-body-md font-medium text-fg-ink dark:text-dark-fg-ink">
								{group.project.name}
							</div>
							<div class="flex flex-wrap items-center gap-2 text-body-sm text-fg-muted dark:text-dark-fg-muted">
								<span class="font-mono-code text-mono-code">{group.project.id}</span>
								{#if group.nodeName}
									<span
										data-testid="worker-node-badge"
										class="inline-flex items-center border border-border-line px-2 py-0.5 text-label-caps font-label-caps uppercase tracking-wide text-fg-muted dark:border-dark-border-line dark:text-dark-fg-muted"
									>
										{group.nodeName}
									</span>
								{/if}
								<span
									data-testid="federation-status-badge"
									class="inline-flex items-center border px-2 py-0.5 text-label-caps font-label-caps uppercase tracking-wide {group.nodeStatus === 'offline'
										? 'border-status-failed bg-status-failed/10 text-status-failed'
										: 'border-status-running bg-status-running/10 text-status-running'}"
								>
									{group.nodeStatus ?? 'unknown'}
								</span>
							</div>
						</div>
						<div class="flex items-center gap-2">
							<button
								type="button"
								onclick={() => void startWorker(group.project.id)}
								disabled={group.nodeStatus === 'offline' || startingByProject.has(group.project.id)}
								class="border border-accent-lever bg-accent-lever px-3 py-1.5 text-body-sm font-medium text-white hover:opacity-90 disabled:cursor-not-allowed disabled:opacity-60 dark:border-dark-accent-lever dark:bg-dark-accent-lever"
							>
								{startingByProject.has(group.project.id) ? 'Starting…' : 'Start worker'}
							</button>
						</div>
					</div>
				{/each}
			</div>
		</section>
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
								<span
									data-testid="worker-node-badge"
									class="inline-flex items-center border border-border-line px-2 py-0.5 text-label-caps font-label-caps uppercase tracking-wide text-fg-muted dark:border-dark-border-line dark:text-dark-fg-muted"
								>
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
							{#if edge.node.state !== 'stopped' && edge.node.state !== 'terminated'}
								<button
									type="button"
									onclick={(event) => {
										event.stopPropagation();
										void stopWorker(edge.node.id);
									}}
									disabled={stoppingByWorker.has(edge.node.id)}
									class="border border-border-line px-2 py-1 text-xs font-medium text-status-failed hover:bg-bg-canvas disabled:cursor-not-allowed disabled:opacity-60 dark:border-dark-border-line dark:hover:bg-dark-bg-surface"
								>
									{stoppingByWorker.has(edge.node.id) ? 'Stopping…' : 'Stop'}
								</button>
							{:else}
								<span class="text-mono-code text-fg-muted dark:text-dark-fg-muted">—</span>
							{/if}
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
