<script lang="ts">
	import type { PageData } from './$types';
	import { page } from '$app/stores';
	import { goto } from '$app/navigation';
	import { onMount } from 'svelte';
	import { createClient } from '$lib/gql/client';
	import { COMMIT_EXECUTION_QUERY } from './+page';

	let { data }: { data: PageData } = $props();

	let executionBySha = $state<Record<string, string | null>>({});

	onMount(async () => {
		const client = createClient(fetch);
		const params = $page.params as Record<string, string>;
		const projectID = params['projectId'];
		await Promise.all(
			data.commits.edges.map(async (edge) => {
				const sha = edge.node.sha;
				if (executionBySha[sha] !== undefined) return;
				try {
					const result = await client.request<{ executionByResultRev: { id: string } | null }>(
						COMMIT_EXECUTION_QUERY,
						{ projectID, sha }
					);
					executionBySha = { ...executionBySha, [sha]: result.executionByResultRev?.id ?? null };
				} catch {
					executionBySha = { ...executionBySha, [sha]: null };
				}
			})
		);
	});

	function executionHref(executionId: string): string {
		const params = $page.params as Record<string, string>;
		const runId = executionId.startsWith('exec-') ? executionId : `exec-${executionId}`;
		return `/nodes/${params['nodeId']}/projects/${params['projectId']}/runs/${runId}`;
	}

	function fmtDate(iso: string): string {
		return new Date(iso).toLocaleString();
	}

	function goNext() {
		const cursor = data.commits.pageInfo.endCursor;
		if (cursor) {
			const p = $page.params as Record<string, string>;
			goto(
				`/nodes/${p['nodeId']}/projects/${p['projectId']}/commits?after=${encodeURIComponent(cursor)}`
			);
		}
	}

	function goPrev() {
		const p = $page.params as Record<string, string>;
		goto(`/nodes/${p['nodeId']}/projects/${p['projectId']}/commits`);
	}

	function openBead(beadId: string) {
		const p = $page.params as Record<string, string>;
		goto(`/nodes/${p['nodeId']}/projects/${p['projectId']}/beads/${beadId}`);
	}
</script>

<div class="space-y-4">
	<div class="flex items-center justify-between">
		<h1 class="text-headline-md font-headline-md text-fg-ink dark:text-dark-fg-ink">Commits</h1>
		<span class="text-body-sm text-fg-muted dark:text-dark-fg-muted">
			{data.commits.totalCount} total
		</span>
	</div>

	<div class="overflow-hidden border border-border-line dark:border-dark-border-line">
		<table class="w-full text-sm">
			<thead>
				<tr class="border-b border-border-line bg-bg-surface dark:border-dark-border-line dark:bg-dark-bg-surface">
					<th class="px-4 py-3 text-left text-label-caps font-label-caps uppercase tracking-wide text-fg-muted dark:text-dark-fg-muted">SHA</th>
					<th class="px-4 py-3 text-left text-label-caps font-label-caps uppercase tracking-wide text-fg-muted dark:text-dark-fg-muted">Subject</th>
					<th class="px-4 py-3 text-left text-label-caps font-label-caps uppercase tracking-wide text-fg-muted dark:text-dark-fg-muted">Author</th>
					<th class="px-4 py-3 text-left text-label-caps font-label-caps uppercase tracking-wide text-fg-muted dark:text-dark-fg-muted">Date</th>
					<th class="px-4 py-3 text-left text-label-caps font-label-caps uppercase tracking-wide text-fg-muted dark:text-dark-fg-muted">Beads</th>
					<th class="px-4 py-3 text-left text-label-caps font-label-caps uppercase tracking-wide text-fg-muted dark:text-dark-fg-muted">Execution</th>
				</tr>
			</thead>
			<tbody>
				{#each data.commits.edges as edge (edge.cursor)}
					{@const c = edge.node}
					<tr class="border-b border-border-line last:border-0 hover:bg-bg-surface dark:border-dark-border-line dark:hover:bg-dark-bg-surface">
						<td class="px-4 py-3 font-mono-code text-mono-code text-accent-lever dark:text-dark-accent-lever">
							{c.shortSha}
						</td>
						<td class="px-4 py-3 text-body-sm text-fg-ink dark:text-dark-fg-ink">
							<span title={c.body ?? undefined}>{c.subject}</span>
						</td>
						<td class="px-4 py-3 text-body-sm text-fg-muted dark:text-dark-fg-muted">
							{c.author}
						</td>
						<td class="px-4 py-3 text-body-sm text-fg-muted dark:text-dark-fg-muted">
							{fmtDate(c.date)}
						</td>
						<td class="px-4 py-3">
							{#if c.beadRefs && c.beadRefs.length > 0}
								<div class="flex flex-wrap gap-1">
									{#each c.beadRefs as beadId}
										<button
											onclick={(e) => {
												e.stopPropagation();
												openBead(beadId);
											}}
											class="badge-status-open inline-flex border px-1.5 py-0.5 font-mono-code text-mono-code uppercase hover:opacity-80"
										>
											{beadId.slice(0, 8)}
										</button>
									{/each}
								</div>
							{:else}
								<span class="text-fg-muted dark:text-dark-fg-muted">—</span>
							{/if}
						</td>
						<td class="px-4 py-3">
							{#if executionBySha[c.sha]}
								<a
									href={executionHref(executionBySha[c.sha] as string)}
									class="font-mono-code text-mono-code text-accent-lever hover:underline dark:text-dark-accent-lever"
								>
									{(executionBySha[c.sha] as string).slice(0, 18)}
								</a>
							{:else}
								<span class="text-fg-muted dark:text-dark-fg-muted">—</span>
							{/if}
						</td>
					</tr>
				{/each}
				{#if data.commits.edges.length === 0}
					<tr>
						<td colspan="6" class="px-4 py-8 text-center text-body-sm text-fg-muted dark:text-dark-fg-muted">
							No commits found.
						</td>
					</tr>
				{/if}
			</tbody>
		</table>
	</div>

	<!-- Pagination -->
	<div class="flex items-center justify-between">
		<button
			onclick={goPrev}
			disabled={!data.after}
			class="border border-border-line px-3 py-1.5 text-body-sm text-fg-muted hover:bg-bg-surface disabled:cursor-not-allowed disabled:opacity-40 dark:border-dark-border-line dark:text-dark-fg-muted dark:hover:bg-dark-bg-surface"
		>
			← Previous
		</button>
		<span class="text-label-caps font-label-caps uppercase tracking-wide text-fg-muted dark:text-dark-fg-muted">
			{data.commits.edges.length} commits shown
		</span>
		<button
			onclick={goNext}
			disabled={!data.commits.pageInfo.hasNextPage}
			class="border border-border-line px-3 py-1.5 text-body-sm text-fg-muted hover:bg-bg-surface disabled:cursor-not-allowed disabled:opacity-40 dark:border-dark-border-line dark:text-dark-fg-muted dark:hover:bg-dark-bg-surface"
		>
			Next →
		</button>
	</div>
</div>
