<script lang="ts">
	import { page } from '$app/stores';
	import { onMount } from 'svelte';
	import { createClient } from '$lib/gql/client';
	import { gql } from 'graphql-request';

	const PROVIDER_STATUSES_QUERY = gql`
		query ProviderStatuses {
			providerStatuses {
				name
				providerType
				baseURL
				model
				status
				modelCount
				isDefault
				cooldownUntil
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

	interface ProviderStatus {
		name: string;
		providerType: string;
		baseURL: string;
		model: string;
		status: string;
		modelCount: number;
		isDefault: boolean;
		cooldownUntil: string | null;
	}

	interface DefaultRouteStatus {
		modelRef: string;
		resolvedProvider: string | null;
		resolvedModel: string | null;
		strategy: string | null;
	}

	let providers = $state<ProviderStatus[]>([]);
	let defaultRoute = $state<DefaultRouteStatus | null>(null);
	let loading = $state(true);
	let error = $state<string | null>(null);

	onMount(async () => {
		try {
			const client = createClient();
			const [providersResult, routeResult] = await Promise.all([
				client.request<{ providerStatuses: ProviderStatus[] }>(PROVIDER_STATUSES_QUERY),
				client.request<{ defaultRouteStatus: DefaultRouteStatus | null }>(DEFAULT_ROUTE_QUERY)
			]);
			providers = providersResult.providerStatuses ?? [];
			defaultRoute = routeResult.defaultRouteStatus ?? null;
		} catch (e) {
			error = e instanceof Error ? e.message : String(e);
		} finally {
			loading = false;
		}
	});

	function statusClass(status: string): string {
		const lower = status.toLowerCase();
		if (lower.includes('connected') || lower.includes('api key configured')) {
			return 'text-green-600 dark:text-green-400';
		}
		if (lower.includes('cooldown') || lower.includes('unreachable') || lower.includes('error')) {
			return 'text-red-600 dark:text-red-400';
		}
		return 'text-yellow-600 dark:text-yellow-400';
	}
</script>

<div class="space-y-6">
	<div class="flex items-center justify-between">
		<h1 class="text-xl font-semibold dark:text-white">Providers</h1>
		{#if !loading}
			<span class="text-sm text-gray-500 dark:text-gray-400">
				{providers.length} configured
			</span>
		{/if}
	</div>

	<!-- Default route widget -->
	{#if defaultRoute && defaultRoute.modelRef}
		<div class="rounded-lg border border-gray-200 bg-gray-50 p-4 dark:border-gray-700 dark:bg-gray-800/50">
			<h2 class="mb-2 text-sm font-medium text-gray-700 dark:text-gray-300">
				Current route for default profile
			</h2>
			<div class="flex flex-wrap gap-4 text-sm">
				<span class="text-gray-500 dark:text-gray-400">
					Model ref: <span class="font-mono font-medium text-gray-900 dark:text-white">{defaultRoute.modelRef}</span>
				</span>
				{#if defaultRoute.strategy}
					<span class="text-gray-500 dark:text-gray-400">
						Strategy: <span class="font-medium text-gray-700 dark:text-gray-300">{defaultRoute.strategy}</span>
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
							<span class="font-mono text-gray-700 dark:text-gray-300">{defaultRoute.resolvedModel}</span>
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

	<!-- Provider table -->
	{#if loading}
		<div class="py-8 text-center text-sm text-gray-400 dark:text-gray-600">Loading providers…</div>
	{:else if error}
		<div class="rounded-lg border border-red-200 bg-red-50 p-4 text-sm text-red-700 dark:border-red-800 dark:bg-red-900/20 dark:text-red-400">
			Error: {error}
		</div>
	{:else}
		<div class="overflow-hidden rounded-lg border border-gray-200 dark:border-gray-700">
			<table class="w-full text-sm">
				<thead>
					<tr class="border-b border-gray-200 bg-gray-50 dark:border-gray-700 dark:bg-gray-800">
						<th class="px-4 py-3 text-left font-medium text-gray-600 dark:text-gray-300">Name</th>
						<th class="px-4 py-3 text-left font-medium text-gray-600 dark:text-gray-300">Type</th>
						<th class="px-4 py-3 text-left font-medium text-gray-600 dark:text-gray-300">URL</th>
						<th class="px-4 py-3 text-left font-medium text-gray-600 dark:text-gray-300">Model</th>
						<th class="px-4 py-3 text-left font-medium text-gray-600 dark:text-gray-300">Status</th>
						<th class="px-4 py-3 text-right font-medium text-gray-600 dark:text-gray-300">Models</th>
					</tr>
				</thead>
				<tbody>
					{#each providers as provider (provider.name)}
						<tr class="border-b border-gray-100 last:border-0 dark:border-gray-700">
							<td class="px-4 py-3 font-medium text-gray-900 dark:text-gray-100">
								{provider.name}
								{#if provider.isDefault}
									<span class="ml-1 inline-flex items-center rounded-full bg-blue-100 px-1.5 py-0.5 text-xs font-medium text-blue-700 dark:bg-blue-900/30 dark:text-blue-300">
										default
									</span>
								{/if}
								{#if provider.cooldownUntil}
									<span class="ml-1 inline-flex items-center rounded-full bg-red-100 px-1.5 py-0.5 text-xs font-medium text-red-700 dark:bg-red-900/30 dark:text-red-300"
										title="Cooldown until {provider.cooldownUntil}">
										cooldown
									</span>
								{/if}
							</td>
							<td class="px-4 py-3 text-gray-600 dark:text-gray-400">
								{provider.providerType}
							</td>
							<td class="max-w-xs truncate px-4 py-3 font-mono text-xs text-gray-500 dark:text-gray-400"
								title={provider.baseURL}>
								{provider.baseURL}
							</td>
							<td class="max-w-xs truncate px-4 py-3 font-mono text-xs text-gray-700 dark:text-gray-300"
								title={provider.model}>
								{provider.model || '—'}
							</td>
							<td class="px-4 py-3">
								<span class="font-medium {statusClass(provider.status)}">
									{provider.status}
								</span>
							</td>
							<td class="px-4 py-3 text-right tabular-nums text-gray-600 dark:text-gray-400">
								{provider.modelCount}
							</td>
						</tr>
					{/each}
					{#if providers.length === 0}
						<tr>
							<td colspan="6" class="px-4 py-8 text-center text-gray-400 dark:text-gray-600">
								No providers configured. Add providers to .agent/config.yaml.
							</td>
						</tr>
					{/if}
				</tbody>
			</table>
		</div>
	{/if}
</div>
