<script lang="ts">
	import type { PageData } from './$types';
	import { goto } from '$app/navigation';
	import { page } from '$app/stores';
	import { createClient } from '$lib/gql/client';
	import { gql } from 'graphql-request';

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
	let filterKey = $derived(`${data.activeStatus}::${data.activeLabel}::${data.activeProject}`);
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

	function setFilter(key: 'status' | 'label' | 'project', value: string | null) {
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

	async function loadMore() {
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

	function statusClass(status: string): string {
		const known = ['open', 'in-progress', 'closed', 'blocked', 'running', 'completed', 'failed'];
		return known.includes(status) ? `text-status-${status}` : 'text-fg-muted dark:text-dark-fg-muted';
	}

	function chipClass(active: boolean): string {
		return active
			? 'rounded-sm border px-3 py-1 text-xs font-medium border-accent-lever bg-accent-lever/10 text-accent-lever dark:border-dark-accent-lever dark:bg-dark-accent-lever/20 dark:text-dark-accent-lever'
			: 'rounded-sm border px-3 py-1 text-xs font-medium border-border-line text-fg-muted hover:border-fg-muted hover:bg-bg-surface dark:border-dark-border-line dark:text-dark-fg-muted dark:hover:bg-dark-bg-surface';
	}

	function projectName(projectID: string | null): string {
		if (!projectID) return '—';
		return data.projectNames[projectID] ?? projectID;
	}
</script>

<div class="space-y-4">
	<div class="flex items-center justify-between">
		<h1 class="text-xl font-semibold text-fg-ink dark:text-dark-fg-ink">All Beads</h1>
		<span class="text-sm text-fg-muted dark:text-dark-fg-muted">
			{edges.length} of {totalCount}
		</span>
	</div>

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
				class="rounded-sm border border-border-line px-3 py-1 text-xs text-fg-muted hover:text-fg-ink dark:border-dark-border-line dark:text-dark-fg-muted dark:hover:text-dark-fg-ink"
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
					class="rounded-sm border border-border-line px-3 py-1 text-xs text-fg-muted hover:text-fg-ink dark:border-dark-border-line dark:text-dark-fg-muted dark:hover:text-dark-fg-ink"
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
					class="rounded-sm border border-border-line px-3 py-1 text-xs text-fg-muted hover:text-fg-ink dark:border-dark-border-line dark:text-dark-fg-muted dark:hover:text-dark-fg-ink"
					onclick={() => setFilter('label', null)}
				>
					clear
				</button>
			{/if}
		</div>
	{/if}

	<div class="overflow-hidden border border-border-line dark:border-dark-border-line">
		<table class="w-full text-sm">
			<thead>
				<tr class="border-b border-border-line bg-bg-surface dark:border-dark-border-line dark:bg-dark-bg-surface">
					<th class="px-4 py-3 text-left font-medium text-fg-muted dark:text-dark-fg-muted">ID</th>
					<th class="px-4 py-3 text-left font-medium text-fg-muted dark:text-dark-fg-muted">Title</th>
					<th class="px-4 py-3 text-left font-medium text-fg-muted dark:text-dark-fg-muted">Project</th>
					<th class="px-4 py-3 text-left font-medium text-fg-muted dark:text-dark-fg-muted">Status</th>
					<th class="px-4 py-3 text-right font-medium text-fg-muted dark:text-dark-fg-muted">Priority</th>
				</tr>
			</thead>
			<tbody>
				{#each edges as edge (edge.cursor)}
					<tr
						class="border-b border-border-line last:border-0 dark:border-dark-border-line"
					>
						<td class="px-4 py-3 font-mono-code text-xs text-lever">
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
							<span class="inline-block border px-1.5 py-0.5 text-[10px] font-semibold uppercase tracking-wide {`badge-status-${edge.node.status}`}">
								{edge.node.status}
							</span>
						</td>
						<td class="px-4 py-3 text-right font-mono-code text-xs font-medium text-priority-p{edge.node.priority}">
							P{edge.node.priority}
						</td>
					</tr>
				{/each}
				{#if edges.length === 0}
					<tr>
						<td colspan="5" class="px-4 py-8 text-center text-fg-muted dark:text-dark-fg-muted">
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
				class="rounded-sm border border-border-line px-4 py-2 text-sm text-fg-muted hover:bg-bg-surface disabled:cursor-not-allowed disabled:opacity-50 dark:border-dark-border-line dark:text-dark-fg-muted dark:hover:bg-dark-bg-surface"
			>
				{loadingMore ? 'Loading…' : 'Load more'}
			</button>
		</div>
	{/if}
</div>
