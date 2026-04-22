<script lang="ts">
	import type { PageData } from './$types';

	let { data }: { data: PageData } = $props();

	// Track which sessions are expanded
	let expanded = $state<Set<string>>(new Set());

	function toggle(id: string) {
		const next = new Set(expanded);
		if (next.has(id)) {
			next.delete(id);
		} else {
			next.add(id);
		}
		expanded = next;
	}

	// Aggregate token summary
	const summary = $derived.by(() => {
		let totalCost = 0;
		let totalPrompt = 0;
		let totalCompletion = 0;
		let totalCached = 0;
		let totalTokens = 0;
		for (const edge of data.sessions.edges) {
			const s = edge.node;
			if (s.cost != null) totalCost += s.cost;
			if (s.tokens) {
				totalPrompt += s.tokens.prompt ?? 0;
				totalCompletion += s.tokens.completion ?? 0;
				totalCached += s.tokens.cached ?? 0;
				totalTokens += s.tokens.total ?? 0;
			}
		}
		const cacheRate = totalTokens > 0 ? Math.round((totalCached / totalTokens) * 100) : 0;
		return { totalCost, totalPrompt, totalCompletion, totalCached, totalTokens, cacheRate };
	});

	function fmtDuration(ms: number): string {
		if (ms < 1000) return `${ms}ms`;
		if (ms < 60_000) return `${(ms / 1000).toFixed(1)}s`;
		const m = Math.floor(ms / 60_000);
		const s = Math.floor((ms % 60_000) / 1000);
		return `${m}m ${s}s`;
	}

	function fmtDate(iso: string): string {
		return new Date(iso).toLocaleString();
	}

	function fmtCost(c: number | null): string {
		if (c == null) return '—';
		return `$${c.toFixed(4)}`;
	}

	function statusClass(status: string): string {
		switch (status) {
			case 'completed':
				return 'text-status-completed';
			case 'running':
				return 'text-status-running';
			case 'failed':
				return 'text-status-failed';
			default:
				return 'text-gray-500 dark:text-gray-400';
		}
	}
</script>

<div class="space-y-4">
	<div class="flex items-center justify-between">
		<h1 class="text-xl font-semibold dark:text-white">Sessions</h1>
		<span class="text-sm text-gray-700 dark:text-gray-300">
			{data.sessions.totalCount} sessions
		</span>
	</div>

	<!-- Token summary -->
	<div class="grid grid-cols-2 gap-3 sm:grid-cols-3 lg:grid-cols-6">
		<div
			class="rounded-lg border border-gray-200 bg-gray-50 p-3 dark:border-gray-700 dark:bg-gray-800"
		>
			<div class="text-xs text-gray-700 dark:text-gray-300">Sessions</div>
			<div class="mt-1 text-lg font-semibold dark:text-white">{data.sessions.totalCount}</div>
		</div>
		<div
			class="rounded-lg border border-gray-200 bg-gray-50 p-3 dark:border-gray-700 dark:bg-gray-800"
		>
			<div class="text-xs text-gray-700 dark:text-gray-300">Total Cost</div>
			<div class="mt-1 text-lg font-semibold dark:text-white">{fmtCost(summary.totalCost)}</div>
		</div>
		<div
			class="rounded-lg border border-gray-200 bg-gray-50 p-3 dark:border-gray-700 dark:bg-gray-800"
		>
			<div class="text-xs text-gray-700 dark:text-gray-300">Total Tokens</div>
			<div class="mt-1 text-lg font-semibold dark:text-white">
				{summary.totalTokens.toLocaleString()}
			</div>
		</div>
		<div
			class="rounded-lg border border-gray-200 bg-gray-50 p-3 dark:border-gray-700 dark:bg-gray-800"
		>
			<div class="text-xs text-gray-700 dark:text-gray-300">Prompt</div>
			<div class="mt-1 text-lg font-semibold dark:text-white">
				{summary.totalPrompt.toLocaleString()}
			</div>
		</div>
		<div
			class="rounded-lg border border-gray-200 bg-gray-50 p-3 dark:border-gray-700 dark:bg-gray-800"
		>
			<div class="text-xs text-gray-700 dark:text-gray-300">Completion</div>
			<div class="mt-1 text-lg font-semibold dark:text-white">
				{summary.totalCompletion.toLocaleString()}
			</div>
		</div>
		<div
			class="rounded-lg border border-gray-200 bg-gray-50 p-3 dark:border-gray-700 dark:bg-gray-800"
		>
			<div class="text-xs text-gray-700 dark:text-gray-300">Cache Hit</div>
			<div class="mt-1 text-lg font-semibold dark:text-white">{summary.cacheRate}%</div>
		</div>
	</div>

	<!-- Sessions list -->
	<div class="overflow-hidden rounded-lg border border-gray-200 dark:border-gray-700">
		<table class="w-full text-sm">
			<thead>
				<tr class="border-b border-gray-200 bg-gray-50 dark:border-gray-700 dark:bg-gray-800">
					<th class="w-6 px-4 py-3"></th>
					<th class="px-4 py-3 text-left font-medium text-gray-600 dark:text-gray-300">ID</th>
					<th class="px-4 py-3 text-left font-medium text-gray-600 dark:text-gray-300"
						>Harness / Model</th
					>
					<th class="px-4 py-3 text-left font-medium text-gray-600 dark:text-gray-300">Status</th>
					<th class="px-4 py-3 text-left font-medium text-gray-600 dark:text-gray-300">Started</th>
					<th class="px-4 py-3 text-right font-medium text-gray-600 dark:text-gray-300">Duration</th
					>
					<th class="px-4 py-3 text-right font-medium text-gray-600 dark:text-gray-300">Cost</th>
					<th class="px-4 py-3 text-right font-medium text-gray-600 dark:text-gray-300">Tokens</th>
				</tr>
			</thead>
			<tbody>
				{#each data.sessions.edges as edge (edge.cursor)}
					{@const s = edge.node}
					{@const isExpanded = expanded.has(s.id)}
					<tr
						onclick={() => toggle(s.id)}
						class="cursor-pointer border-b border-gray-100 last:border-0 hover:bg-gray-50 dark:border-gray-700 dark:hover:bg-gray-800 {isExpanded
							? 'bg-blue-50 dark:bg-blue-900/20'
							: ''}"
					>
						<td class="px-4 py-3 text-gray-400 dark:text-gray-500">
							{isExpanded ? '▾' : '▸'}
						</td>
						<td class="px-4 py-3 font-mono text-xs text-gray-500 dark:text-gray-400">
							{s.id.slice(0, 8)}
						</td>
						<td class="px-4 py-3 text-gray-900 dark:text-gray-100">
							<span>{s.harness}</span>
							<span class="ml-1 text-xs text-gray-400 dark:text-gray-500">{s.model}</span>
						</td>
						<td class="px-4 py-3">
							<span class="font-medium {statusClass(s.status)}">{s.status}</span>
						</td>
						<td class="px-4 py-3 text-xs text-gray-500 dark:text-gray-400">
							{fmtDate(s.startedAt)}
						</td>
						<td class="px-4 py-3 text-right text-gray-600 dark:text-gray-300">
							{fmtDuration(s.durationMs)}
						</td>
						<td class="px-4 py-3 text-right font-mono text-xs text-gray-600 dark:text-gray-300">
							{fmtCost(s.cost)}
						</td>
						<td class="px-4 py-3 text-right font-mono text-xs text-gray-600 dark:text-gray-300">
							{s.tokens?.total?.toLocaleString() ?? '—'}
						</td>
					</tr>
					{#if isExpanded}
						<tr
							class="border-b border-gray-100 bg-blue-50/50 dark:border-gray-700 dark:bg-blue-900/10"
						>
							<td colspan="8" class="px-6 py-4">
								<div class="grid grid-cols-2 gap-4 text-sm sm:grid-cols-4">
									<div>
										<div class="text-xs font-medium text-gray-500 dark:text-gray-400">Bead</div>
										<div class="mt-1 font-mono text-xs dark:text-gray-200">
											{s.beadId ?? '—'}
										</div>
									</div>
									<div>
										<div class="text-xs font-medium text-gray-500 dark:text-gray-400">Effort</div>
										<div class="mt-1 dark:text-gray-200">{s.effort}</div>
									</div>
									<div>
										<div class="text-xs font-medium text-gray-500 dark:text-gray-400">Outcome</div>
										<div class="mt-1 dark:text-gray-200">{s.outcome ?? '—'}</div>
									</div>
									<div>
										<div class="text-xs font-medium text-gray-500 dark:text-gray-400">Ended</div>
										<div class="mt-1 text-xs dark:text-gray-200">
											{s.endedAt ? fmtDate(s.endedAt) : '—'}
										</div>
									</div>
									{#if s.tokens}
										<div>
											<div class="text-xs font-medium text-gray-500 dark:text-gray-400">
												Prompt tokens
											</div>
											<div class="mt-1 font-mono text-xs dark:text-gray-200">
												{s.tokens.prompt?.toLocaleString() ?? '—'}
											</div>
										</div>
										<div>
											<div class="text-xs font-medium text-gray-500 dark:text-gray-400">
												Completion tokens
											</div>
											<div class="mt-1 font-mono text-xs dark:text-gray-200">
												{s.tokens.completion?.toLocaleString() ?? '—'}
											</div>
										</div>
										<div>
											<div class="text-xs font-medium text-gray-500 dark:text-gray-400">
												Cached tokens
											</div>
											<div class="mt-1 font-mono text-xs dark:text-gray-200">
												{s.tokens.cached?.toLocaleString() ?? '—'}
											</div>
										</div>
										<div>
											<div class="text-xs font-medium text-gray-500 dark:text-gray-400">
												Total tokens
											</div>
											<div class="mt-1 font-mono text-xs dark:text-gray-200">
												{s.tokens.total?.toLocaleString() ?? '—'}
											</div>
										</div>
									{/if}
									{#if s.detail}
										<div class="col-span-2 sm:col-span-4">
											<div class="text-xs font-medium text-gray-500 dark:text-gray-400">Detail</div>
											<div class="mt-1 text-xs text-gray-700 dark:text-gray-300">{s.detail}</div>
										</div>
									{/if}
								</div>
							</td>
						</tr>
					{/if}
				{/each}
				{#if data.sessions.edges.length === 0}
					<tr>
						<td colspan="8" class="px-4 py-8 text-center text-gray-700 dark:text-gray-300">
							No sessions found for this project.
						</td>
					</tr>
				{/if}
			</tbody>
		</table>
	</div>
</div>
