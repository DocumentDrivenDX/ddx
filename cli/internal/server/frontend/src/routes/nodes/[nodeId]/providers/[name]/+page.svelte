<script lang="ts">
	import { page } from '$app/stores';
	import { onMount } from 'svelte';
	import { createClient } from '$lib/gql/client';
	import { gql } from 'graphql-request';

	const TREND_QUERY = gql`
		query ProviderTrend($name: String!, $windowDays: Int!) {
			providerTrend(name: $name, windowDays: $windowDays) {
				name
				kind
				windowDays
				ceilingTokens
				projectedRunOutHours
				series {
					bucketStart
					tokens
					requests
				}
			}
		}
	`;

	const RECENT_USAGE_QUERY = gql`
		query ProviderRecentUsage($provider: String!, $first: Int!) {
			agentSessions(provider: $provider, first: $first) {
				edges {
					node {
						id
						startedAt
						durationMs
						harness
						provider
						model
						effort
						status
						detail
					}
				}
			}
		}
	`;

	interface TrendPoint {
		bucketStart: string;
		tokens: number;
		requests: number;
	}

	interface ProviderTrend {
		name: string;
		kind: 'ENDPOINT' | 'HARNESS';
		windowDays: number;
		ceilingTokens: number | null;
		projectedRunOutHours: number | null;
		series: TrendPoint[];
	}

	interface AgentSessionRow {
		id: string;
		startedAt: string;
		durationMs: number;
		harness: string;
		provider: string | null;
		model: string;
		effort: string;
		status: string;
		detail: string | null;
	}

	let name = $derived($page.params.name);
	let trend7 = $state<ProviderTrend | null>(null);
	let trend30 = $state<ProviderTrend | null>(null);
	let recentUsage = $state<AgentSessionRow[]>([]);
	let loading = $state(true);
	let error = $state<string | null>(null);

	onMount(async () => {
		const client = createClient();
		try {
			const [trend7Result, trend30Result, usageResult] = await Promise.all([
				client.request<{ providerTrend: ProviderTrend | null }>(TREND_QUERY, {
					name: name,
					windowDays: 7
				}),
				client.request<{ providerTrend: ProviderTrend | null }>(TREND_QUERY, {
					name: name,
					windowDays: 30
				}),
				client.request<{
					agentSessions: { edges: Array<{ node: AgentSessionRow }> };
				}>(RECENT_USAGE_QUERY, {
					provider: name,
					first: 12
				})
			]);
			trend7 = trend7Result.providerTrend ?? null;
			trend30 = trend30Result.providerTrend ?? null;
			recentUsage = usageResult.agentSessions.edges.map((edge) => edge.node);
		} catch (e) {
			error = e instanceof Error ? e.message : String(e);
		} finally {
			loading = false;
		}
	});

	function maxTokens(series: TrendPoint[], ceiling: number | null): number {
		let m = 0;
		for (const p of series) {
			if (p.tokens > m) m = p.tokens;
		}
		if (ceiling != null && ceiling > m) m = ceiling;
		return m === 0 ? 1 : m;
	}

	function barHeight(tokens: number, max: number): string {
		const pct = Math.round((tokens * 100) / max);
		return `${Math.max(2, pct)}%`;
	}

	function formatHours(hours: number | null): string {
		if (hours == null) return '—';
		if (hours < 1) return `${Math.round(hours * 60)}m`;
		if (hours < 48) return `${hours.toFixed(1)}h`;
		return `${(hours / 24).toFixed(1)}d`;
	}

	function totalTokens(series: TrendPoint[]): number {
		let sum = 0;
		for (const p of series) sum += p.tokens;
		return sum;
	}

	function totalRequests(series: TrendPoint[]): number {
		let sum = 0;
		for (const p of series) sum += p.requests;
		return sum;
	}

	function formatN(n: number): string {
		if (n < 1000) return `${n}`;
		if (n < 1_000_000) return `${(n / 1000).toFixed(1)}k`;
		return `${(n / 1_000_000).toFixed(2)}M`;
	}

	function formatStartedAt(value: string): string {
		const date = new Date(value);
		if (Number.isNaN(date.getTime())) return value;
		return date.toLocaleString([], {
			month: 'short',
			day: 'numeric',
			hour: '2-digit',
			minute: '2-digit'
		});
	}

	function formatDuration(ms: number): string {
		if (ms < 1000) return `${ms}ms`;
		if (ms < 60_000) return `${(ms / 1000).toFixed(1)}s`;
		const minutes = Math.floor(ms / 60_000);
		const seconds = Math.floor((ms % 60_000) / 1000);
		return `${minutes}m ${seconds}s`;
	}
</script>

<svelte:head>
	<title>{name} · trend · DDx</title>
</svelte:head>

<div class="space-y-6" data-testid="provider-trend">
	<div class="flex items-center justify-between">
		<div>
			<a href="/nodes/{$page.params.nodeId}/providers" class="text-body-sm text-accent-lever hover:underline dark:text-dark-accent-lever">
				← Agent endpoints
			</a>
			<h1 class="mt-1 text-headline-md font-headline-md text-fg-ink dark:text-dark-fg-ink">{name}</h1>
			{#if trend7}
				<p class="text-body-sm text-fg-muted dark:text-dark-fg-muted">
					{trend7.kind === 'HARNESS' ? 'Subprocess harness' : 'API endpoint'}
				</p>
			{/if}
		</div>
	</div>

	{#if loading}
		<div class="py-8 text-center text-body-sm text-fg-muted dark:text-dark-fg-muted">Loading trend…</div>
	{:else if error}
		<div class="border border-border-line bg-bg-surface p-4 text-body-sm text-error dark:border-dark-border-line dark:bg-dark-bg-surface dark:text-dark-error">
			Error: {error}
		</div>
	{:else if trend7 && trend7.projectedRunOutHours != null}
		<div class="alert-caution border p-4 text-body-sm" data-testid="projection-callout">
			Projected to hit quota in ~{formatHours(trend7.projectedRunOutHours)} at current rate.
		</div>
	{/if}

	{#if trend7}
		{@const max7 = maxTokens(trend7.series, trend7.ceilingTokens)}
		<section class="border border-border-line p-4 dark:border-dark-border-line" data-testid="series-7d">
			<div class="mb-2 flex items-center justify-between">
				<h2 class="text-body-sm font-semibold text-fg-ink dark:text-dark-fg-ink">Last 7 days · hourly buckets</h2>
				<span class="text-label-caps font-label-caps text-fg-muted dark:text-dark-fg-muted">
					{formatN(totalTokens(trend7.series))} tokens · {formatN(totalRequests(trend7.series))} requests
				</span>
			</div>
			<div class="relative h-24" role="img" aria-label="7-day tokens-per-hour series">
				<div class="flex h-full items-end gap-[1px]">
					{#each trend7.series as point (point.bucketStart)}
						<div
							class="w-full bg-accent-lever dark:bg-dark-accent-lever"
							style="height: {barHeight(point.tokens, max7)}"
							title="{point.bucketStart}: {point.tokens} tokens, {point.requests} requests"></div>
					{/each}
				</div>
				{#if trend7.ceilingTokens != null && trend7.ceilingTokens > 0}
					<div
						class="pointer-events-none absolute right-0 left-0 border-t-2 border-dashed border-red-500/70"
						style="bottom: {Math.min(100, Math.round((trend7.ceilingTokens * 100) / max7))}%"
						data-testid="ceiling-overlay-7d"
						title="Quota ceiling: {formatN(trend7.ceilingTokens)} tokens"
					>
						<span
							class="absolute -top-4 right-0 bg-red-500/80 px-1 text-[10px] font-medium text-white"
						>
							ceiling {formatN(trend7.ceilingTokens)}
						</span>
					</div>
				{/if}
			</div>
		</section>
	{/if}

	{#if trend30}
		{@const max30 = maxTokens(trend30.series, trend30.ceilingTokens)}
		<section class="border border-border-line p-4 dark:border-dark-border-line" data-testid="series-30d">
			<div class="mb-2 flex items-center justify-between">
				<h2 class="text-body-sm font-semibold text-fg-ink dark:text-dark-fg-ink">Last 30 days · 4-hour buckets</h2>
				<span class="text-label-caps font-label-caps text-fg-muted dark:text-dark-fg-muted">
					{formatN(totalTokens(trend30.series))} tokens · {formatN(totalRequests(trend30.series))} requests
				</span>
			</div>
			<div class="relative h-24" role="img" aria-label="30-day tokens-per-4h series">
				<div class="flex h-full items-end gap-[1px]">
					{#each trend30.series as point (point.bucketStart)}
						<div
							class="w-full bg-accent-fulcrum dark:bg-dark-accent-fulcrum"
							style="height: {barHeight(point.tokens, max30)}"
							title="{point.bucketStart}: {point.tokens} tokens, {point.requests} requests"></div>
					{/each}
				</div>
				{#if trend30.ceilingTokens != null && trend30.ceilingTokens > 0}
					<div
						class="pointer-events-none absolute right-0 left-0 border-t-2 border-dashed border-red-500/70"
						style="bottom: {Math.min(100, Math.round((trend30.ceilingTokens * 100) / max30))}%"
						data-testid="ceiling-overlay-30d"
						title="Quota ceiling: {formatN(trend30.ceilingTokens)} tokens"
					></div>
				{/if}
			</div>
		</section>
	{/if}

	<section class="border border-border-line p-4 dark:border-dark-border-line" data-testid="recent-usage">
		<div class="mb-2 flex items-center justify-between">
			<h2 class="text-body-sm font-semibold text-fg-ink dark:text-dark-fg-ink">Recent usage</h2>
			<span class="text-label-caps font-label-caps text-fg-muted dark:text-dark-fg-muted">
				{recentUsage.length} rows
			</span>
		</div>
		{#if loading}
			<div class="py-4 text-body-sm text-fg-muted dark:text-dark-fg-muted">Loading usage…</div>
		{:else if error}
			<div class="py-4 text-body-sm text-error dark:text-dark-error">Error: {error}</div>
		{:else if recentUsage.length === 0}
			<div class="py-4 text-body-sm text-fg-muted dark:text-dark-fg-muted">No recent usage rows.</div>
		{:else}
			<div class="overflow-x-auto">
				<table class="min-w-full border-collapse text-left text-body-sm">
					<thead>
						<tr class="border-b border-border-line dark:border-dark-border-line">
							<th class="py-2 pr-3 font-semibold">Started</th>
							<th class="py-2 pr-3 font-semibold">Harness</th>
							<th class="py-2 pr-3 font-semibold">Model</th>
							<th class="py-2 pr-3 font-semibold">Status</th>
							<th class="py-2 pr-3 font-semibold">Duration</th>
							<th class="py-2 pr-3 font-semibold">Detail</th>
						</tr>
					</thead>
					<tbody>
						{#each recentUsage as row (row.id)}
							<tr
								class="border-b border-border-line/60 dark:border-dark-border-line/60"
								data-testid="recent-usage-row-{row.id}"
							>
								<td class="py-2 pr-3 whitespace-nowrap">{formatStartedAt(row.startedAt)}</td>
								<td class="py-2 pr-3">{row.harness}</td>
								<td class="py-2 pr-3">{row.model}</td>
								<td class="py-2 pr-3 capitalize">{row.status}</td>
								<td class="py-2 pr-3 whitespace-nowrap">{formatDuration(row.durationMs)}</td>
								<td class="py-2 pr-3 text-fg-muted dark:text-dark-fg-muted">{row.detail ?? '—'}</td>
							</tr>
						{/each}
					</tbody>
				</table>
			</div>
		{/if}
	</section>
</div>
