<script lang="ts">
	import type { PageData } from './$types';
	import { goto } from '$app/navigation';
	import { page } from '$app/stores';
	import { createClient } from '$lib/gql/client';
	import { gql } from 'graphql-request';
	import { federationBadgeClass } from '$lib/federationStatus';

	const BEADS_QUERY = gql`
		query BeadsAllProjects($first: Int, $after: String, $status: String, $label: String, $projectID: String) {
			beads(first: $first, after: $after, status: $status, label: $label, projectID: $projectID) {
				edges {
					node {
						id
						title
						status
						priority
						labels
						projectID
					}
					cursor
				}
				pageInfo {
					hasNextPage
					endCursor
				}
				totalCount
			}
		}
	`;

	interface BeadNode {
		id: string;
		title: string;
		status: string;
		priority: number;
		labels: string[] | null;
		projectID: string | null;
	}

	interface BeadEdge {
		node: BeadNode;
		cursor: string;
	}

	interface PageInfo {
		hasNextPage: boolean;
		endCursor: string | null;
	}

	interface BeadsResult {
		beads: {
			edges: BeadEdge[];
			pageInfo: PageInfo;
			totalCount: number;
		};
	}

	const STATUS_OPTIONS = ['open', 'in-progress', 'closed', 'blocked'];

	let { data }: { data: PageData } = $props();

	let appendedEdges = $state<BeadEdge[]>([]);
	let appendedPageInfo = $state<PageInfo | null>(null);
	let loadingMore = $state(false);

	// Reset appended pages on filter change
	let filterKey = $derived(`${data.scope}::${data.activeStatus}::${data.activeLabel}::${data.activeProject}`);
	let prevFilterKey = $state('');
	$effect(() => {
		if (filterKey !== prevFilterKey) {
			prevFilterKey = filterKey;
			appendedEdges = [];
			appendedPageInfo = null;
		}
	});

	let edges = $derived([...data.beads.edges, ...appendedEdges]);
	let pageInfo = $derived<PageInfo>(appendedPageInfo ?? data.beads.pageInfo);
	let totalCount = $derived(data.beads.totalCount);

	// Derive all unique labels from current result set
	let allLabels = $derived(
		Array.from(new Set(edges.flatMap((e) => e.node.labels ?? []))).sort()
	);

	function setFilter(key: 'status' | 'label' | 'project' | 'scope', value: string | null) {
		const params = new URLSearchParams($page.url.searchParams);
		if (value === null) {
			params.delete(key);
		} else {
			params.set(key, value);
		}
		params.delete('after');
		const search = params.toString();
		goto(search ? `?${search}` : $page.url.pathname, { replaceState: false });
	}

	function toggleStatus(status: string) {
		setFilter('status', data.activeStatus === status ? null : status);
	}

	function toggleLabel(label: string) {
		setFilter('label', data.activeLabel === label ? null : label);
	}

	function toggleProject(projectId: string) {
		setFilter('project', data.activeProject === projectId ? null : projectId);
	}

	function toggleScope() {
		setFilter('scope', data.scope === 'federation' ? null : 'federation');
	}

	async function loadMore() {
		// Federated mode does not paginate (B14.6b returns the full fan-out result).
		if (data.scope === 'federation') return;
		if (!pageInfo.hasNextPage || loadingMore) return;
		loadingMore = true;
		try {
			const client = createClient();
			const result = await client.request<BeadsResult>(BEADS_QUERY, {
				first: 20,
				after: pageInfo.endCursor,
				status: data.activeStatus ?? undefined,
				label: data.activeLabel ?? undefined,
				projectID: data.activeProject ?? undefined
			});
			appendedEdges = [...appendedEdges, ...result.beads.edges];
			appendedPageInfo = result.beads.pageInfo;
		} finally {
			loadingMore = false;
		}
	}

	function statusBadgeClass(status: string): string {
		switch (status) {
			case 'open':
				return 'badge-status-open';
			case 'in-progress':
				return 'badge-status-in-progress';
			case 'closed':
				return 'badge-status-closed';
			case 'blocked':
				return 'badge-status-blocked';
			case 'running':
				return 'badge-status-running';
			case 'completed':
				return 'badge-status-completed';
			case 'failed':
				return 'badge-status-failed';
			default:
				return 'badge-status-neutral';
		}
	}

	function chipClass(active: boolean): string {
		return active
			? 'rounded-sm border px-3 py-1 text-xs font-medium border-accent-lever bg-accent-lever/10 text-accent-lever dark:border-dark-accent-lever dark:bg-dark-accent-lever/20 dark:text-dark-accent-lever'
			: 'rounded-sm border px-3 py-1 text-xs font-medium border-border-line bg-bg-elevated text-fg-ink hover:border-fg-muted hover:bg-bg-surface dark:border-dark-border-line dark:bg-dark-bg-elevated dark:text-dark-fg-ink dark:hover:bg-dark-bg-surface';
	}

	function projectName(projectID: string | null): string {
		if (!projectID) return '—';
		return data.projectNames[projectID] ?? projectID;
	}
</script>

<div class="min-h-full space-y-4 bg-bg-canvas dark:bg-dark-bg-canvas">
	<div class="flex items-center justify-between">
		<h1 class="text-xl font-semibold text-fg-ink dark:text-dark-fg-ink">
			All Beads
			{#if data.scope === 'federation'}
				<span
					data-testid="scope-indicator"
					class="ml-2 inline-block border px-1.5 py-0.5 align-middle font-mono-code text-[10px] uppercase badge-status-in-progress"
				>federation</span>
			{/if}
		</h1>
		<div class="flex items-center gap-3">
			<button
				data-testid="scope-toggle"
				onclick={toggleScope}
				class={chipClass(data.scope === 'federation')}
			>
				{data.scope === 'federation' ? 'scope: federation' : 'scope: local'}
			</button>
			<span class="text-sm text-fg-muted dark:text-dark-fg-muted">
				{edges.length} of {totalCount}
			</span>
		</div>
	</div>

	{#if data.federationError}
		<div
			data-testid="federation-error"
			class="border border-accent-load/40 bg-accent-load/10 px-4 py-2 font-label-caps text-label-caps text-accent-load dark:border-dark-accent-load/40 dark:bg-dark-accent-load/10 dark:text-dark-accent-load"
		>
			Federated query failed: {data.federationError}
		</div>
	{/if}

	<!-- Status filter chips -->
	<div class="flex flex-wrap gap-2">
		<span class="self-center text-xs text-fg-muted dark:text-dark-fg-muted">Status:</span>
		{#each STATUS_OPTIONS as status}
			<button class={chipClass(data.activeStatus === status)} onclick={() => toggleStatus(status)}>
				{status}
			</button>
		{/each}
		{#if data.activeStatus}
			<button
				class="rounded-sm border border-border-line bg-bg-elevated px-3 py-1 text-xs text-fg-ink hover:bg-bg-surface dark:border-dark-border-line dark:bg-dark-bg-elevated dark:text-dark-fg-ink dark:hover:bg-dark-bg-surface"
				onclick={() => setFilter('status', null)}
			>
				clear
			</button>
		{/if}
	</div>

	<!-- Project filter chips -->
	{#if data.projects.length > 0}
		<div class="flex flex-wrap gap-2">
			<span class="self-center text-xs text-fg-muted dark:text-dark-fg-muted">Project:</span>
			{#each data.projects as project}
				<button
					class={chipClass(data.activeProject === project.id)}
					onclick={() => toggleProject(project.id)}
				>
					{project.name}
				</button>
			{/each}
			{#if data.activeProject}
				<button
					class="rounded-sm border border-border-line bg-bg-elevated px-3 py-1 text-xs text-fg-ink hover:bg-bg-surface dark:border-dark-border-line dark:bg-dark-bg-elevated dark:text-dark-fg-ink dark:hover:bg-dark-bg-surface"
					onclick={() => setFilter('project', null)}
				>
					clear
				</button>
			{/if}
		</div>
	{/if}

	<!-- Label filter chips (only shown when labels exist in current result) -->
	{#if allLabels.length > 0}
		<div class="flex flex-wrap gap-2">
			<span class="self-center text-xs text-fg-muted dark:text-dark-fg-muted">Label:</span>
			{#each allLabels as label}
				<button class={chipClass(data.activeLabel === label)} onclick={() => toggleLabel(label)}>
					{label}
				</button>
			{/each}
			{#if data.activeLabel}
				<button
					class="rounded-sm border border-border-line bg-bg-elevated px-3 py-1 text-xs text-fg-ink hover:bg-bg-surface dark:border-dark-border-line dark:bg-dark-bg-elevated dark:text-dark-fg-ink dark:hover:bg-dark-bg-surface"
					onclick={() => setFilter('label', null)}
				>
					clear
				</button>
			{/if}
		</div>
	{/if}

	<div class="overflow-hidden border border-border-line bg-bg-elevated dark:border-dark-border-line dark:bg-dark-bg-elevated">
		<table class="w-full border-collapse text-sm">
			<thead>
				<tr class="border-b border-border-line bg-bg-surface dark:border-dark-border-line dark:bg-dark-bg-surface">
					<th class="px-4 py-3 text-left font-medium text-fg-muted dark:text-dark-fg-muted">ID</th>
					<th class="px-4 py-3 text-left font-medium text-fg-muted dark:text-dark-fg-muted">Title</th>
					<th class="px-4 py-3 text-left font-medium text-fg-muted dark:text-dark-fg-muted">Project</th>
					<th class="px-4 py-3 text-left font-medium text-fg-muted dark:text-dark-fg-muted">Status</th>
					{#if data.scope === 'federation'}
						<th class="px-4 py-3 text-left font-medium text-fg-muted dark:text-dark-fg-muted">Node</th>
					{/if}
					<th class="px-4 py-3 text-right font-medium text-fg-muted dark:text-dark-fg-muted">Priority</th>
				</tr>
			</thead>
			<tbody>
				{#each edges as edge (edge.cursor)}
					<tr
						class="border-b border-border-line last:border-0 dark:border-dark-border-line"
					>
						<td class="px-4 py-3 font-mono-code text-xs text-accent-lever dark:text-dark-accent-lever">
							{edge.node.id}
						</td>
						<td class="px-4 py-3 text-fg-ink dark:text-dark-fg-ink">
							{edge.node.title}
						</td>
						<td class="px-4 py-3">
							<span class="inline-flex items-center border border-border-line px-2 py-0.5 text-xs font-medium text-fg-muted dark:border-dark-border-line dark:text-dark-fg-muted">
								{projectName(edge.node.projectID)}
							</span>
						</td>
						<td class="px-4 py-3">
							<span class="inline-block border px-1.5 py-0.5 text-[10px] font-semibold uppercase tracking-wide {statusBadgeClass(edge.node.status)}">
								{edge.node.status}
							</span>
						</td>
						{#if data.scope === 'federation'}
							{@const fed = data.federationByBeadId[edge.node.id]}
							<td class="px-4 py-3" data-testid="federation-row-cell">
								{#if fed}
									<div class="flex items-center gap-2">
										<span
											data-testid="row-node-badge"
											data-status={fed.status}
											class="inline-block border px-1.5 py-0.5 font-mono-code text-[10px] uppercase {federationBadgeClass(
												fed.status
											)}"
										>
											{fed.nodeId}
										</span>
										<a
											data-testid="row-spoke-link"
											href={fed.projectUrl}
											target="_blank"
											rel="noopener noreferrer"
											class="font-mono-code text-[10px] text-accent-lever hover:underline dark:text-dark-accent-lever"
										>
											spoke ↗
										</a>
									</div>
								{:else}
									<span class="text-fg-muted dark:text-dark-fg-muted">—</span>
								{/if}
							</td>
						{/if}
						<td class="px-4 py-3 text-right font-mono-code text-xs font-medium text-priority-p{edge.node.priority}">
							P{edge.node.priority}
						</td>
					</tr>
				{/each}
				{#if edges.length === 0}
					<tr>
						<td
							colspan={data.scope === 'federation' ? 6 : 5}
							class="px-4 py-8 text-center text-fg-muted dark:text-dark-fg-muted"
						>
							No beads found.
						</td>
					</tr>
				{/if}
			</tbody>
		</table>
	</div>

	{#if pageInfo.hasNextPage}
		<div class="flex justify-center">
			<button
				onclick={loadMore}
				disabled={loadingMore}
				class="rounded-sm border border-border-line bg-bg-elevated px-4 py-2 text-sm text-fg-ink hover:bg-bg-surface disabled:cursor-not-allowed disabled:opacity-50 dark:border-dark-border-line dark:bg-dark-bg-elevated dark:text-dark-fg-ink dark:hover:bg-dark-bg-surface"
			>
				{loadingMore ? 'Loading…' : 'Load more'}
			</button>
		</div>
	{/if}
</div>
