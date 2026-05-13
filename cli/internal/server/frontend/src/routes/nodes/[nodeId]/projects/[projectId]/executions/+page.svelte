<script lang="ts">
	import type { PageData } from './$types';
	import { goto } from '$app/navigation';

	let { data }: { data: PageData } = $props();
	let selectedVerdict = $state<string>(data.activeVerdict ?? '');

	function execHref(id: string): string {
		return `/nodes/${data.nodeId}/projects/${data.projectId}/executions/${id}`;
	}

	function applyFilter() {
		const params = new URLSearchParams();
		if (selectedVerdict.trim()) {
			params.set('verdict', selectedVerdict.trim());
		}
		const search = params.toString();
		goto(search ? `?${search}` : '.', { replaceState: true });
	}
</script>

<svelte:head>
	<title>Executions | DDx</title>
</svelte:head>

<div class="space-y-4">
	<div class="flex items-end justify-between gap-4">
		<div>
			<h1 class="text-headline-md font-headline-md text-fg-ink dark:text-dark-fg-ink">Executions</h1>
			<p class="text-body-sm text-fg-muted dark:text-dark-fg-muted">{data.executions.totalCount} total</p>
		</div>
		<div class="flex items-end gap-2">
			<label class="flex flex-col gap-1 text-sm">
				<span>Verdict</span>
				<select bind:value={selectedVerdict} class="border border-border-line px-2 py-1">
					<option value="">All</option>
					<option value="PASS">PASS</option>
					<option value="BLOCK">BLOCK</option>
					<option value="FAIL">FAIL</option>
				</select>
			</label>
			<button class="border border-border-line px-3 py-2" onclick={applyFilter}>Apply</button>
		</div>
	</div>

	<table class="w-full border-collapse text-sm">
		<thead>
			<tr>
				<th class="border-b border-border-line px-3 py-2 text-left">Execution</th>
				<th class="border-b border-border-line px-3 py-2 text-left">Verdict</th>
				<th class="border-b border-border-line px-3 py-2 text-left">Bead</th>
				<th class="border-b border-border-line px-3 py-2 text-left">Started</th>
			</tr>
		</thead>
		<tbody>
			{#each data.executions.edges as edge (edge.node.id)}
				<tr class="hover:bg-bg-elevated dark:hover:bg-dark-bg-elevated">
					<td class="border-b border-border-line px-3 py-2">
						<a class="text-accent-lever dark:text-dark-accent-lever hover:underline" href={execHref(edge.node.id)}>
							{edge.node.id}
						</a>
					</td>
					<td class="border-b border-border-line px-3 py-2">{edge.node.verdict ?? '—'}</td>
					<td class="border-b border-border-line px-3 py-2">{edge.node.beadTitle ?? edge.node.beadId ?? '—'}</td>
					<td class="border-b border-border-line px-3 py-2">{edge.node.createdAt}</td>
				</tr>
			{/each}
		</tbody>
	</table>
</div>
