<script lang="ts">
	import { createClient } from '$lib/gql/client';
	import { invalidateAll } from '$app/navigation';
	import { onMount } from 'svelte';
	import type { PageData } from './$types';
	import { SESSION_DETAIL_QUERY, SESSION_EXECUTION_QUERY, type SessionNode } from './+page';

	let { data }: { data: PageData } = $props();

	// Track which sessions are expanded
	let expanded = $state<Set<string>>(new Set());
	let sessionBodies = $state<Record<string, Pick<SessionNode, 'prompt' | 'response' | 'stderr'>>>(
		{}
	);
	let sessionExecutions = $state<Record<string, string | null>>({});

	function executionHref(executionId: string): string {
		return `/nodes/${data.nodeId}/projects/${data.projectId}/executions/${executionId}`;
	}

	onMount(() => {
		const timer = window.setInterval(() => {
			void invalidateAll();
		}, 2000);
		return () => window.clearInterval(timer);
	});

	async function toggle(id: string) {
		const next = new Set(expanded);
		if (next.has(id)) {
			next.delete(id);
		} else {
			next.add(id);
			if (!sessionBodies[id]) {
				const client = createClient(fetch);
				const detail = await client.request<{ agentSession: SessionNode | null }>(
					SESSION_DETAIL_QUERY,
					{ id }
				);
				if (detail.agentSession) {
					sessionBodies = {
						...sessionBodies,
						[id]: {
							prompt: detail.agentSession.prompt,
							response: detail.agentSession.response,
							stderr: detail.agentSession.stderr
						}
					};
				}
			}
			if (sessionExecutions[id] === undefined) {
				try {
					const client2 = createClient(fetch);
					const exec = await client2.request<{ executionBySessionId: { id: string } | null }>(
						SESSION_EXECUTION_QUERY,
						{ projectID: data.projectId, sessionID: id }
					);
					sessionExecutions = {
						...sessionExecutions,
						[id]: exec.executionBySessionId?.id ?? null
					};
				} catch {
					sessionExecutions = { ...sessionExecutions, [id]: null };
				}
			}
		}
		expanded = next;
	}

	const recordingGap = $derived.by(() => {
		const sorted = [...data.sessions.edges].sort(
			(a, b) => new Date(a.node.startedAt).getTime() - new Date(b.node.startedAt).getTime()
		);
		for (let i = 1; i < sorted.length; i++) {
			const prev = new Date(sorted[i - 1].node.startedAt);
			const next = new Date(sorted[i].node.startedAt);
			if (next.getTime() - prev.getTime() > 24 * 60 * 60 * 1000) {
				return `No sessions recorded between ${prev.toLocaleDateString()} and ${next.toLocaleDateString()}`;
			}
		}
		return null;
	});

	// Aggregate token summary
	const summary = $derived.by(() => {
		let totalPrompt = 0;
		let totalCompletion = 0;
		let totalCached = 0;
		let totalTokens = 0;
		let paidSessions = 0;
		let subscriptionSessions = 0;
		for (const edge of data.sessions.edges) {
			const s = edge.node;
			if (s.billingMode === 'paid') paidSessions++;
			if (s.billingMode === 'subscription') subscriptionSessions++;
			if (s.tokens) {
				totalPrompt += s.tokens.prompt ?? 0;
				totalCompletion += s.tokens.completion ?? 0;
				totalCached += s.tokens.cached ?? 0;
				totalTokens += s.tokens.total ?? 0;
			}
		}
		const cacheRate = totalTokens > 0 ? Math.round((totalCached / totalTokens) * 100) : 0;
		return {
			totalPrompt,
			totalCompletion,
			totalCached,
			totalTokens,
			cacheRate,
			paidSessions,
			subscriptionSessions
		};
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

	function fmtCardCost(value: number, hasSessions: boolean): string {
		if (!hasSessions) return '—';
		return `$${value.toFixed(2)}`;
	}

	function fmtLocalValue(): string {
		if (data.costSummary.localSessionCount === 0) return '0';
		if (data.costSummary.localEstimatedUsd != null) {
			return `$${data.costSummary.localEstimatedUsd.toFixed(2)} est.`;
		}
		return data.costSummary.localSessionCount.toLocaleString();
	}

	function billingBadge(mode: SessionNode['billingMode']): string {
		switch (mode) {
			case 'paid':
				return 'cash';
			case 'subscription':
				return 'sub';
			case 'local':
				return 'local';
			default:
				return mode;
		}
	}

	function billingDescription(mode: SessionNode['billingMode']): string {
		switch (mode) {
			case 'paid':
				return 'Billed by pay-per-token APIs (OpenRouter, direct API keys)';
			case 'subscription':
				return 'Dollar-equivalent for tokens consumed under Claude Code / Codex subscriptions. Not cash out of pocket.';
			case 'local':
				return 'Sessions served locally. Compute cost not modeled.';
			default:
				return 'Cost bucket is unknown.';
		}
	}

	function billingBadgeClass(mode: SessionNode['billingMode']): string {
		switch (mode) {
			case 'paid':
				return 'border-emerald-300 bg-emerald-50 text-emerald-700 dark:border-emerald-700 dark:bg-emerald-950 dark:text-emerald-200';
			case 'subscription':
				return 'border-sky-300 bg-sky-50 text-sky-700 dark:border-sky-700 dark:bg-sky-950 dark:text-sky-200';
			case 'local':
				return 'border-gray-300 bg-gray-100 text-gray-700 dark:border-gray-600 dark:bg-gray-800 dark:text-gray-200';
			default:
				return 'border-gray-300 bg-gray-100 text-gray-700 dark:border-gray-600 dark:bg-gray-800 dark:text-gray-200';
		}
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

	function workersHref(): string {
		return `/nodes/${data.nodeId}/projects/${data.projectId}/workers`;
	}

	function workerHref(workerId: string): string {
		return `/nodes/${data.nodeId}/projects/${data.projectId}/workers/${workerId}`;
	}
</script>

<div class="space-y-4">
	<div class="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
		<div>
			<h1 class="text-xl font-semibold dark:text-white">Sessions</h1>
			<p class="mt-1 max-w-2xl text-sm text-gray-600 dark:text-gray-300">
				Sessions are immutable agent-run history; Workers are the live queue-draining processes that
				can produce many sessions.
			</p>
			<a
				class="mt-2 inline-flex text-sm text-blue-600 hover:underline dark:text-blue-400"
				href={workersHref()}
			>
				Workers →
			</a>
		</div>
		<span class="text-sm text-gray-700 dark:text-gray-300">
			{data.sessions.totalCount} sessions
		</span>
	</div>

	{#if recordingGap}
		<div
			class="border-l-4 border-amber-500 bg-amber-50 px-3 py-2 text-sm text-amber-900 dark:bg-amber-950 dark:text-amber-100"
		>
			{recordingGap}
		</div>
	{/if}

	<!-- Token summary -->
	<div class="grid grid-cols-2 gap-3 sm:grid-cols-3 lg:grid-cols-8">
		<div
			class="rounded-lg border border-gray-200 bg-gray-50 p-3 dark:border-gray-700 dark:bg-gray-800"
		>
			<div class="text-xs text-gray-700 dark:text-gray-300">Sessions</div>
			<div class="mt-1 text-lg font-semibold dark:text-white">{data.sessions.totalCount}</div>
		</div>
		<div
			aria-label="Cash paid. Billed by pay-per-token APIs (OpenRouter, direct API keys)"
			class="rounded-lg border border-gray-200 bg-gray-50 p-3 dark:border-gray-700 dark:bg-gray-800"
		>
			<div
				class="group relative inline-flex items-center gap-1 text-xs text-gray-700 dark:text-gray-300"
			>
				<span>Cash paid</span>
				<span
					class="inline-flex h-4 w-4 items-center justify-center rounded-full border border-gray-300 text-[10px] dark:border-gray-600"
					>?</span
				>
				<span
					role="tooltip"
					class="pointer-events-none absolute top-6 left-0 z-20 hidden w-56 rounded border border-gray-200 bg-white p-2 text-xs text-gray-700 shadow-lg group-focus-within:block group-hover:block dark:border-gray-700 dark:bg-gray-900 dark:text-gray-200"
				>
					Billed by pay-per-token APIs (OpenRouter, direct API keys)
				</span>
			</div>
			<div class="mt-1 text-lg font-semibold dark:text-white">
				{fmtCardCost(
					data.costSummary.cashUsd,
					summary.paidSessions > 0 || data.costSummary.cashUsd > 0
				)}
			</div>
		</div>
		<div
			aria-label="Subscription value. Dollar-equivalent for tokens consumed under Claude Code / Codex subscriptions. Not cash out of pocket."
			class="rounded-lg border border-gray-200 bg-gray-50 p-3 dark:border-gray-700 dark:bg-gray-800"
		>
			<div
				class="group relative inline-flex items-center gap-1 text-xs text-gray-700 dark:text-gray-300"
			>
				<span>Subscription value</span>
				<span
					class="inline-flex h-4 w-4 items-center justify-center rounded-full border border-gray-300 text-[10px] dark:border-gray-600"
					>?</span
				>
				<span
					role="tooltip"
					class="pointer-events-none absolute top-6 left-0 z-20 hidden w-64 rounded border border-gray-200 bg-white p-2 text-xs text-gray-700 shadow-lg group-focus-within:block group-hover:block dark:border-gray-700 dark:bg-gray-900 dark:text-gray-200"
				>
					Dollar-equivalent for tokens consumed under Claude Code / Codex subscriptions. Not cash
					out of pocket.
				</span>
			</div>
			<div class="mt-1 text-lg font-semibold dark:text-white">
				{fmtCardCost(
					data.costSummary.subscriptionEquivUsd,
					summary.subscriptionSessions > 0 || data.costSummary.subscriptionEquivUsd > 0
				)}
			</div>
		</div>
		<div
			aria-label="Local sessions. Sessions served locally. Compute cost not modeled."
			class="rounded-lg border border-gray-200 bg-gray-50 p-3 dark:border-gray-700 dark:bg-gray-800"
		>
			<div
				class="group relative inline-flex items-center gap-1 text-xs text-gray-700 dark:text-gray-300"
			>
				<span>Local sessions</span>
				<span
					class="inline-flex h-4 w-4 items-center justify-center rounded-full border border-gray-300 text-[10px] dark:border-gray-600"
					>?</span
				>
				<span
					role="tooltip"
					class="pointer-events-none absolute top-6 left-0 z-20 hidden w-56 rounded border border-gray-200 bg-white p-2 text-xs text-gray-700 shadow-lg group-focus-within:block group-hover:block dark:border-gray-700 dark:bg-gray-900 dark:text-gray-200"
				>
					Sessions served locally. Compute cost not modeled.
				</span>
			</div>
			<div class="mt-1 text-lg font-semibold dark:text-white">{fmtLocalValue()}</div>
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
							<span class="ml-1 text-xs text-gray-600 dark:text-gray-400">{s.model}</span>
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
							<div class="flex items-center justify-end gap-2">
								<span>{fmtCost(s.cost)}</span>
								<span
									class="group relative inline-flex min-w-10 justify-center rounded border px-1.5 py-0.5 font-sans text-[10px] leading-none font-semibold uppercase {billingBadgeClass(
										s.billingMode
									)}"
									aria-label="{billingBadge(s.billingMode)}: {billingDescription(s.billingMode)}"
								>
									{billingBadge(s.billingMode)}
									<span
										role="tooltip"
										class="pointer-events-none absolute top-5 right-0 z-20 hidden w-64 rounded border border-gray-200 bg-white p-2 text-left text-xs font-normal text-gray-700 normal-case shadow-lg group-focus-within:block group-hover:block dark:border-gray-700 dark:bg-gray-900 dark:text-gray-200"
									>
										{billingDescription(s.billingMode)}
									</span>
								</span>
							</div>
						</td>
						<td class="px-4 py-3 text-right font-mono text-xs text-gray-600 dark:text-gray-300">
							{s.tokens?.total?.toLocaleString() ?? '—'}
						</td>
					</tr>
					{#if isExpanded}
						{@const bodies = sessionBodies[s.id]}
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
										<div class="text-xs font-medium text-gray-500 dark:text-gray-400">Worker</div>
										<div class="mt-1 font-mono text-xs dark:text-gray-200">
											{#if s.workerId}
												<a
													href={workerHref(s.workerId)}
													onclick={(event) => event.stopPropagation()}
													class="text-blue-600 hover:underline dark:text-blue-400"
												>
													{s.workerId}
												</a>
											{:else}
												—
											{/if}
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
										<div class="text-xs font-medium text-gray-500 dark:text-gray-400">Execution</div>
										<div class="mt-1 font-mono text-xs dark:text-gray-200">
											{#if sessionExecutions[s.id]}
												<a
													href={executionHref(sessionExecutions[s.id] as string)}
													onclick={(event) => event.stopPropagation()}
													class="text-blue-600 hover:underline dark:text-blue-400"
												>
													{(sessionExecutions[s.id] as string).slice(0, 18)}…
												</a>
											{:else}
												—
											{/if}
										</div>
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
									{#if bodies?.prompt}
										<div class="col-span-2 sm:col-span-4">
											<div class="text-xs font-medium text-gray-500 dark:text-gray-400">Prompt</div>
											<pre
												class="mt-1 max-h-56 overflow-auto rounded border border-gray-200 bg-white p-3 text-xs whitespace-pre-wrap text-gray-800 dark:border-gray-700 dark:bg-gray-950 dark:text-gray-200">{bodies.prompt}</pre>
										</div>
									{/if}
									{#if bodies?.response}
										<div class="col-span-2 sm:col-span-4">
											<div class="text-xs font-medium text-gray-500 dark:text-gray-400">
												Response
											</div>
											<pre
												class="mt-1 max-h-56 overflow-auto rounded border border-gray-200 bg-white p-3 text-xs whitespace-pre-wrap text-gray-800 dark:border-gray-700 dark:bg-gray-950 dark:text-gray-200">{bodies.response}</pre>
										</div>
									{/if}
									{#if bodies?.stderr}
										<div class="col-span-2 sm:col-span-4">
											<div class="text-xs font-medium text-gray-500 dark:text-gray-400">Stderr</div>
											<pre
												class="mt-1 max-h-56 overflow-auto rounded border border-gray-200 bg-white p-3 text-xs whitespace-pre-wrap text-gray-800 dark:border-gray-700 dark:bg-gray-950 dark:text-gray-200">{bodies.stderr}</pre>
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
