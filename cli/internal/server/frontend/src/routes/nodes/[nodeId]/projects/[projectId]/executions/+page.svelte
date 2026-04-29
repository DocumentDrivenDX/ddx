<script lang="ts">
	import type { PageData } from './$types';
	import { goto } from '$app/navigation';
	import type { ExecutionListNode } from './+page';

	let { data }: { data: PageData } = $props();

	let bead = $state(data.filters.bead);
	let verdict = $state(data.filters.verdict);
	let harness = $state(data.filters.harness);
	let search = $state(data.filters.search);

	function applyFilters() {
		const params = new URLSearchParams();
		if (bead) params.set('bead', bead);
		if (verdict) params.set('verdict', verdict);
		if (harness) params.set('harness', harness);
		if (search) params.set('q', search);
		const qs = params.toString();
		goto(
			`/nodes/${data.nodeId}/projects/${data.projectId}/executions${qs ? `?${qs}` : ''}`,
			{ keepFocus: true, noScroll: true }
		);
	}

	function clearFilters() {
		bead = '';
		verdict = '';
		harness = '';
		search = '';
		applyFilters();
	}

	function detailHref(id: string): string {
		return `/nodes/${data.nodeId}/projects/${data.projectId}/executions/${id}`;
	}

	function beadHref(beadId: string): string {
		return `/nodes/${data.nodeId}/projects/${data.projectId}/beads/${beadId}`;
	}

	function fmtDuration(ms: number | null): string {
		if (ms == null) return '—';
		if (ms < 1000) return `${ms}ms`;
		if (ms < 60_000) return `${(ms / 1000).toFixed(1)}s`;
		const m = Math.floor(ms / 60_000);
		const s = Math.floor((ms % 60_000) / 1000);
		return `${m}m ${s}s`;
	}

	function fmtCost(c: number | null): string {
		if (c == null) return '—';
		return `$${c.toFixed(4)}`;
	}

	function fmtDate(iso: string | null): string {
		if (!iso) return '—';
		return new Date(iso).toLocaleString();
	}

	function verdictClass(v: string | null): string {
		const lc = (v ?? '').toLowerCase();
		if (lc === 'pass' || lc === 'success' || lc === 'task_succeeded') {
			return 'border-emerald-300 bg-emerald-50 text-emerald-700 dark:border-emerald-700 dark:bg-emerald-950 dark:text-emerald-200';
		}
		if (lc === 'block' || lc === 'failure' || lc === 'task_failed') {
			return 'border-red-300 bg-red-50 text-red-700 dark:border-red-800 dark:bg-red-950 dark:text-red-200';
		}
		if (lc === 'no_changes' || lc === 'task_no_changes') {
			return 'border-amber-300 bg-amber-50 text-amber-700 dark:border-amber-700 dark:bg-amber-950 dark:text-amber-200';
		}
		return 'border-gray-300 bg-gray-100 text-gray-700 dark:border-gray-600 dark:bg-gray-800 dark:text-gray-200';
	}

	function goNext() {
		const cursor = data.executions.pageInfo.endCursor;
		if (!cursor) return;
		const params = new URLSearchParams();
		if (bead) params.set('bead', bead);
		if (verdict) params.set('verdict', verdict);
		if (harness) params.set('harness', harness);
		if (search) params.set('q', search);
		params.set('after', cursor);
		goto(`/nodes/${data.nodeId}/projects/${data.projectId}/executions?${params.toString()}`);
	}

	function goPrev() {
		const params = new URLSearchParams();
		if (bead) params.set('bead', bead);
		if (verdict) params.set('verdict', verdict);
		if (harness) params.set('harness', harness);
		if (search) params.set('q', search);
		const qs = params.toString();
		goto(`/nodes/${data.nodeId}/projects/${data.projectId}/executions${qs ? `?${qs}` : ''}`);
	}

	const rows = $derived(data.executions.edges.map((e: { node: ExecutionListNode }) => e.node));
</script>

<div class="space-y-4">
	<div class="flex items-start justify-between">
		<div>
			<h1 class="text-xl font-semibold dark:text-white">Executions</h1>
			<p class="mt-1 max-w-2xl text-sm text-gray-600 dark:text-gray-300">
				Each row is one <code>ddx agent execute-bead</code> attempt bundle from
				<code>.ddx/executions/</code>: the prompt that was sent, the verdict that came back, and
				the linked bead and session.
			</p>
		</div>
		<span class="text-sm text-gray-700 dark:text-gray-300">
			{data.executions.totalCount} executions
		</span>
	</div>

	<form
		class="flex flex-wrap items-end gap-3 rounded-lg border border-gray-200 bg-gray-50 p-3 dark:border-gray-700 dark:bg-gray-800"
		onsubmit={(e) => {
			e.preventDefault();
			applyFilters();
		}}
	>
		<label class="flex flex-col text-xs text-gray-600 dark:text-gray-300">
			<span class="mb-1">Bead</span>
			<input
				type="text"
				bind:value={bead}
				placeholder="ddx-…"
				class="w-40 rounded border border-gray-300 px-2 py-1 text-sm dark:border-gray-600 dark:bg-gray-900 dark:text-white"
			/>
		</label>
		<label class="flex flex-col text-xs text-gray-600 dark:text-gray-300">
			<span class="mb-1">Verdict</span>
			<select
				bind:value={verdict}
				class="w-32 rounded border border-gray-300 px-2 py-1 text-sm dark:border-gray-600 dark:bg-gray-900 dark:text-white"
			>
				<option value="">Any</option>
				<option value="PASS">PASS</option>
				<option value="BLOCK">BLOCK</option>
				<option value="success">success</option>
				<option value="failure">failure</option>
				<option value="no_changes">no_changes</option>
			</select>
		</label>
		<label class="flex flex-col text-xs text-gray-600 dark:text-gray-300">
			<span class="mb-1">Harness</span>
			<input
				type="text"
				bind:value={harness}
				placeholder="claude / codex / agent"
				class="w-44 rounded border border-gray-300 px-2 py-1 text-sm dark:border-gray-600 dark:bg-gray-900 dark:text-white"
			/>
		</label>
		<label class="flex flex-1 flex-col text-xs text-gray-600 dark:text-gray-300">
			<span class="mb-1">Search</span>
			<input
				type="text"
				bind:value={search}
				placeholder="bead title / id"
				class="rounded border border-gray-300 px-2 py-1 text-sm dark:border-gray-600 dark:bg-gray-900 dark:text-white"
			/>
		</label>
		<button
			type="submit"
			class="rounded bg-blue-600 px-3 py-1.5 text-sm font-medium text-white hover:bg-blue-700"
		>
			Apply
		</button>
		<button
			type="button"
			onclick={clearFilters}
			class="rounded border border-gray-300 px-3 py-1.5 text-sm text-gray-700 hover:bg-gray-100 dark:border-gray-600 dark:text-gray-300 dark:hover:bg-gray-700"
		>
			Clear
		</button>
	</form>

	<div class="overflow-hidden rounded-lg border border-gray-200 dark:border-gray-700">
		<table class="w-full text-sm">
			<thead>
				<tr class="border-b border-gray-200 bg-gray-50 dark:border-gray-700 dark:bg-gray-800">
					<th class="px-4 py-3 text-left font-medium text-gray-600 dark:text-gray-300">Created</th>
					<th class="px-4 py-3 text-left font-medium text-gray-600 dark:text-gray-300">Bead</th>
					<th class="px-4 py-3 text-left font-medium text-gray-600 dark:text-gray-300">Harness</th>
					<th class="px-4 py-3 text-left font-medium text-gray-600 dark:text-gray-300">Verdict</th>
					<th class="px-4 py-3 text-right font-medium text-gray-600 dark:text-gray-300">Duration</th>
					<th class="px-4 py-3 text-right font-medium text-gray-600 dark:text-gray-300">Cost</th>
				</tr>
			</thead>
			<tbody>
				{#each rows as exec (exec.id)}
					<tr class="border-b border-gray-100 last:border-0 hover:bg-gray-50 dark:border-gray-700 dark:hover:bg-gray-800">
						<td class="px-4 py-3 text-xs text-gray-500 dark:text-gray-400">
							<a class="font-mono text-blue-600 hover:underline dark:text-blue-400" href={detailHref(exec.id)}>
								{fmtDate(exec.createdAt)}
							</a>
						</td>
						<td class="px-4 py-3">
							{#if exec.beadId}
								<a class="font-mono text-xs text-blue-600 hover:underline dark:text-blue-400" href={beadHref(exec.beadId)}>
									{exec.beadId}
								</a>
								{#if exec.beadTitle}
									<div class="truncate text-xs text-gray-500 dark:text-gray-400">{exec.beadTitle}</div>
								{/if}
							{:else}
								<span class="text-gray-400 dark:text-gray-500">—</span>
							{/if}
						</td>
						<td class="px-4 py-3 text-gray-900 dark:text-gray-100">
							<span>{exec.harness ?? '—'}</span>
							{#if exec.model}
								<span class="ml-1 text-xs text-gray-600 dark:text-gray-400">{exec.model}</span>
							{/if}
						</td>
						<td class="px-4 py-3">
							{#if exec.verdict}
								<span
									class="inline-flex rounded border px-1.5 py-0.5 font-mono text-[11px] uppercase {verdictClass(exec.verdict)}"
								>
									{exec.verdict}
								</span>
							{:else}
								<span class="text-gray-400 dark:text-gray-500">—</span>
							{/if}
						</td>
						<td class="px-4 py-3 text-right text-gray-600 dark:text-gray-300">
							{fmtDuration(exec.durationMs)}
						</td>
						<td class="px-4 py-3 text-right font-mono text-xs text-gray-600 dark:text-gray-300">
							{fmtCost(exec.costUsd)}
						</td>
					</tr>
				{/each}
				{#if rows.length === 0}
					<tr>
						<td colspan="6" class="px-4 py-8 text-center text-gray-700 dark:text-gray-300">
							No executions found.
						</td>
					</tr>
				{/if}
			</tbody>
		</table>
	</div>

	<div class="flex items-center justify-between">
		<button
			onclick={goPrev}
			disabled={!data.filters.after}
			class="rounded border border-gray-200 px-3 py-1.5 text-sm text-gray-600 hover:bg-gray-50 disabled:cursor-not-allowed disabled:opacity-40 dark:border-gray-700 dark:text-gray-300 dark:hover:bg-gray-800"
		>
			← Previous
		</button>
		<span class="text-xs text-gray-400 dark:text-gray-500">
			{rows.length} shown
		</span>
		<button
			onclick={goNext}
			disabled={!data.executions.pageInfo.hasNextPage}
			class="rounded border border-gray-200 px-3 py-1.5 text-sm text-gray-600 hover:bg-gray-50 disabled:cursor-not-allowed disabled:opacity-40 dark:border-gray-700 dark:text-gray-300 dark:hover:bg-gray-800"
		>
			Next →
		</button>
	</div>
</div>
