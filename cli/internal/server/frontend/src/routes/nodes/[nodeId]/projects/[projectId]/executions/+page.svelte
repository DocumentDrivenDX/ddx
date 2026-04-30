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
			return 'badge-status-closed';
		}
		if (lc === 'block' || lc === 'failure' || lc === 'task_failed') {
			return 'badge-status-failed';
		}
		if (lc === 'no_changes' || lc === 'task_no_changes') {
			return 'badge-status-in-progress';
		}
		return 'badge-status-open';
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
			<h1 class="text-headline-md font-headline-md text-fg-ink dark:text-dark-fg-ink">Executions</h1>
			<p class="mt-1 max-w-2xl text-body-sm text-fg-muted dark:text-dark-fg-muted">
				Each row is one <code class="font-mono-code text-mono-code">ddx agent execute-bead</code> attempt bundle from
				<code class="font-mono-code text-mono-code">.ddx/executions/</code>: the prompt that was sent, the verdict that came back, and
				the linked bead and session.
			</p>
		</div>
		<span class="text-body-sm text-fg-muted dark:text-dark-fg-muted">
			{data.executions.totalCount} executions
		</span>
	</div>

	<form
		class="flex flex-wrap items-end gap-3 border border-border-line bg-bg-surface p-3 dark:border-dark-border-line dark:bg-dark-bg-surface"
		onsubmit={(e) => {
			e.preventDefault();
			applyFilters();
		}}
	>
		<label class="flex flex-col text-label-caps font-label-caps uppercase tracking-wide text-fg-muted dark:text-dark-fg-muted">
			<span class="mb-1">Bead</span>
			<input
				type="text"
				bind:value={bead}
				placeholder="ddx-…"
				class="w-40 border border-border-line bg-bg-elevated px-2 py-1 font-mono-code text-mono-code text-fg-ink dark:border-dark-border-line dark:bg-dark-bg-elevated dark:text-dark-fg-ink"
			/>
		</label>
		<label class="flex flex-col text-label-caps font-label-caps uppercase tracking-wide text-fg-muted dark:text-dark-fg-muted">
			<span class="mb-1">Verdict</span>
			<select
				bind:value={verdict}
				class="w-32 border border-border-line bg-bg-elevated px-2 py-1 text-body-sm text-fg-ink dark:border-dark-border-line dark:bg-dark-bg-elevated dark:text-dark-fg-ink"
			>
				<option value="">Any</option>
				<option value="PASS">PASS</option>
				<option value="BLOCK">BLOCK</option>
				<option value="success">success</option>
				<option value="failure">failure</option>
				<option value="no_changes">no_changes</option>
			</select>
		</label>
		<label class="flex flex-col text-label-caps font-label-caps uppercase tracking-wide text-fg-muted dark:text-dark-fg-muted">
			<span class="mb-1">Harness</span>
			<input
				type="text"
				bind:value={harness}
				placeholder="claude / codex / agent"
				class="w-44 border border-border-line bg-bg-elevated px-2 py-1 font-mono-code text-mono-code text-fg-ink dark:border-dark-border-line dark:bg-dark-bg-elevated dark:text-dark-fg-ink"
			/>
		</label>
		<label class="flex flex-1 flex-col text-label-caps font-label-caps uppercase tracking-wide text-fg-muted dark:text-dark-fg-muted">
			<span class="mb-1">Search</span>
			<input
				type="text"
				bind:value={search}
				placeholder="bead title / id"
				class="border border-border-line bg-bg-elevated px-2 py-1 text-body-sm text-fg-ink dark:border-dark-border-line dark:bg-dark-bg-elevated dark:text-dark-fg-ink"
			/>
		</label>
		<button
			type="submit"
			class="bg-accent-lever px-3 py-1.5 text-body-sm font-medium text-bg-elevated hover:opacity-90 dark:bg-dark-accent-lever dark:text-dark-bg-canvas"
		>
			Apply
		</button>
		<button
			type="button"
			onclick={clearFilters}
			class="border border-border-line px-3 py-1.5 text-body-sm text-fg-muted hover:bg-bg-surface dark:border-dark-border-line dark:text-dark-fg-muted dark:hover:bg-dark-bg-surface"
		>
			Clear
		</button>
	</form>

	<div class="overflow-hidden border border-border-line dark:border-dark-border-line">
		<table class="w-full text-sm">
			<thead>
				<tr class="border-b border-border-line bg-bg-surface dark:border-dark-border-line dark:bg-dark-bg-surface">
					<th class="px-4 py-3 text-left text-label-caps font-label-caps uppercase tracking-wide text-fg-muted dark:text-dark-fg-muted">Created</th>
					<th class="px-4 py-3 text-left text-label-caps font-label-caps uppercase tracking-wide text-fg-muted dark:text-dark-fg-muted">Bead</th>
					<th class="px-4 py-3 text-left text-label-caps font-label-caps uppercase tracking-wide text-fg-muted dark:text-dark-fg-muted">Harness</th>
					<th class="px-4 py-3 text-left text-label-caps font-label-caps uppercase tracking-wide text-fg-muted dark:text-dark-fg-muted">Verdict</th>
					<th class="px-4 py-3 text-right text-label-caps font-label-caps uppercase tracking-wide text-fg-muted dark:text-dark-fg-muted">Duration</th>
					<th class="px-4 py-3 text-right text-label-caps font-label-caps uppercase tracking-wide text-fg-muted dark:text-dark-fg-muted">Cost</th>
				</tr>
			</thead>
			<tbody>
				{#each rows as exec (exec.id)}
					<tr class="border-b border-border-line last:border-0 hover:bg-bg-surface dark:border-dark-border-line dark:hover:bg-dark-bg-surface">
						<td class="px-4 py-3">
							<a class="font-mono-code text-mono-code text-accent-lever hover:underline dark:text-dark-accent-lever" href={detailHref(exec.id)}>
								{fmtDate(exec.createdAt)}
							</a>
						</td>
						<td class="px-4 py-3">
							{#if exec.beadId}
								<a class="font-mono-code text-mono-code text-accent-lever hover:underline dark:text-dark-accent-lever" href={beadHref(exec.beadId)}>
									{exec.beadId}
								</a>
								{#if exec.beadTitle}
									<div class="truncate text-body-sm text-fg-muted dark:text-dark-fg-muted">{exec.beadTitle}</div>
								{/if}
							{:else}
								<span class="text-fg-muted dark:text-dark-fg-muted">—</span>
							{/if}
						</td>
						<td class="px-4 py-3 text-fg-ink dark:text-dark-fg-ink">
							<span>{exec.harness ?? '—'}</span>
							{#if exec.model}
								<span class="ml-1 font-mono-code text-mono-code text-fg-muted dark:text-dark-fg-muted">{exec.model}</span>
							{/if}
						</td>
						<td class="px-4 py-3">
							{#if exec.verdict}
								<span
									class="inline-flex border px-1.5 py-0.5 font-mono-code text-mono-code uppercase {verdictClass(exec.verdict)}"
								>
									{exec.verdict}
								</span>
							{:else}
								<span class="text-fg-muted dark:text-dark-fg-muted">—</span>
							{/if}
						</td>
						<td class="px-4 py-3 text-right font-mono-code text-mono-code text-fg-muted dark:text-dark-fg-muted">
							{fmtDuration(exec.durationMs)}
						</td>
						<td class="px-4 py-3 text-right font-mono-code text-mono-code text-fg-muted dark:text-dark-fg-muted">
							{fmtCost(exec.costUsd)}
						</td>
					</tr>
				{/each}
				{#if rows.length === 0}
					<tr>
						<td colspan="6" class="px-4 py-8 text-center text-body-sm text-fg-muted dark:text-dark-fg-muted">
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
			class="border border-border-line px-3 py-1.5 text-body-sm text-fg-muted hover:bg-bg-surface disabled:cursor-not-allowed disabled:opacity-40 dark:border-dark-border-line dark:text-dark-fg-muted dark:hover:bg-dark-bg-surface"
		>
			← Previous
		</button>
		<span class="text-label-caps font-label-caps uppercase tracking-wide text-fg-muted dark:text-dark-fg-muted">
			{rows.length} shown
		</span>
		<button
			onclick={goNext}
			disabled={!data.executions.pageInfo.hasNextPage}
			class="border border-border-line px-3 py-1.5 text-body-sm text-fg-muted hover:bg-bg-surface disabled:cursor-not-allowed disabled:opacity-40 dark:border-dark-border-line dark:text-dark-fg-muted dark:hover:bg-dark-bg-surface"
		>
			Next →
		</button>
	</div>
</div>
