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
		switch (status) {
			case 'open':
				return 'text-blue-600 dark:text-blue-400';
			case 'in-progress':
				return 'text-yellow-600 dark:text-yellow-400';
			case 'closed':
				return 'text-green-600 dark:text-green-400';
			case 'blocked':
				return 'text-red-600 dark:text-red-400';
			default:
				return 'text-gray-500 dark:text-gray-400';
		}
	}

	function chipClass(active: boolean): string {
		return active
			? 'rounded-full border px-3 py-1 text-xs font-medium border-blue-500 bg-blue-50 text-blue-700 dark:border-blue-400 dark:bg-blue-900/30 dark:text-blue-300'
			: 'rounded-full border px-3 py-1 text-xs font-medium border-gray-300 text-gray-600 hover:border-gray-400 hover:bg-gray-50 dark:border-gray-600 dark:text-gray-400 dark:hover:bg-gray-800';
	}

	function projectName(projectID: string | null): string {
		if (!projectID) return '—';
		return data.projectNames[projectID] ?? projectID;
	}
</script>

<div class="space-y-4">
	<div class="flex items-center justify-between">
		<h1 class="text-xl font-semibold dark:text-white">All Beads</h1>
		<span class="text-sm text-gray-500 dark:text-gray-400">
			{edges.length} of {totalCount}
		</span>
	</div>

	<!-- Status filter chips -->
	<div class="flex flex-wrap gap-2">
		<span class="self-center text-xs text-gray-500 dark:text-gray-400">Status:</span>
		{#each STATUS_OPTIONS as status}
			<button class={chipClass(data.activeStatus === status)} onclick={() => toggleStatus(status)}>
				{status}
			</button>
		{/each}
		{#if data.activeStatus}
			<button
				class="rounded-full border border-gray-300 px-3 py-1 text-xs text-gray-400 hover:text-gray-600 dark:border-gray-600 dark:text-gray-500"
				onclick={() => setFilter('status', null)}
			>
				clear
			</button>
		{/if}
	</div>

	<!-- Project filter chips -->
	{#if data.projects.length > 0}
		<div class="flex flex-wrap gap-2">
			<span class="self-center text-xs text-gray-500 dark:text-gray-400">Project:</span>
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
					class="rounded-full border border-gray-300 px-3 py-1 text-xs text-gray-400 hover:text-gray-600 dark:border-gray-600 dark:text-gray-500"
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
			<span class="self-center text-xs text-gray-500 dark:text-gray-400">Label:</span>
			{#each allLabels as label}
				<button class={chipClass(data.activeLabel === label)} onclick={() => toggleLabel(label)}>
					{label}
				</button>
			{/each}
			{#if data.activeLabel}
				<button
					class="rounded-full border border-gray-300 px-3 py-1 text-xs text-gray-400 hover:text-gray-600 dark:border-gray-600 dark:text-gray-500"
					onclick={() => setFilter('label', null)}
				>
					clear
				</button>
			{/if}
		</div>
	{/if}

	<div class="overflow-hidden rounded-lg border border-gray-200 dark:border-gray-700">
		<table class="w-full text-sm">
			<thead>
				<tr class="border-b border-gray-200 bg-gray-50 dark:border-gray-700 dark:bg-gray-800">
					<th class="px-4 py-3 text-left font-medium text-gray-600 dark:text-gray-300">ID</th>
					<th class="px-4 py-3 text-left font-medium text-gray-600 dark:text-gray-300">Title</th>
					<th class="px-4 py-3 text-left font-medium text-gray-600 dark:text-gray-300">Project</th>
					<th class="px-4 py-3 text-left font-medium text-gray-600 dark:text-gray-300">Status</th>
					<th class="px-4 py-3 text-right font-medium text-gray-600 dark:text-gray-300">Priority</th>
				</tr>
			</thead>
			<tbody>
				{#each edges as edge (edge.cursor)}
					<tr
						class="border-b border-gray-100 last:border-0 dark:border-gray-700"
					>
						<td class="px-4 py-3 font-mono text-xs text-gray-500 dark:text-gray-400">
							{edge.node.id}
						</td>
						<td class="px-4 py-3 text-gray-900 dark:text-gray-100">
							{edge.node.title}
						</td>
						<td class="px-4 py-3">
							<span class="inline-flex items-center rounded-full bg-gray-100 px-2 py-0.5 text-xs font-medium text-gray-700 dark:bg-gray-700 dark:text-gray-300">
								{projectName(edge.node.projectID)}
							</span>
						</td>
						<td class="px-4 py-3">
							<span class="font-medium {statusClass(edge.node.status)}">
								{edge.node.status}
							</span>
						</td>
						<td class="px-4 py-3 text-right text-gray-600 dark:text-gray-300">
							{edge.node.priority}
						</td>
					</tr>
				{/each}
				{#if edges.length === 0}
					<tr>
						<td colspan="5" class="px-4 py-8 text-center text-gray-400 dark:text-gray-600">
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
				class="rounded-md border border-gray-300 px-4 py-2 text-sm text-gray-700 hover:bg-gray-50 disabled:cursor-not-allowed disabled:opacity-50 dark:border-gray-600 dark:text-gray-300 dark:hover:bg-gray-800"
			>
				{loadingMore ? 'Loading…' : 'Load more'}
			</button>
		</div>
	{/if}
</div>
