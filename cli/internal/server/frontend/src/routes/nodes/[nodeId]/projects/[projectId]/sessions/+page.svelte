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
				return 'badge-billing-paid';
			case 'subscription':
				return 'badge-billing-subscription';
			case 'local':
				return 'badge-billing-local';
			default:
				return 'badge-billing-local';
		}
	}

	function statusBadgeClass(status: string): string {
		switch (status) {
			case 'completed':
				return 'badge-status-completed';
			case 'running':
				return 'badge-status-running';
			case 'failed':
				return 'badge-status-failed';
			default:
				return 'badge-status-open';
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
			<h1 class="text-headline-lg font-headline-lg text-fg-ink dark:text-dark-fg-ink">Sessions</h1>
			<p class="mt-1 max-w-2xl text-body-sm text-fg-muted dark:text-dark-fg-muted">
				Sessions are immutable agent-run history; Workers are the live queue-draining processes that
				can produce many sessions.
			</p>
			<a
				class="mt-2 inline-flex text-body-sm text-accent-lever hover:underline dark:text-dark-accent-lever"
				href={workersHref()}
			>
				Workers →
			</a>
		</div>
		<span class="text-body-sm text-fg-muted dark:text-dark-fg-muted">
			{data.sessions.totalCount} sessions
		</span>
	</div>

	{#if recordingGap}
		<div class="alert-caution border-l-4 px-3 py-2 text-body-sm">
			{recordingGap}
		</div>
	{/if}

	<!-- Token summary -->
	<div class="grid grid-cols-2 gap-3 sm:grid-cols-3 lg:grid-cols-8">
		<div class="border border-border-line bg-bg-surface p-3 dark:border-dark-border-line dark:bg-dark-bg-surface">
			<div class="text-label-caps font-label-caps uppercase tracking-wide text-fg-muted dark:text-dark-fg-muted">Sessions</div>
			<div class="mt-1 text-lg font-semibold text-fg-ink dark:text-dark-fg-ink">{data.sessions.totalCount}</div>
		</div>
		<div
			aria-label="Cash paid. Billed by pay-per-token APIs (OpenRouter, direct API keys)"
			class="border border-border-line bg-bg-surface p-3 dark:border-dark-border-line dark:bg-dark-bg-surface"
		>
			<div
				class="group relative inline-flex items-center gap-1 text-label-caps font-label-caps uppercase tracking-wide text-fg-muted dark:text-dark-fg-muted"
			>
				<span>Cash paid</span>
				<span
					class="inline-flex h-4 w-4 items-center justify-center rounded-full border border-border-line text-[10px] dark:border-dark-border-line"
					>?</span
				>
				<span
					role="tooltip"
					class="pointer-events-none absolute top-6 left-0 z-20 hidden w-56 border border-border-line bg-bg-elevated p-2 text-body-sm text-fg-ink shadow-lg group-focus-within:block group-hover:block dark:border-dark-border-line dark:bg-dark-bg-elevated dark:text-dark-fg-ink"
				>
					Billed by pay-per-token APIs (OpenRouter, direct API keys)
				</span>
			</div>
			<div class="mt-1 text-lg font-semibold text-fg-ink dark:text-dark-fg-ink">
				{fmtCardCost(
					data.costSummary.cashUsd,
					summary.paidSessions > 0 || data.costSummary.cashUsd > 0
				)}
			</div>
		</div>
		<div
			aria-label="Subscription value. Dollar-equivalent for tokens consumed under Claude Code / Codex subscriptions. Not cash out of pocket."
			class="border border-border-line bg-bg-surface p-3 dark:border-dark-border-line dark:bg-dark-bg-surface"
		>
			<div
				class="group relative inline-flex items-center gap-1 text-label-caps font-label-caps uppercase tracking-wide text-fg-muted dark:text-dark-fg-muted"
			>
				<span>Subscription value</span>
				<span
					class="inline-flex h-4 w-4 items-center justify-center rounded-full border border-border-line text-[10px] dark:border-dark-border-line"
					>?</span
				>
				<span
					role="tooltip"
					class="pointer-events-none absolute top-6 left-0 z-20 hidden w-64 border border-border-line bg-bg-elevated p-2 text-body-sm text-fg-ink shadow-lg group-focus-within:block group-hover:block dark:border-dark-border-line dark:bg-dark-bg-elevated dark:text-dark-fg-ink"
				>
					Dollar-equivalent for tokens consumed under Claude Code / Codex subscriptions. Not cash
					out of pocket.
				</span>
			</div>
			<div class="mt-1 text-lg font-semibold text-fg-ink dark:text-dark-fg-ink">
				{fmtCardCost(
					data.costSummary.subscriptionEquivUsd,
					summary.subscriptionSessions > 0 || data.costSummary.subscriptionEquivUsd > 0
				)}
			</div>
		</div>
		<div
			aria-label="Local sessions. Sessions served locally. Compute cost not modeled."
			class="border border-border-line bg-bg-surface p-3 dark:border-dark-border-line dark:bg-dark-bg-surface"
		>
			<div
				class="group relative inline-flex items-center gap-1 text-label-caps font-label-caps uppercase tracking-wide text-fg-muted dark:text-dark-fg-muted"
			>
				<span>Local sessions</span>
				<span
					class="inline-flex h-4 w-4 items-center justify-center rounded-full border border-border-line text-[10px] dark:border-dark-border-line"
					>?</span
				>
				<span
					role="tooltip"
					class="pointer-events-none absolute top-6 left-0 z-20 hidden w-56 border border-border-line bg-bg-elevated p-2 text-body-sm text-fg-ink shadow-lg group-focus-within:block group-hover:block dark:border-dark-border-line dark:bg-dark-bg-elevated dark:text-dark-fg-ink"
				>
					Sessions served locally. Compute cost not modeled.
				</span>
			</div>
			<div class="mt-1 text-lg font-semibold text-fg-ink dark:text-dark-fg-ink">{fmtLocalValue()}</div>
		</div>
		<div class="border border-border-line bg-bg-surface p-3 dark:border-dark-border-line dark:bg-dark-bg-surface">
			<div class="text-label-caps font-label-caps uppercase tracking-wide text-fg-muted dark:text-dark-fg-muted">Total Tokens</div>
			<div class="mt-1 text-lg font-semibold text-fg-ink dark:text-dark-fg-ink">
				{summary.totalTokens.toLocaleString()}
			</div>
		</div>
		<div class="border border-border-line bg-bg-surface p-3 dark:border-dark-border-line dark:bg-dark-bg-surface">
			<div class="text-label-caps font-label-caps uppercase tracking-wide text-fg-muted dark:text-dark-fg-muted">Prompt</div>
			<div class="mt-1 text-lg font-semibold text-fg-ink dark:text-dark-fg-ink">
				{summary.totalPrompt.toLocaleString()}
			</div>
		</div>
		<div class="border border-border-line bg-bg-surface p-3 dark:border-dark-border-line dark:bg-dark-bg-surface">
			<div class="text-label-caps font-label-caps uppercase tracking-wide text-fg-muted dark:text-dark-fg-muted">Completion</div>
			<div class="mt-1 text-lg font-semibold text-fg-ink dark:text-dark-fg-ink">
				{summary.totalCompletion.toLocaleString()}
			</div>
		</div>
		<div class="border border-border-line bg-bg-surface p-3 dark:border-dark-border-line dark:bg-dark-bg-surface">
			<div class="text-label-caps font-label-caps uppercase tracking-wide text-fg-muted dark:text-dark-fg-muted">Cache Hit</div>
			<div class="mt-1 text-lg font-semibold text-fg-ink dark:text-dark-fg-ink">{summary.cacheRate}%</div>
		</div>
	</div>

	<!-- Sessions list -->
	<div class="overflow-hidden border border-border-line dark:border-dark-border-line">
		<table class="w-full border-collapse text-sm">
			<thead>
				<tr class="border-b border-border-line bg-bg-surface dark:border-dark-border-line dark:bg-dark-bg-surface">
					<th class="w-6 px-4 py-3"></th>
					<th class="px-4 py-3 text-left text-label-caps font-label-caps uppercase tracking-wide text-fg-muted dark:text-dark-fg-muted">ID</th>
					<th class="px-4 py-3 text-left text-label-caps font-label-caps uppercase tracking-wide text-fg-muted dark:text-dark-fg-muted"
						>Harness / Model</th
					>
					<th class="px-4 py-3 text-left text-label-caps font-label-caps uppercase tracking-wide text-fg-muted dark:text-dark-fg-muted">Status</th>
					<th class="px-4 py-3 text-left text-label-caps font-label-caps uppercase tracking-wide text-fg-muted dark:text-dark-fg-muted">Started</th>
					<th class="px-4 py-3 text-right text-label-caps font-label-caps uppercase tracking-wide text-fg-muted dark:text-dark-fg-muted">Duration</th
					>
					<th class="px-4 py-3 text-right text-label-caps font-label-caps uppercase tracking-wide text-fg-muted dark:text-dark-fg-muted">Cost</th>
					<th class="px-4 py-3 text-right text-label-caps font-label-caps uppercase tracking-wide text-fg-muted dark:text-dark-fg-muted">Tokens</th>
				</tr>
			</thead>
			<tbody>
				{#each data.sessions.edges as edge (edge.cursor)}
					{@const s = edge.node}
					{@const isExpanded = expanded.has(s.id)}
					<tr
						onclick={() => toggle(s.id)}
						class="cursor-pointer border-b border-border-line last:border-0 hover:bg-bg-surface dark:border-dark-border-line dark:hover:bg-dark-bg-surface {isExpanded
							? 'bg-accent-lever/10 dark:bg-dark-accent-lever/10'
							: ''}"
					>
						<td class="px-4 py-3 text-fg-muted dark:text-dark-fg-muted">
							{isExpanded ? '▾' : '▸'}
						</td>
						<td class="px-4 py-3 font-mono-code text-mono-code text-accent-lever dark:text-dark-accent-lever">
							{s.id.slice(0, 8)}
						</td>
						<td class="px-4 py-3 text-fg-ink dark:text-dark-fg-ink">
							<span>{s.harness}</span>
							<span class="ml-1 font-mono-code text-mono-code text-fg-muted dark:text-dark-fg-muted">{s.model}</span>
						</td>
						<td class="px-4 py-3">
							<span class="inline-block border px-1.5 py-0.5 text-[10px] font-semibold uppercase tracking-wide {statusBadgeClass(s.status)}">{s.status}</span>
						</td>
						<td class="px-4 py-3 text-body-sm text-fg-muted dark:text-dark-fg-muted">
							{fmtDate(s.startedAt)}
						</td>
						<td class="px-4 py-3 text-right text-body-sm text-fg-muted dark:text-dark-fg-muted">
							{fmtDuration(s.durationMs)}
						</td>
						<td class="px-4 py-3 text-right font-mono-code text-mono-code text-fg-muted dark:text-dark-fg-muted">
							<div class="flex items-center justify-end gap-2">
								<span>{fmtCost(s.cost)}</span>
								<span
									class="group relative inline-flex min-w-10 justify-center border px-1.5 py-0.5 text-[10px] leading-none font-semibold uppercase {billingBadgeClass(
										s.billingMode
									)}"
									aria-label="{billingBadge(s.billingMode)}: {billingDescription(s.billingMode)}"
								>
									{billingBadge(s.billingMode)}
									<span
										role="tooltip"
										class="pointer-events-none absolute top-5 right-0 z-20 hidden w-64 border border-border-line bg-bg-elevated p-2 text-left text-body-sm font-normal text-fg-ink normal-case shadow-lg group-focus-within:block group-hover:block dark:border-dark-border-line dark:bg-dark-bg-elevated dark:text-dark-fg-ink"
									>
										{billingDescription(s.billingMode)}
									</span>
								</span>
							</div>
						</td>
						<td class="px-4 py-3 text-right font-mono-code text-mono-code text-fg-muted dark:text-dark-fg-muted">
							{s.tokens?.total?.toLocaleString() ?? '—'}
						</td>
					</tr>
					{#if isExpanded}
						{@const bodies = sessionBodies[s.id]}
						<tr
							class="border-b border-border-line bg-bg-surface dark:border-dark-border-line dark:bg-dark-bg-surface"
						>
							<td colspan="8" class="px-6 py-4">
								<div class="grid grid-cols-2 gap-4 text-sm sm:grid-cols-4">
									<div>
										<div class="text-label-caps font-label-caps uppercase tracking-wide text-fg-muted dark:text-dark-fg-muted">Bead</div>
										<div class="mt-1 font-mono-code text-mono-code text-fg-ink dark:text-dark-fg-ink">
											{s.beadId ?? '—'}
										</div>
									</div>
									<div>
										<div class="text-label-caps font-label-caps uppercase tracking-wide text-fg-muted dark:text-dark-fg-muted">Worker</div>
										<div class="mt-1 font-mono-code text-mono-code text-fg-ink dark:text-dark-fg-ink">
											{#if s.workerId}
												<a
													href={workerHref(s.workerId)}
													onclick={(event) => event.stopPropagation()}
													class="text-accent-lever hover:underline dark:text-dark-accent-lever"
												>
													{s.workerId}
												</a>
											{:else}
												—
											{/if}
										</div>
									</div>
									<div>
										<div class="text-label-caps font-label-caps uppercase tracking-wide text-fg-muted dark:text-dark-fg-muted">Effort</div>
										<div class="mt-1 text-body-sm text-fg-ink dark:text-dark-fg-ink">{s.effort}</div>
									</div>
									<div>
										<div class="text-label-caps font-label-caps uppercase tracking-wide text-fg-muted dark:text-dark-fg-muted">Outcome</div>
										<div class="mt-1 text-body-sm text-fg-ink dark:text-dark-fg-ink">{s.outcome ?? '—'}</div>
									</div>
									<div>
										<div class="text-label-caps font-label-caps uppercase tracking-wide text-fg-muted dark:text-dark-fg-muted">Execution</div>
										<div class="mt-1 font-mono-code text-mono-code text-fg-ink dark:text-dark-fg-ink">
											{#if sessionExecutions[s.id]}
												<a
													href={executionHref(sessionExecutions[s.id] as string)}
													onclick={(event) => event.stopPropagation()}
													class="text-accent-lever hover:underline dark:text-dark-accent-lever"
												>
													{(sessionExecutions[s.id] as string).slice(0, 18)}…
												</a>
											{:else}
												—
											{/if}
										</div>
									</div>
									<div>
										<div class="text-label-caps font-label-caps uppercase tracking-wide text-fg-muted dark:text-dark-fg-muted">Ended</div>
										<div class="mt-1 text-body-sm text-fg-ink dark:text-dark-fg-ink">
											{s.endedAt ? fmtDate(s.endedAt) : '—'}
										</div>
									</div>
									{#if s.tokens}
										<div>
											<div class="text-label-caps font-label-caps uppercase tracking-wide text-fg-muted dark:text-dark-fg-muted">
												Prompt tokens
											</div>
											<div class="mt-1 font-mono-code text-mono-code text-fg-ink dark:text-dark-fg-ink">
												{s.tokens.prompt?.toLocaleString() ?? '—'}
											</div>
										</div>
										<div>
											<div class="text-label-caps font-label-caps uppercase tracking-wide text-fg-muted dark:text-dark-fg-muted">
												Completion tokens
											</div>
											<div class="mt-1 font-mono-code text-mono-code text-fg-ink dark:text-dark-fg-ink">
												{s.tokens.completion?.toLocaleString() ?? '—'}
											</div>
										</div>
										<div>
											<div class="text-label-caps font-label-caps uppercase tracking-wide text-fg-muted dark:text-dark-fg-muted">
												Cached tokens
											</div>
											<div class="mt-1 font-mono-code text-mono-code text-fg-ink dark:text-dark-fg-ink">
												{s.tokens.cached?.toLocaleString() ?? '—'}
											</div>
										</div>
										<div>
											<div class="text-label-caps font-label-caps uppercase tracking-wide text-fg-muted dark:text-dark-fg-muted">
												Total tokens
											</div>
											<div class="mt-1 font-mono-code text-mono-code text-fg-ink dark:text-dark-fg-ink">
												{s.tokens.total?.toLocaleString() ?? '—'}
											</div>
										</div>
									{/if}
									{#if s.detail}
										<div class="col-span-2 sm:col-span-4">
											<div class="text-label-caps font-label-caps uppercase tracking-wide text-fg-muted dark:text-dark-fg-muted">Detail</div>
											<div class="mt-1 text-body-sm text-fg-ink dark:text-dark-fg-ink">{s.detail}</div>
										</div>
									{/if}
									{#if bodies?.prompt}
										<div class="col-span-2 sm:col-span-4">
											<div class="text-label-caps font-label-caps uppercase tracking-wide text-fg-muted dark:text-dark-fg-muted">Prompt</div>
											<pre
												class="mt-1 max-h-56 overflow-auto border border-border-line bg-terminal-bg p-3 font-mono-code text-mono-code whitespace-pre-wrap text-terminal-fg dark:border-dark-border-line">{bodies.prompt}</pre>
										</div>
									{/if}
									{#if bodies?.response}
										<div class="col-span-2 sm:col-span-4">
											<div class="text-label-caps font-label-caps uppercase tracking-wide text-fg-muted dark:text-dark-fg-muted">
												Response
											</div>
											<pre
												class="mt-1 max-h-56 overflow-auto border border-border-line bg-terminal-bg p-3 font-mono-code text-mono-code whitespace-pre-wrap text-terminal-fg dark:border-dark-border-line">{bodies.response}</pre>
										</div>
									{/if}
									{#if bodies?.stderr}
										<div class="col-span-2 sm:col-span-4">
											<div class="text-label-caps font-label-caps uppercase tracking-wide text-fg-muted dark:text-dark-fg-muted">Stderr</div>
											<pre
												class="mt-1 max-h-56 overflow-auto border border-border-line bg-terminal-bg p-3 font-mono-code text-mono-code whitespace-pre-wrap text-terminal-fg dark:border-dark-border-line">{bodies.stderr}</pre>
										</div>
									{/if}
								</div>
							</td>
						</tr>
					{/if}
				{/each}
				{#if data.sessions.edges.length === 0}
					<tr>
						<td colspan="8" class="px-4 py-8 text-center text-body-sm text-fg-muted dark:text-dark-fg-muted">
							No sessions found for this project.
						</td>
					</tr>
				{/if}
			</tbody>
		</table>
	</div>
</div>
