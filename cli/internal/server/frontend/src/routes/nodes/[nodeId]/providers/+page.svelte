<script lang="ts">
	import { page } from '$app/stores';
	import { onMount } from 'svelte';
	import { createClient } from '$lib/gql/client';
	import { gql } from 'graphql-request';

	const PROVIDER_STATUSES_QUERY = gql`
		query ProviderStatuses {
			providerStatuses {
				name
				kind
				providerType
				baseURL
				model
				status
				reachable
				detail
				modelCount
				isDefault
				cooldownUntil
				lastCheckedAt
				defaultForProfile
				usage {
					tokensUsedLastHour
					tokensUsedLast24h
					requestsLastHour
					requestsLast24h
				}
				quota {
					ceilingTokens
					ceilingWindowSeconds
					remaining
					resetAt
				}
				sparkline
			}
			harnessStatuses {
				name
				kind
				providerType
				baseURL
				model
				status
				reachable
				detail
				modelCount
				isDefault
				cooldownUntil
				lastCheckedAt
				defaultForProfile
				usage {
					tokensUsedLastHour
					tokensUsedLast24h
					requestsLastHour
					requestsLast24h
				}
				quota {
					ceilingTokens
					ceilingWindowSeconds
					remaining
					resetAt
				}
				sparkline
			}
		}
	`;

	const DEFAULT_ROUTE_QUERY = gql`
		query DefaultRouteStatus {
			defaultRouteStatus {
				modelRef
				resolvedProvider
				resolvedModel
				strategy
			}
		}
	`;

	interface ProviderUsage {
		tokensUsedLastHour: number | null;
		tokensUsedLast24h: number | null;
		requestsLastHour: number | null;
		requestsLast24h: number | null;
	}

	interface ProviderQuota {
		ceilingTokens: number | null;
		ceilingWindowSeconds: number | null;
		remaining: number | null;
		resetAt: string | null;
	}

	interface ProviderStatus {
		name: string;
		kind: 'ENDPOINT' | 'HARNESS';
		providerType: string;
		baseURL: string;
		model: string;
		status: string;
		reachable: boolean;
		detail: string;
		modelCount: number;
		isDefault: boolean;
		cooldownUntil: string | null;
		lastCheckedAt: string | null;
		defaultForProfile: string[];
		usage: ProviderUsage | null;
		quota: ProviderQuota | null;
		sparkline: number[];
	}

	interface DefaultRouteStatus {
		modelRef: string;
		resolvedProvider: string | null;
		resolvedModel: string | null;
		strategy: string | null;
	}

	// First-paint state: we render the table from the query result as soon as
	// it lands. The query itself returns cached probe results — a live refresh
	// action will enqueue fresh probes in a future iteration.
	let rows = $state<ProviderStatus[]>([]);
	let defaultRoute = $state<DefaultRouteStatus | null>(null);
	let loading = $state(true);
	let error = $state<string | null>(null);
	let firstPaintAt = $state<number | null>(null);

	$effect(() => {
		if (!loading && firstPaintAt === null) {
			firstPaintAt = Date.now();
		}
	});

	// Polling interval for live row patching after first paint. Chosen to be
	// short enough that probe results surface within the 5s AC budget but long
	// enough to not hammer the server. AC 1: rows patch within 5s of probe
	// completion — since the resolver returns cached rows updated asynchronously
	// by refreshProviderStatuses, polling here is sufficient to reflect them.
	const POLL_INTERVAL_MS = 2500;
	let pollTimer: ReturnType<typeof setInterval> | null = null;

	async function refresh(client: ReturnType<typeof createClient>) {
		const result = await client.request<{
			providerStatuses: ProviderStatus[];
			harnessStatuses: ProviderStatus[];
		}>(PROVIDER_STATUSES_QUERY);
		rows = [...(result.providerStatuses ?? []), ...(result.harnessStatuses ?? [])];
	}

	onMount(() => {
		const client = createClient();
		// First paint: provider + harness rows. defaultRouteStatus fires in
		// parallel via a separate query so a slow route-resolver can't delay
		// the table from rendering. AC 1: table interactive within 500ms.
		refresh(client)
			.catch((e) => {
				error = e instanceof Error ? e.message : String(e);
			})
			.finally(() => {
				loading = false;
			});

		client
			.request<{ defaultRouteStatus: DefaultRouteStatus | null }>(DEFAULT_ROUTE_QUERY)
			.then((res) => {
				defaultRoute = res.defaultRouteStatus ?? null;
			})
			.catch(() => {
				// Non-fatal: the default-route widget is informational only.
			});

		pollTimer = setInterval(() => {
			refresh(client).catch(() => {
				// Polling errors are transient; keep the last-known rows.
			});
		}, POLL_INTERVAL_MS);

		return () => {
			if (pollTimer != null) {
				clearInterval(pollTimer);
				pollTimer = null;
			}
		};
	});

	function statusClass(row: ProviderStatus): string {
		if (row.reachable) {
			return 'text-green-600 dark:text-green-400';
		}
		const status = row.status;
		const lower = status.toLowerCase();
		if (
			lower.includes('connected') ||
			lower === 'available' ||
			lower.includes('api key configured')
		) {
			return 'text-green-600 dark:text-green-400';
		}
		if (
			lower.includes('cooldown') ||
			lower.includes('unreachable') ||
			lower.includes('error') ||
			lower === 'unavailable' ||
			lower.startsWith('unavailable')
		) {
			return 'text-red-600 dark:text-red-400';
		}
		return 'text-yellow-600 dark:text-yellow-400';
	}

	function formatTokens(n: number | null | undefined): string {
		if (n == null) return '—';
		if (n < 1000) return `${n}`;
		if (n < 1_000_000) return `${(n / 1000).toFixed(1)}k`;
		return `${(n / 1_000_000).toFixed(2)}M`;
	}

	function utilizationPct(usage: ProviderUsage | null, quota: ProviderQuota | null): number | null {
		if (!usage || !quota) return null;
		if (quota.ceilingTokens == null || quota.ceilingTokens <= 0) return null;
		const window = quota.ceilingWindowSeconds ?? 60;
		// Choose the usage field that matches the ceiling window (1h / 24h).
		const usedTokens = window <= 3600 ? usage.tokensUsedLastHour : usage.tokensUsedLast24h;
		if (usedTokens == null) return null;
		return Math.min(100, Math.round((usedTokens * 100) / quota.ceilingTokens));
	}

	function sparklineMax(values: number[]): number {
		let m = 0;
		for (const v of values) if (v > m) m = v;
		return m === 0 ? 1 : m;
	}

	function sparkBarHeight(value: number, max: number): string {
		const pct = Math.round((value * 100) / max);
		return `${Math.max(2, pct)}%`;
	}

	function detailHref(row: ProviderStatus): string {
		const nodeId = $page.params.nodeId;
		return `/nodes/${nodeId}/providers/${encodeURIComponent(row.name)}`;
	}
</script>

<svelte:head>
	<title>Agent endpoints · DDx</title>
</svelte:head>

<div class="space-y-6" data-testid="agent-endpoints">
	<div class="flex items-center justify-between">
		<h1 class="text-xl font-semibold dark:text-white">Agent endpoints</h1>
		{#if !loading}
			<span class="text-sm text-gray-500 dark:text-gray-400">
				{rows.length} total ({rows.filter((r) => r.kind === 'ENDPOINT').length} endpoints · {rows.filter(
					(r) => r.kind === 'HARNESS'
				).length} harnesses)
			</span>
		{/if}
	</div>

	<!-- Default route widget -->
	{#if defaultRoute && defaultRoute.modelRef}
		<div
			class="rounded-lg border border-gray-200 bg-gray-50 p-4 dark:border-gray-700 dark:bg-gray-800/50"
		>
			<h2 class="mb-2 text-sm font-medium text-gray-700 dark:text-gray-300">
				Current route for default profile
			</h2>
			<div class="flex flex-wrap gap-4 text-sm">
				<span class="text-gray-500 dark:text-gray-400">
					Model ref: <span class="font-mono font-medium text-gray-900 dark:text-white"
						>{defaultRoute.modelRef}</span
					>
				</span>
				{#if defaultRoute.strategy}
					<span class="text-gray-500 dark:text-gray-400">
						Strategy: <span class="font-medium text-gray-700 dark:text-gray-300"
							>{defaultRoute.strategy}</span
						>
					</span>
				{/if}
				{#if defaultRoute.resolvedProvider}
					<span class="text-gray-500 dark:text-gray-400">
						Resolves to:
						<span class="font-medium text-green-700 dark:text-green-400">
							{defaultRoute.resolvedProvider}
						</span>
						{#if defaultRoute.resolvedModel}
							/
							<span class="font-mono text-gray-700 dark:text-gray-300"
								>{defaultRoute.resolvedModel}</span
							>
						{/if}
					</span>
				{:else}
					<span class="font-medium text-red-600 dark:text-red-400">
						No healthy candidate available
					</span>
				{/if}
			</div>
		</div>
	{/if}

	<!-- Unified table -->
	{#if loading}
		<div class="py-8 text-center text-sm text-gray-400 dark:text-gray-600" data-testid="loading">
			Loading agent endpoints…
		</div>
	{:else if error}
		<div
			class="rounded-lg border border-red-200 bg-red-50 p-4 text-sm text-red-700 dark:border-red-800 dark:bg-red-900/20 dark:text-red-400"
		>
			Error: {error}
		</div>
	{:else}
		<div class="overflow-hidden rounded-lg border border-gray-200 dark:border-gray-700">
			<table class="w-full text-sm" data-testid="agent-endpoints-table">
				<thead>
					<tr class="border-b border-gray-200 bg-gray-50 dark:border-gray-700 dark:bg-gray-800">
						<th class="px-4 py-3 text-left font-medium text-gray-600 dark:text-gray-300">Name</th>
						<th class="px-4 py-3 text-left font-medium text-gray-600 dark:text-gray-300">Kind</th>
						<th class="px-4 py-3 text-left font-medium text-gray-600 dark:text-gray-300">Type</th>
						<th class="px-4 py-3 text-left font-medium text-gray-600 dark:text-gray-300">Model</th>
						<th class="px-4 py-3 text-left font-medium text-gray-600 dark:text-gray-300">Status</th>
						<th class="px-4 py-3 text-left font-medium text-gray-600 dark:text-gray-300"
							>Tokens (1h / 24h)</th
						>
						<th class="px-4 py-3 text-left font-medium text-gray-600 dark:text-gray-300"
							>Utilization</th
						>
						<th class="px-4 py-3 text-left font-medium text-gray-600 dark:text-gray-300"
							>Trend (24h)</th
						>
					</tr>
				</thead>
				<tbody>
					{#each rows as row (row.kind + '|' + row.name)}
						<tr
							class="border-b border-gray-100 last:border-0 dark:border-gray-700"
							data-testid="endpoint-row-{row.name}"
						>
							<td class="px-4 py-3 font-medium text-gray-900 dark:text-gray-100">
								<a
									class="text-blue-600 hover:underline dark:text-blue-400"
									href={detailHref(row)}
									data-testid="endpoint-link-{row.name}">{row.name}</a
								>
								{#if row.isDefault}
									<span
										class="ml-1 inline-flex items-center rounded-full bg-blue-100 px-1.5 py-0.5 text-xs font-medium text-blue-700 dark:bg-blue-900/30 dark:text-blue-300"
									>
										default
									</span>
								{/if}
								{#if row.cooldownUntil}
									<span
										class="ml-1 inline-flex items-center rounded-full bg-red-100 px-1.5 py-0.5 text-xs font-medium text-red-700 dark:bg-red-900/30 dark:text-red-300"
										title="Cooldown until {row.cooldownUntil}"
									>
										cooldown
									</span>
								{/if}
							</td>
							<td
								class="px-4 py-3 text-xs text-gray-500 uppercase dark:text-gray-400"
								data-testid="endpoint-kind-{row.name}"
							>
								{row.kind === 'ENDPOINT' ? 'endpoint' : 'harness'}
							</td>
							<td class="px-4 py-3 text-gray-600 dark:text-gray-400">
								{row.providerType}
							</td>
							<td
								class="max-w-xs truncate px-4 py-3 font-mono text-xs text-gray-700 dark:text-gray-300"
								title={row.model}
							>
								{row.model || '—'}
							</td>
							<td class="px-4 py-3">
								<span
									class="font-medium {statusClass(row)}"
									data-testid="endpoint-reachable-{row.name}"
								>
									{row.reachable ? 'reachable' : 'not reachable'}
								</span>
								<span class="ml-1 text-gray-500 dark:text-gray-400" title={row.detail}>
									{row.status}
								</span>
								{#if row.lastCheckedAt}
									<span class="ml-1 text-xs text-gray-400" title="Last checked {row.lastCheckedAt}"
										>·</span
									>
								{/if}
							</td>
							<td
								class="px-4 py-3 text-gray-600 tabular-nums dark:text-gray-300"
								data-testid="endpoint-tokens-{row.name}"
							>
								{#if row.usage}
									{formatTokens(row.usage.tokensUsedLastHour)} / {formatTokens(
										row.usage.tokensUsedLast24h
									)}
								{:else}
									<span class="text-gray-400">not reported</span>
								{/if}
							</td>
							<td class="px-4 py-3">
								{#if utilizationPct(row.usage, row.quota) != null}
									<div class="flex items-center gap-2">
										<div class="h-2 w-20 overflow-hidden rounded-full bg-gray-200 dark:bg-gray-700">
											<div
												class="h-full bg-blue-500"
												style="width: {utilizationPct(row.usage, row.quota)}%"
											></div>
										</div>
										<span class="text-xs text-gray-500 tabular-nums dark:text-gray-400">
											{utilizationPct(row.usage, row.quota)}%
										</span>
									</div>
								{:else}
									<span class="text-xs text-gray-400">not reported</span>
								{/if}
							</td>
							<td class="px-4 py-3" data-testid="endpoint-sparkline-{row.name}">
								{#if row.sparkline && row.sparkline.length >= 6}
									{@const max = sparklineMax(row.sparkline)}
									<div
										class="flex h-6 w-24 items-end gap-[1px]"
										role="img"
										aria-label="24-hour token trend for {row.name}"
										data-testid="endpoint-sparkline-bars-{row.name}"
									>
										{#each row.sparkline as v, i (i)}
											<div
												class="w-full bg-blue-400 dark:bg-blue-500"
												style="height: {sparkBarHeight(v, max)}"
												title="{v} tokens"
											></div>
										{/each}
									</div>
								{:else}
									<span class="text-xs text-gray-400">—</span>
								{/if}
							</td>
						</tr>
					{/each}
					{#if rows.length === 0}
						<tr>
							<td colspan="8" class="px-4 py-8 text-center text-gray-400 dark:text-gray-600">
								No agent endpoints configured. Add providers to .ddx/config.yaml or install a
								harness binary.
							</td>
						</tr>
					{/if}
				</tbody>
			</table>
		</div>
	{/if}
</div>
