<script lang="ts">
	import type { PageData } from './$types';
	import { createClient } from '$lib/gql/client';
	import { gql } from 'graphql-request';

	const BEADS_QUERY = gql`
		query BeadsByProject($projectID: String!, $first: Int, $after: String) {
			beadsByProject(projectID: $projectID, first: $first, after: $after) {
				edges {
					node {
						id
						title
						status
						priority
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
		beadsByProject: {
			edges: BeadEdge[];
			pageInfo: PageInfo;
			totalCount: number;
		};
	}

	let { data }: { data: PageData } = $props();

	let edges = $state<BeadEdge[]>(data.beads.edges);
	let pageInfo = $state<PageInfo>(data.beads.pageInfo);
	let totalCount = $state<number>(data.beads.totalCount);
	let loadingMore = $state(false);

	async function loadMore() {
		if (!pageInfo.hasNextPage || loadingMore) return;
		loadingMore = true;
		try {
			const client = createClient();
			const result = await client.request<BeadsResult>(BEADS_QUERY, {
				projectID: data.projectId,
				first: 10,
				after: pageInfo.endCursor
			});
			edges = [...edges, ...result.beadsByProject.edges];
			pageInfo = result.beadsByProject.pageInfo;
			totalCount = result.beadsByProject.totalCount;
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
</script>

<div class="space-y-4">
	<div class="flex items-center justify-between">
		<h1 class="text-xl font-semibold dark:text-white">Beads</h1>
		<span class="text-sm text-gray-500 dark:text-gray-400">
			{edges.length} of {totalCount}
		</span>
	</div>

	<div class="overflow-hidden rounded-lg border border-gray-200 dark:border-gray-700">
		<table class="w-full text-sm">
			<thead>
				<tr class="border-b border-gray-200 bg-gray-50 dark:border-gray-700 dark:bg-gray-800">
					<th class="px-4 py-3 text-left font-medium text-gray-600 dark:text-gray-300">ID</th>
					<th class="px-4 py-3 text-left font-medium text-gray-600 dark:text-gray-300">Title</th>
					<th class="px-4 py-3 text-left font-medium text-gray-600 dark:text-gray-300">Status</th>
					<th class="px-4 py-3 text-right font-medium text-gray-600 dark:text-gray-300">Priority</th>
				</tr>
			</thead>
			<tbody>
				{#each edges as edge (edge.cursor)}
					<tr
						class="border-b border-gray-100 last:border-0 hover:bg-gray-50 dark:border-gray-700 dark:hover:bg-gray-800"
					>
						<td class="px-4 py-3 font-mono text-xs text-gray-500 dark:text-gray-400">
							{edge.node.id}
						</td>
						<td class="px-4 py-3 text-gray-900 dark:text-gray-100">
							{edge.node.title}
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
						<td colspan="4" class="px-4 py-8 text-center text-gray-400 dark:text-gray-600">
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
