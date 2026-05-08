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

	let name = $derived($page.params.name);
	let trend7 = $state<ProviderTrend | null>(null);
	let trend30 = $state<ProviderTrend | null>(null);
	let loading = $state(true);
	let error = $state<string | null>(null);

	onMount(async () => {
		try {
			const client = createClient();
			const [r7, r30] = await Promise.all([
				client.request<{ providerTrend: ProviderTrend | null }>(TREND_QUERY, {
					name: name,
					windowDays: 7
				}),
				client.request<{ providerTrend: ProviderTrend | null }>(TREND_QUERY, {
					name: name,
					windowDays: 30
				})
			]);
			trend7 = r7.providerTrend ?? null;
			trend30 = r30.providerTrend ?? null;
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
</script>

<svelte:head>
	<title>{name} · trend · DDx</title>
</svelte:head>

<div class="space-y-6" data-testid="provider-trend">
	<div class="flex items-center justify-between">
		<div>
			<a href="/nodes/{$page.params.nodeId}/providers" class="text-sm text-blue-600 hover:underline dark:text-blue-400">
				← Agent endpoints
			</a>
			<h1 class="mt-1 text-xl font-semibold dark:text-white">{name}</h1>
			{#if trend7}
				<p class="text-sm text-gray-500 dark:text-gray-400">
					{trend7.kind === 'HARNESS' ? 'Subprocess harness' : 'API endpoint'}
				</p>
			{/if}
		</div>
	</div>

	{#if loading}
		<div class="py-8 text-center text-sm text-gray-400 dark:text-gray-600">Loading trend…</div>
	{:else if error}
		<div class="rounded-lg border border-red-200 bg-red-50 p-4 text-sm text-red-700 dark:border-red-800 dark:bg-red-900/20 dark:text-red-400">
			Error: {error}
		</div>
	{:else if trend7 && trend7.projectedRunOutHours != null}
		<div class="rounded-lg border border-orange-200 bg-orange-50 p-4 text-sm text-orange-800 dark:border-orange-800 dark:bg-orange-900/20 dark:text-orange-300" data-testid="projection-callout">
			Projected to hit quota in ~{formatHours(trend7.projectedRunOutHours)} at current rate.
		</div>
	{/if}

	{#if trend7}
		<section class="rounded-lg border border-gray-200 p-4 dark:border-gray-700" data-testid="series-7d">
			<div class="mb-2 flex items-center justify-between">
				<h2 class="text-sm font-semibold dark:text-white">Last 7 days · hourly buckets</h2>
				<span class="text-xs text-gray-500 dark:text-gray-400">
					{formatN(totalTokens(trend7.series))} tokens · {formatN(totalRequests(trend7.series))} requests
				</span>
			</div>
			<div class="flex items-end gap-[1px] h-24" role="img" aria-label="7-day tokens-per-hour series">
				{#each trend7.series as point (point.bucketStart)}
					<div
						class="w-full bg-blue-500"
						style="height: {barHeight(point.tokens, maxTokens(trend7.series, trend7.ceilingTokens))}"
						title="{point.bucketStart}: {point.tokens} tokens, {point.requests} requests"></div>
				{/each}
			</div>
			{#if trend7.ceilingTokens != null}
				<p class="mt-2 text-xs text-gray-500 dark:text-gray-400">
					Ceiling: {formatN(trend7.ceilingTokens)} tokens/window
				</p>
			{/if}
		</section>
	{/if}

	{#if trend30}
		<section class="rounded-lg border border-gray-200 p-4 dark:border-gray-700" data-testid="series-30d">
			<div class="mb-2 flex items-center justify-between">
				<h2 class="text-sm font-semibold dark:text-white">Last 30 days · 4-hour buckets</h2>
				<span class="text-xs text-gray-500 dark:text-gray-400">
					{formatN(totalTokens(trend30.series))} tokens · {formatN(totalRequests(trend30.series))} requests
				</span>
			</div>
			<div class="flex items-end gap-[1px] h-24" role="img" aria-label="30-day tokens-per-4h series">
				{#each trend30.series as point (point.bucketStart)}
					<div
						class="w-full bg-indigo-500"
						style="height: {barHeight(point.tokens, maxTokens(trend30.series, trend30.ceilingTokens))}"
						title="{point.bucketStart}: {point.tokens} tokens, {point.requests} requests"></div>
				{/each}
			</div>
		</section>
	{/if}
</div>
