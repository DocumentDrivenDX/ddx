<script lang="ts">
	import type { LayoutData } from './$types';
	import type { Snippet } from 'svelte';
	import { goto } from '$app/navigation';
	import { page } from '$app/stores';

	let { data, children }: { data: LayoutData; children: Snippet } = $props();

	const activeWorker = $derived(($page.params as Record<string, string>)['workerId'] ?? null);

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
				</tr>
			</thead>
			<tbody>
				{#each data.workers.edges as edge (edge.cursor)}
					{@const projectName = workerProjectName(edge.node)}
					{@const nodeName = workerNodeName(edge.node)}
					<tr
						onclick={() => openWorker(edge.node.id)}
						class="cursor-pointer border-b border-border-line last:border-0 hover:bg-bg-surface dark:border-dark-border-line dark:hover:bg-dark-bg-surface {activeWorker ===
						edge.node.id
							? 'bg-accent-lever/10 dark:bg-dark-accent-lever/10'
							: ''}"
					>
						<td class="px-4 py-3">
							{#if data.scope === 'federation' && nodeName}
								<span class="inline-flex items-center border border-border-line px-2 py-0.5 text-label-caps font-label-caps uppercase tracking-wide text-fg-muted dark:border-dark-border-line dark:text-dark-fg-muted">
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
							<span class="font-medium {stateClass(edge.node.state)}">
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
					</tr>
				{/each}
				{#if data.workers.edges.length === 0}
					<tr>
						<td
							colspan="7"
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
