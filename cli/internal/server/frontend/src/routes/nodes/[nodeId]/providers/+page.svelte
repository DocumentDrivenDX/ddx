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

	const PROVIDER_MODELS_QUERY = gql`
		query ProviderModels($name: String!, $kind: ProviderKind!) {
			providerModels(name: $name, kind: $kind) {
				name
				kind
				baseURL
				fetchedAt
				fromCache
				models {
					id
					contextLength
					available
				}
			}
		}
	`;

	const REFRESH_PROVIDER_MODELS_MUTATION = gql`
		mutation RefreshProviderModels($name: String!, $kind: ProviderKind!) {
			refreshProviderModels(name: $name, kind: $kind) {
				name
				kind
				baseURL
				fetchedAt
				fromCache
				models {
					id
					contextLength
					available
				}
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

	interface ProviderModelEntry {
		id: string;
		contextLength: number | null;
		available: boolean;
	}

	interface ProviderModelsResult {
		name: string;
		kind: 'ENDPOINT' | 'HARNESS';
		baseURL: string;
		fetchedAt: string;
		fromCache: boolean;
		models: ProviderModelEntry[];
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

	// Per-row model snapshots loaded on demand when a row is expanded.
	const modelsByKey = $state<Record<string, ProviderModelsResult>>({});
	const modelsLoading = $state<Record<string, boolean>>({});
	const modelsError = $state<Record<string, string>>({});
	const expanded = $state<Record<string, boolean>>({});

	// Threshold above which the inline list is truncated and a drilldown
	// link to the per-provider detail page is shown.
	const MODEL_LIST_INLINE_LIMIT = 10;

	function rowKey(row: ProviderStatus): string {
		return row.kind + '|' + row.name;
	}

	let modelsClient: ReturnType<typeof createClient> | null = null;

	async function loadModels(row: ProviderStatus): Promise<void> {
		const key = rowKey(row);
		if (!modelsClient) modelsClient = createClient();
		modelsLoading[key] = true;
		delete modelsError[key];
		try {
			const res = await modelsClient.request<{
				providerModels: ProviderModelsResult | null;
			}>(PROVIDER_MODELS_QUERY, { name: row.name, kind: row.kind });
			if (res.providerModels) modelsByKey[key] = res.providerModels;
		} catch (e) {
			modelsError[key] = e instanceof Error ? e.message : String(e);
		} finally {
			modelsLoading[key] = false;
		}
	}

	async function refreshModels(row: ProviderStatus): Promise<void> {
		const key = rowKey(row);
		if (!modelsClient) modelsClient = createClient();
		modelsLoading[key] = true;
		delete modelsError[key];
		try {
			const res = await modelsClient.request<{
				refreshProviderModels: ProviderModelsResult;
			}>(REFRESH_PROVIDER_MODELS_MUTATION, { name: row.name, kind: row.kind });
			modelsByKey[key] = res.refreshProviderModels;
			expanded[key] = true;
		} catch (e) {
			modelsError[key] = e instanceof Error ? e.message : String(e);
		} finally {
			modelsLoading[key] = false;
		}
	}

	function toggleExpanded(row: ProviderStatus): void {
		const key = rowKey(row);
		const next = !expanded[key];
		expanded[key] = next;
		if (next && !modelsByKey[key] && !modelsLoading[key]) {
			void loadModels(row);
		}
	}

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
		if (row.reachable) return 'text-status-closed';
		const lower = row.status.toLowerCase();
		if (
			lower.includes('connected') ||
			lower === 'available' ||
			lower.includes('api key configured')
		) {
			return 'text-status-closed';
		}
		if (
			lower.includes('cooldown') ||
			lower.includes('unreachable') ||
			lower.includes('error') ||
			lower === 'unavailable' ||
			lower.startsWith('unavailable')
		) {
			return 'text-status-failed';
		}
		return 'text-status-in-progress';
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
	<title>Agent availability · DDx</title>
</svelte:head>

<div class="space-y-6" data-testid="agent-endpoints">
	<div class="flex items-center justify-between">
		<h1 class="text-headline-md font-headline-md text-fg-ink dark:text-dark-fg-ink">Agent availability</h1>
		{#if !loading}
			<span class="text-body-sm text-fg-muted dark:text-dark-fg-muted">
				{rows.length} total ({rows.filter((r) => r.kind === 'ENDPOINT').length} endpoints · {rows.filter(
					(r) => r.kind === 'HARNESS'
				).length} harnesses)
			</span>
		{/if}
	</div>

	<!-- Default route widget -->
	{#if defaultRoute && defaultRoute.modelRef}
		<div
			class="border border-border-line bg-bg-surface p-4 dark:border-dark-border-line dark:bg-dark-bg-surface"
		>
			<h2 class="mb-2 text-body-sm font-medium text-fg-muted dark:text-dark-fg-muted">
				Current route for default profile
			</h2>
			<div class="flex flex-wrap gap-4 text-body-sm">
				<span class="text-fg-muted dark:text-dark-fg-muted">
					Model ref: <span class="font-mono-code text-mono-code font-medium text-fg-ink dark:text-dark-fg-ink"
						>{defaultRoute.modelRef}</span
					>
				</span>
				{#if defaultRoute.strategy}
					<span class="text-fg-muted dark:text-dark-fg-muted">
						Strategy: <span class="font-medium text-fg-ink dark:text-dark-fg-ink"
							>{defaultRoute.strategy}</span
						>
					</span>
				{/if}
				{#if defaultRoute.resolvedProvider}
					<span class="text-fg-muted dark:text-dark-fg-muted">
						Resolves to:
						<span class="font-medium text-status-closed">
							{defaultRoute.resolvedProvider}
						</span>
						{#if defaultRoute.resolvedModel}
							/
							<span class="font-mono-code text-mono-code text-fg-muted dark:text-dark-fg-muted"
								>{defaultRoute.resolvedModel}</span
							>
						{/if}
					</span>
				{:else}
					<span class="font-medium text-status-failed">
						No healthy candidate available
					</span>
				{/if}
			</div>
		</div>
	{/if}

	<!-- Unified table -->
	{#if loading}
		<div class="py-8 text-center text-body-sm text-fg-muted dark:text-dark-fg-muted" data-testid="loading">
			Loading agent endpoints…
		</div>
	{:else if error}
		<div
			class="border border-border-line bg-bg-surface p-4 text-body-sm text-error dark:border-dark-border-line dark:bg-dark-bg-surface dark:text-dark-error"
		>
			Error: {error}
		</div>
	{:else}
		<div class="overflow-hidden border border-border-line dark:border-dark-border-line">
			<table class="w-full text-sm" data-testid="agent-endpoints-table">
				<thead>
					<tr class="border-b border-border-line bg-bg-surface dark:border-dark-border-line dark:bg-dark-bg-surface">
						<th class="px-4 py-3 text-left text-label-caps font-label-caps uppercase tracking-wide text-fg-muted dark:text-dark-fg-muted">Name</th>
						<th class="px-4 py-3 text-left text-label-caps font-label-caps uppercase tracking-wide text-fg-muted dark:text-dark-fg-muted">Kind</th>
						<th class="px-4 py-3 text-left text-label-caps font-label-caps uppercase tracking-wide text-fg-muted dark:text-dark-fg-muted">Type</th>
						<th class="px-4 py-3 text-left text-label-caps font-label-caps uppercase tracking-wide text-fg-muted dark:text-dark-fg-muted">Model</th>
						<th class="px-4 py-3 text-left text-label-caps font-label-caps uppercase tracking-wide text-fg-muted dark:text-dark-fg-muted">Status</th>
						<th class="px-4 py-3 text-left text-label-caps font-label-caps uppercase tracking-wide text-fg-muted dark:text-dark-fg-muted"
							>Tokens (1h / 24h)</th
						>
						<th class="px-4 py-3 text-left text-label-caps font-label-caps uppercase tracking-wide text-fg-muted dark:text-dark-fg-muted"
							>Utilization</th
						>
						<th class="px-4 py-3 text-left text-label-caps font-label-caps uppercase tracking-wide text-fg-muted dark:text-dark-fg-muted"
							>Trend (24h)</th
						>
						<th class="px-4 py-3 text-left text-label-caps font-label-caps uppercase tracking-wide text-fg-muted dark:text-dark-fg-muted"
							>Models</th
						>
					</tr>
				</thead>
				<tbody>
					{#each rows as row (row.kind + '|' + row.name)}
						<tr
							class="border-b border-border-line last:border-0 hover:bg-bg-surface dark:border-dark-border-line dark:hover:bg-dark-bg-surface"
							data-testid="endpoint-row-{row.name}"
						>
							<td class="px-4 py-3 font-medium text-fg-ink dark:text-dark-fg-ink">
								<a
									class="text-accent-lever hover:underline dark:text-dark-accent-lever"
									href={detailHref(row)}
									data-testid="endpoint-link-{row.name}">{row.name}</a
								>
								{#if row.isDefault}
									<span
										class="ml-1 inline-flex items-center border border-border-line px-1.5 py-0.5 text-label-caps font-label-caps uppercase text-fg-muted dark:border-dark-border-line dark:text-dark-fg-muted"
									>
										default
									</span>
								{/if}
								{#if row.cooldownUntil}
									<span
										class="ml-1 inline-flex items-center border border-border-line bg-bg-surface px-1.5 py-0.5 text-label-caps font-label-caps uppercase text-error dark:border-dark-border-line dark:bg-dark-bg-surface dark:text-dark-error"
										title="Cooldown until {row.cooldownUntil}"
									>
										cooldown
									</span>
								{/if}
							</td>
							<td
								class="px-4 py-3 text-label-caps font-label-caps uppercase text-fg-muted dark:text-dark-fg-muted"
								data-testid="endpoint-kind-{row.name}"
							>
								{row.kind === 'ENDPOINT' ? 'endpoint' : 'harness'}
							</td>
							<td class="px-4 py-3 text-fg-muted dark:text-dark-fg-muted">
								{row.providerType}
							</td>
							<td
								class="max-w-xs truncate px-4 py-3 font-mono-code text-mono-code text-fg-muted dark:text-dark-fg-muted"
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
								<span class="ml-1 text-fg-muted dark:text-dark-fg-muted" title={row.detail}>
									{row.status}
								</span>
								{#if row.lastCheckedAt}
									<span class="ml-1 text-label-caps text-fg-muted dark:text-dark-fg-muted" title="Last checked {row.lastCheckedAt}"
										>·</span
									>
								{/if}
							</td>
							<td
								class="px-4 py-3 tabular-nums text-fg-muted dark:text-dark-fg-muted"
								data-testid="endpoint-tokens-{row.name}"
							>
								{#if row.usage}
									{formatTokens(row.usage.tokensUsedLastHour)} / {formatTokens(
										row.usage.tokensUsedLast24h
									)}
								{:else}
									<span class="text-fg-muted dark:text-dark-fg-muted">not reported</span>
								{/if}
							</td>
							<td class="px-4 py-3">
								{#if utilizationPct(row.usage, row.quota) != null}
									<div class="flex items-center gap-2">
										<div class="h-2 w-20 overflow-hidden bg-border-line dark:bg-dark-border-line">
											<div
												class="h-full bg-accent-lever dark:bg-dark-accent-lever"
												style="width: {utilizationPct(row.usage, row.quota)}%"
											></div>
										</div>
										<span class="text-label-caps font-label-caps tabular-nums text-fg-muted dark:text-dark-fg-muted">
											{utilizationPct(row.usage, row.quota)}%
										</span>
									</div>
								{:else}
									<span class="text-label-caps font-label-caps text-fg-muted dark:text-dark-fg-muted">not reported</span>
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
												class="w-full bg-accent-lever dark:bg-dark-accent-lever"
												style="height: {sparkBarHeight(v, max)}"
												title="{v} tokens"
											></div>
										{/each}
									</div>
								{:else}
									<span class="text-label-caps font-label-caps text-fg-muted dark:text-dark-fg-muted">—</span>
								{/if}
							</td>
							<td class="px-4 py-3" data-testid="endpoint-models-cell-{row.name}">
								<div class="flex items-center gap-2">
									<button
										type="button"
										class="inline-flex items-center gap-1 text-accent-lever hover:underline dark:text-dark-accent-lever"
										data-testid="endpoint-models-toggle-{row.name}"
										aria-expanded={expanded[rowKey(row)] ? 'true' : 'false'}
										onclick={() => toggleExpanded(row)}
									>
										<span aria-hidden="true">{expanded[rowKey(row)] ? '▼' : '▶'}</span>
										<span>{row.modelCount} models</span>
									</button>
									<button
										type="button"
										class="inline-flex items-center border border-border-line px-1.5 py-0.5 text-label-caps font-label-caps uppercase text-fg-muted hover:bg-bg-surface dark:border-dark-border-line dark:text-dark-fg-muted dark:hover:bg-dark-bg-surface disabled:opacity-50"
										data-testid="endpoint-models-refresh-{row.name}"
										title="Refresh model list"
										aria-label="Refresh model list for {row.name}"
										disabled={modelsLoading[rowKey(row)]}
										onclick={() => refreshModels(row)}
									>
										{modelsLoading[rowKey(row)] ? '…' : '↻'}
									</button>
								</div>
							</td>
						</tr>
						{#if expanded[rowKey(row)]}
							<tr
								class="border-b border-border-line bg-bg-surface dark:border-dark-border-line dark:bg-dark-bg-surface"
								data-testid="endpoint-models-row-{row.name}"
							>
								<td colspan="9" class="px-4 py-3">
									{#if modelsLoading[rowKey(row)] && !modelsByKey[rowKey(row)]}
										<span class="text-body-sm text-fg-muted dark:text-dark-fg-muted">Loading models…</span>
									{:else if modelsError[rowKey(row)]}
										<span class="text-body-sm text-error dark:text-dark-error"
											>Error: {modelsError[rowKey(row)]}</span
										>
									{:else if modelsByKey[rowKey(row)]}
										{@const snap = modelsByKey[rowKey(row)]}
										{@const total = snap.models.length}
										{@const shown = total > MODEL_LIST_INLINE_LIMIT ? snap.models.slice(0, MODEL_LIST_INLINE_LIMIT) : snap.models}
										<div class="space-y-2">
											<div class="flex flex-wrap items-center gap-3 text-label-caps font-label-caps uppercase text-fg-muted dark:text-dark-fg-muted">
												<span>{total} model{total === 1 ? '' : 's'}</span>
												<span title="Last fetched {snap.fetchedAt}">{snap.fromCache ? 'cached' : 'fresh'}</span>
											</div>
											{#if total === 0}
												<span class="text-body-sm text-fg-muted dark:text-dark-fg-muted">No models discovered.</span>
											{:else}
												<ul
													class="flex flex-wrap gap-x-4 gap-y-1 font-mono-code text-mono-code"
													data-testid="endpoint-models-list-{row.name}"
												>
													{#each shown as m (m.id)}
														<li class="text-fg-ink dark:text-dark-fg-ink">
															<span class={m.available ? '' : 'text-fg-muted line-through dark:text-dark-fg-muted'}>{m.id}</span>
															{#if m.contextLength}
																<span class="text-label-caps text-fg-muted dark:text-dark-fg-muted">· {m.contextLength}</span>
															{/if}
														</li>
													{/each}
												</ul>
												{#if total > MODEL_LIST_INLINE_LIMIT}
													<a
														class="text-body-sm text-accent-lever hover:underline dark:text-dark-accent-lever"
														href={detailHref(row)}
														data-testid="endpoint-models-drilldown-{row.name}"
													>
														View all {total} models →
													</a>
												{/if}
											{/if}
										</div>
									{:else}
										<span class="text-body-sm text-fg-muted dark:text-dark-fg-muted">No data.</span>
									{/if}
								</td>
							</tr>
						{/if}
					{/each}
					{#if rows.length === 0}
						<tr>
							<td colspan="9" class="px-4 py-8 text-center text-body-sm text-fg-muted dark:text-dark-fg-muted">
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
