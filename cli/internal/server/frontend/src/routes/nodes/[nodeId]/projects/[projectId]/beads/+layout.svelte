<script lang="ts">
	import type { LayoutData } from './$types';
	import type { Snippet } from 'svelte';
	import { goto, invalidateAll } from '$app/navigation';
	import { page } from '$app/stores';
	import { createClient } from '$lib/gql/client';
	import { gql } from 'graphql-request';
	import BeadForm from '$lib/components/BeadForm.svelte';
	import { subscribeBeadLifecycle } from '$lib/gql/subscriptions';

	const BEADS_QUERY = gql`
		query BeadsByProject(
			$projectID: String!
			$first: Int
			$after: String
			$status: String
			$label: String
			$search: String
		) {
			beadsByProject(
				projectID: $projectID
				first: $first
				after: $after
				status: $status
				label: $label
				search: $search
			) {
				edges {
					node {
						id
						title
						status
						priority
						owner
						updatedAt
						labels
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
		owner: string | null;
		updatedAt: string;
		labels: string[] | null;
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

	const STATUS_OPTIONS = ['open', 'ready', 'in-progress', 'closed', 'blocked'];
	const PRIORITY_OPTIONS = [0, 1, 2, 3, 4];

	let { data, children }: { data: LayoutData; children: Snippet } = $props();

	// Extra edges accumulated via "load more" (reset when filter key changes)
	let appendedEdges = $state<BeadEdge[]>([]);
	let appendedPageInfo = $state<PageInfo | null>(null);
	let loadingMore = $state(false);
	let showCreateForm = $state(false);
	let prioritySortAsc = $derived(data.activeSort !== 'priority-desc');

	// Live status overrides from beadLifecycle subscription (beadID -> status)
	let liveStatusOverrides = $state<Map<string, string>>(new Map());

	// Subscribe to bead lifecycle events while the page is open
	$effect(() => {
		const pid = data.projectId;
		liveStatusOverrides = new Map();
		const dispose = subscribeBeadLifecycle(pid, (evt) => {
			if (evt.kind === 'status_changed' && evt.summary) {
				const match = evt.summary.match(/status changed from \S+ to (\S+)/);
				if (match) {
					const next = new Map(liveStatusOverrides);
					next.set(evt.beadID, match[1]);
					liveStatusOverrides = next;
				}
			}
		});
		return dispose;
	});

	// Local search input state (drives URL via debounce)
	let searchInput = $state(data.activeSearch ?? '');

	// Debounce: update URL ?q= 200ms after user stops typing
	let debounceTimer: ReturnType<typeof setTimeout> | null = null;
	$effect(() => {
		const val = searchInput;
		if (debounceTimer) clearTimeout(debounceTimer);
		debounceTimer = setTimeout(() => {
			// Skip if the URL already reflects the current input value
			const currentQ = $page.url.searchParams.get('q') ?? '';
			if (val === currentQ) return;
			const params = new URLSearchParams($page.url.searchParams);
			if (val) {
				params.set('q', val);
			} else {
				params.delete('q');
			}
			params.delete('after');
			const search = params.toString();
			// Preserve beadId in path if panel is open
			const pathname = $page.url.pathname;
			goto(search ? `${pathname}?${search}` : pathname, { replaceState: true });
		}, 200);
	});

	// Sync searchInput when URL changes (e.g. back/forward navigation)
	$effect(() => {
		searchInput = data.activeSearch ?? '';
	});

	// Track the active filter combo so we can reset appended pages on change
	let filterKey = $derived(
		`${data.activeStatus}::${data.activePriority}::${data.activeLabel}::${data.activeSearch}`
	);
	let prevFilterKey = $state('');
	$effect(() => {
		if (filterKey !== prevFilterKey) {
			prevFilterKey = filterKey;
			appendedEdges = [];
			appendedPageInfo = null;
		}
	});

	let edges = $derived([...data.beads.edges, ...appendedEdges]);
	let filteredEdges = $derived(
		edges.filter((edge) => {
			const activeStatus = liveStatusOverrides.get(edge.node.id) ?? edge.node.status;
			const labels = edge.node.labels ?? [];
			const search = data.activeSearch?.toLowerCase();
			return (
				(!data.activeStatus || activeStatus === data.activeStatus) &&
				(!data.activePriority || String(edge.node.priority) === data.activePriority) &&
				(!data.activeLabel || labels.includes(data.activeLabel)) &&
				(!search ||
					edge.node.title.toLowerCase().includes(search) ||
					edge.node.id.toLowerCase().includes(search) ||
					labels.some((label) => label.toLowerCase().includes(search)))
			);
		})
	);
	let sortedEdges = $derived(
		[...filteredEdges].sort((a, b) =>
			prioritySortAsc ? a.node.priority - b.node.priority : b.node.priority - a.node.priority
		)
	);
	let pageInfo = $derived<PageInfo>(appendedPageInfo ?? data.beads.pageInfo);
	let totalCount = $derived(data.beads.totalCount);

	// Derive all unique labels from current result set
	let allLabels = $derived(Array.from(new Set(edges.flatMap((e) => e.node.labels ?? []))).sort());
	let hasActiveFilters = $derived(
		Boolean(data.activeStatus || data.activePriority || data.activeLabel || data.activeSearch)
	);

	// The currently open bead (from child route params)
	let activeBead = $derived(($page.params as Record<string, string>)['beadId'] ?? null);

	function setFilter(key: 'status' | 'priority' | 'labels', value: string | null) {
		const params = new URLSearchParams($page.url.searchParams);
		if (value === null) {
			params.delete(key);
		} else {
			params.set(key, value);
		}
		if (key === 'labels') {
			params.delete('label');
		}
		// Changing filters resets pagination
		params.delete('after');
		const search = params.toString();
		// Stay on same path (either /beads or /beads/[beadId])
		goto(search ? `?${search}` : $page.url.pathname, { replaceState: true });
	}

	function toggleStatus(status: string) {
		setFilter('status', data.activeStatus === status ? null : status);
	}

	function togglePriority(priority: number) {
		const priorityValue = String(priority);
		setFilter('priority', data.activePriority === priorityValue ? null : priorityValue);
	}

	function togglePrioritySort() {
		const params = new URLSearchParams($page.url.searchParams);
		const nextSort = prioritySortAsc ? 'priority-desc' : 'priority-asc';
		if (nextSort === 'priority-asc') {
			params.delete('sort');
		} else {
			params.set('sort', nextSort);
		}
		const search = params.toString();
		goto(search ? `?${search}` : $page.url.pathname, { replaceState: true });
	}

	function toggleLabel(label: string) {
		setFilter('labels', data.activeLabel === label ? null : label);
	}

	function clearFilters() {
		const params = new URLSearchParams($page.url.searchParams);
		params.delete('status');
		params.delete('priority');
		params.delete('label');
		params.delete('labels');
		params.delete('q');
		params.delete('after');
		searchInput = '';
		const search = params.toString();
		goto(search ? `?${search}` : $page.url.pathname, { replaceState: true });
	}

	async function loadMore() {
		if (!pageInfo.hasNextPage || loadingMore) return;
		loadingMore = true;
		try {
			const client = createClient();
			const result = await client.request<BeadsResult>(BEADS_QUERY, {
				projectID: data.projectId,
				first: 10,
				after: pageInfo.endCursor,
				status: data.activeStatus ?? undefined,
				label: data.activeLabel ?? undefined,
				search: data.activeSearch ?? undefined
			});
			appendedEdges = [...appendedEdges, ...result.beadsByProject.edges];
			appendedPageInfo = result.beadsByProject.pageInfo;
		} finally {
			loadingMore = false;
		}
	}

	function openBead(beadId: string) {
		const p = $page.params as Record<string, string>;
		const searchStr = $page.url.searchParams.toString();
		const beadPath = `/nodes/${p['nodeId']}/projects/${p['projectId']}/beads/${beadId}`;
		goto(searchStr ? `${beadPath}?${searchStr}` : beadPath);
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

	function priorityLabel(priority: number): string {
		return `P${priority}`;
	}

	function priorityClass(priority: number): string {
		return `text-priority-p${priority}`;
	}
</script>

<div class="min-h-full space-y-4 bg-bg-canvas dark:bg-dark-bg-canvas">
	<div class="flex items-center justify-between">
		<h1 class="text-xl font-semibold text-fg-ink dark:text-dark-fg-ink">Beads</h1>
		<div class="flex items-center gap-3">
			<span class="text-sm text-fg-muted dark:text-dark-fg-muted">
				{sortedEdges.length} of {totalCount}
			</span>
			<button
				onclick={() => (showCreateForm = true)}
				class="rounded-sm bg-accent-lever px-3 py-1.5 text-sm font-medium text-white hover:bg-accent-lever/90"
			>
				New bead
			</button>
		</div>
	</div>

	<!-- Search input -->
	<div class="relative">
		<input
			type="search"
			bind:value={searchInput}
			placeholder="Search beads…"
			class="w-full rounded-none border border-border-line bg-bg-elevated px-3 py-2 text-sm text-fg-ink placeholder-fg-muted focus:border-accent-lever focus:ring-1 focus:ring-accent-lever focus:outline-none dark:border-dark-border-line dark:bg-dark-bg-elevated dark:text-dark-fg-ink dark:placeholder-dark-fg-muted dark:focus:border-dark-accent-lever"
		/>
	</div>

	<!-- Status filter chips -->
	<div class="flex flex-wrap gap-2">
		<span class="self-center text-xs text-fg-muted dark:text-dark-fg-muted">Status:</span>
		{#each STATUS_OPTIONS as status}
			<button
				type="button"
				aria-pressed={data.activeStatus === status}
				class={chipClass(data.activeStatus === status)}
				onclick={() => toggleStatus(status)}
			>
				{status}
			</button>
		{/each}
		{#if data.activeStatus}
			<button
				type="button"
				class="rounded-sm border border-border-line bg-bg-elevated px-3 py-1 text-xs text-fg-ink hover:bg-bg-surface dark:border-dark-border-line dark:bg-dark-bg-elevated dark:text-dark-fg-ink dark:hover:bg-dark-bg-surface"
				onclick={() => setFilter('status', null)}
			>
				clear
			</button>
		{/if}
	</div>

	<!-- Priority filter chips -->
	<div class="flex flex-wrap gap-2">
		<span class="self-center text-xs text-fg-muted dark:text-dark-fg-muted">Priority:</span>
		{#each PRIORITY_OPTIONS as priority}
			<button
				type="button"
				aria-pressed={data.activePriority === String(priority)}
				class={chipClass(data.activePriority === String(priority))}
				onclick={() => togglePriority(priority)}
			>
				{priorityLabel(priority)}
			</button>
		{/each}
		{#if data.activePriority}
			<button
				type="button"
				class="rounded-sm border border-border-line bg-bg-elevated px-3 py-1 text-xs text-fg-ink hover:bg-bg-surface dark:border-dark-border-line dark:bg-dark-bg-elevated dark:text-dark-fg-ink dark:hover:bg-dark-bg-surface"
				onclick={() => setFilter('priority', null)}
			>
				clear
			</button>
		{/if}
	</div>

	<!-- Label filter chips (only shown when labels exist in current result) -->
	{#if allLabels.length > 0}
		<div class="flex flex-wrap gap-2">
			<span class="self-center text-xs text-fg-muted dark:text-dark-fg-muted">Labels:</span>
			{#each allLabels as label}
				<button
					type="button"
					aria-pressed={data.activeLabel === label}
					class={chipClass(data.activeLabel === label)}
					onclick={() => toggleLabel(label)}
				>
					{label}
				</button>
			{/each}
			{#if data.activeLabel}
				<button
					type="button"
					class="rounded-sm border border-border-line bg-bg-elevated px-3 py-1 text-xs text-fg-ink hover:bg-bg-surface dark:border-dark-border-line dark:bg-dark-bg-elevated dark:text-dark-fg-ink dark:hover:bg-dark-bg-surface"
					onclick={() => setFilter('labels', null)}
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
					<th class="px-4 py-3 text-left font-medium text-fg-muted dark:text-dark-fg-muted">Status</th>
					<th class="px-4 py-3 text-right font-medium text-fg-muted dark:text-dark-fg-muted">
						<button
							type="button"
							aria-label="Sort by priority"
							class="ml-auto inline-flex items-center gap-1 hover:text-fg-ink dark:hover:text-dark-fg-ink"
							onclick={togglePrioritySort}
						>
							Priority
							<span aria-hidden="true">{prioritySortAsc ? '↑' : '↓'}</span>
						</button>
					</th>
					<th class="px-4 py-3 text-left font-medium text-fg-muted dark:text-dark-fg-muted">Owner</th>
					<th class="px-4 py-3 text-left font-medium text-fg-muted dark:text-dark-fg-muted">Updated</th>
				</tr>
			</thead>
			<tbody>
				{#each sortedEdges as edge (edge.node.id)}
					<tr
						data-testid="bead-row"
						data-priority={edge.node.priority}
						onclick={() => openBead(edge.node.id)}
						class="cursor-pointer border-b border-border-line last:border-0 hover:bg-bg-surface dark:border-dark-border-line dark:hover:bg-dark-bg-surface {activeBead ===
						edge.node.id
							? 'bg-accent-lever/10 dark:bg-dark-accent-lever/10'
							: ''}"
					>
						<td class="px-4 py-3 font-mono text-xs text-accent-lever dark:text-dark-accent-lever">
							{edge.node.id}
						</td>
						<td class="px-4 py-3 text-fg-ink dark:text-dark-fg-ink">
							{edge.node.title}
							{#if edge.node.labels?.length}
								<div class="mt-2 flex flex-wrap gap-1">
									{#each edge.node.labels as label}
										<button
											type="button"
											data-testid="label-chip"
											aria-pressed={data.activeLabel === label}
											class={chipClass(data.activeLabel === label)}
											onclick={(event) => {
												event.stopPropagation();
												toggleLabel(label);
											}}
										>
											{label}
										</button>
									{/each}
								</div>
							{/if}
						</td>
						<td class="px-4 py-3">
							<span
								class="inline-block border px-1.5 py-0.5 text-[10px] font-semibold uppercase tracking-wide {statusBadgeClass(
									liveStatusOverrides.get(edge.node.id) ?? edge.node.status
								)}"
							>
								{liveStatusOverrides.get(edge.node.id) ?? edge.node.status}
							</span>
						</td>
						<td class="px-4 py-3 text-right font-mono text-xs font-medium {priorityClass(edge.node.priority)}">
							{priorityLabel(edge.node.priority)}
						</td>
						<td class="px-4 py-3 text-fg-muted dark:text-dark-fg-muted">
							{edge.node.owner ?? '—'}
						</td>
						<td class="px-4 py-3 text-xs text-fg-muted dark:text-dark-fg-muted">
							{new Date(edge.node.updatedAt).toLocaleDateString()}
						</td>
					</tr>
				{/each}
				{#if sortedEdges.length === 0}
					<tr>
						<td colspan="6" class="px-4 py-8 text-center text-fg-muted dark:text-dark-fg-muted">
							{#if hasActiveFilters}
								<div class="space-y-3">
									<p>No beads match the current filters.</p>
									<button
										type="button"
										class="rounded-sm border border-border-line bg-bg-elevated px-3 py-1.5 text-sm text-fg-ink hover:bg-bg-surface dark:border-dark-border-line dark:bg-dark-bg-elevated dark:text-dark-fg-ink dark:hover:bg-dark-bg-surface"
										onclick={clearFilters}
									>
										Clear filters
									</button>
								</div>
							{:else}
								No beads found.
							{/if}
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

{@render children()}

{#if showCreateForm}
	<!-- Backdrop -->
	<div
		class="fixed inset-0 z-40 bg-black/20 dark:bg-black/40"
		onclick={() => (showCreateForm = false)}
		role="button"
		tabindex="-1"
		aria-label="Close"
		onkeydown={(e) => e.key === 'Escape' && (showCreateForm = false)}
	></div>

	<!-- Create form panel -->
	<div
		class="fixed top-0 right-0 z-50 flex h-full w-full max-w-xl flex-col bg-bg-elevated shadow-xl dark:bg-dark-bg-elevated"
		style="max-width: 36rem;"
	>
		<div
			class="flex shrink-0 items-center justify-between border-b border-border-line px-6 py-4 dark:border-dark-border-line"
		>
			<h2 class="text-base font-semibold text-fg-ink dark:text-dark-fg-ink">New bead</h2>
		</div>
		<div class="flex-1 overflow-auto p-6">
			<BeadForm
				onSuccess={async (newBead) => {
					showCreateForm = false;
					await invalidateAll();
					const p = $page.params as Record<string, string>;
					const searchStr = $page.url.searchParams.toString();
					const beadPath = `/nodes/${p['nodeId']}/projects/${p['projectId']}/beads/${newBead.id}`;
					goto(searchStr ? `${beadPath}?${searchStr}` : beadPath);
				}}
				onCancel={() => (showCreateForm = false)}
			/>
		</div>
	</div>
{/if}
