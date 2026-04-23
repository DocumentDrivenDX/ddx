<script lang="ts">
	import type { PageData } from './$types';
	import { resolve } from '$app/paths';
	import { page } from '$app/stores';
	import { createClient } from '$lib/gql/client';
	import { PLUGIN_DISPATCH_MUTATION, PLUGINS_LIST_QUERY } from '$lib/gql/feat008';
	import { subscribeWorkerProgress } from '$lib/gql/subscriptions';
	import ConfirmDialog from '$lib/components/ConfirmDialog.svelte';
	import Tooltip from '$lib/components/Tooltip.svelte';
	import { onDestroy } from 'svelte';
	import { SvelteMap, SvelteSet } from 'svelte/reactivity';
	import {
		AlertCircle,
		Download,
		ExternalLink,
		Loader2,
		PackageCheck,
		RefreshCw
	} from 'lucide-svelte';

	type Scope = 'global' | 'project';
	type PluginAction = 'install' | 'update';
	type PluginInfo = PageData['plugins'][number];

	interface DispatchResult {
		pluginDispatch: {
			id: string;
			state: string;
			action: PluginAction;
		};
	}

	interface PluginsListResult {
		pluginsList: PluginInfo[];
	}

	interface PluginSnapshot {
		status: string;
		installedVersion: string | null;
	}

	interface FailureState {
		workerId: string;
		action: PluginAction;
	}

	let { data }: { data: PageData } = $props();

	// Local list is refreshed after plugin workers finish.
	// svelte-ignore state_referenced_locally
	const initialPlugins = data.plugins;
	let plugins = $state<PluginInfo[]>(initialPlugins);
	let installingPlugin = $state<PluginInfo | null>(null);
	let installOpen = $state(false);
	let installScope = $state<Scope>('global');
	const dispatchingPlugins = new SvelteSet<string>();
	const inFlightWorkers = new SvelteMap<string, string>();
	const inFlightActions = new SvelteMap<string, PluginAction>();
	const dispatchSnapshots = new SvelteMap<string, PluginSnapshot>();
	const workerFailures = new SvelteMap<string, FailureState>();
	let dispatchError = $state<string | null>(null);

	const client = createClient();
	const terminalPhases = ['done', 'exited', 'stopped', 'failed', 'error', 'preserved'];
	const failurePhases = ['failed', 'error'];
	const subscriptions = new SvelteMap<string, () => void>();
	const fallbackTimeouts = new SvelteMap<string, number>();
	const pollingIntervals = new SvelteMap<string, number>();

	function fallbackDelayMs(): number {
		if (typeof window === 'undefined') return 30_000;
		const value = (window as typeof window & { __ddxPluginFallbackDelayMs?: number })
			.__ddxPluginFallbackDelayMs;
		return typeof value === 'number' && value >= 0 ? value : 30_000;
	}

	function pollIntervalMs(): number {
		if (typeof window === 'undefined') return 2_000;
		const value = (window as typeof window & { __ddxPluginPollIntervalMs?: number })
			.__ddxPluginPollIntervalMs;
		return typeof value === 'number' && value > 0 ? value : 2_000;
	}

	function formatDisk(bytes: number): string {
		const units = ['B', 'KB', 'MB', 'GB'];
		let value = bytes;
		let index = 0;
		while (value >= 1000 && index < units.length - 1) {
			value /= 1000;
			index += 1;
		}
		const display = value >= 10 || Number.isInteger(value) ? value.toFixed(0) : value.toFixed(1);
		return `${display} ${units[index]}`;
	}

	function statusLabel(status: string): string {
		if (status === 'update-available') return 'Update available';
		return status;
	}

	function statusClass(status: string): string {
		if (status === 'installed') {
			return 'border-emerald-200 bg-emerald-50 text-emerald-700 dark:border-emerald-900 dark:bg-emerald-950 dark:text-emerald-300';
		}
		if (status === 'update-available') {
			return 'border-amber-200 bg-amber-50 text-amber-800 dark:border-amber-900 dark:bg-amber-950 dark:text-amber-300';
		}
		return 'border-sky-200 bg-sky-50 text-sky-700 dark:border-sky-900 dark:bg-sky-950 dark:text-sky-300';
	}

	function pluginByName(name: string): PluginInfo | null {
		return plugins.find((plugin) => plugin.name === name) ?? null;
	}

	function isBusy(name: string): boolean {
		return dispatchingPlugins.has(name) || inFlightWorkers.has(name);
	}

	function inFlightActionLabel(action: PluginAction): string {
		return action === 'install' ? 'Installing...' : 'Updating...';
	}

	function actionFailedLabel(action: PluginAction): string {
		return action === 'install' ? 'Install failed' : 'Update failed';
	}

	function isTerminalPhase(phase: string): boolean {
		return terminalPhases.includes(phase);
	}

	function isFailurePhase(phase: string): boolean {
		return failurePhases.includes(phase);
	}

	function setDispatching(name: string, value: boolean) {
		if (value) dispatchingPlugins.add(name);
		else dispatchingPlugins.delete(name);
	}

	function clearFailure(name: string) {
		workerFailures.delete(name);
	}

	function openInstall(plugin: PluginInfo) {
		if (isBusy(plugin.name)) return;
		dispatchError = null;
		installScope = 'global';
		installingPlugin = plugin;
		installOpen = true;
	}

	function snapshotFor(plugin: PluginInfo): PluginSnapshot {
		return {
			status: plugin.status,
			installedVersion: plugin.installedVersion
		};
	}

	function hasPluginChanged(name: string, previous: PluginSnapshot): boolean {
		const current = pluginByName(name);
		if (!current) return true;
		return (
			current.status !== previous.status || current.installedVersion !== previous.installedVersion
		);
	}

	function clearInFlight(name: string) {
		subscriptions.get(name)?.();
		subscriptions.delete(name);

		const timeout = fallbackTimeouts.get(name);
		if (timeout != null) window.clearTimeout(timeout);
		fallbackTimeouts.delete(name);

		const interval = pollingIntervals.get(name);
		if (interval != null) window.clearInterval(interval);
		pollingIntervals.delete(name);

		inFlightWorkers.delete(name);
		inFlightActions.delete(name);
		dispatchSnapshots.delete(name);
	}

	async function refreshPlugins(): Promise<PluginInfo[]> {
		const result = await client.request<PluginsListResult>(PLUGINS_LIST_QUERY);
		plugins = result.pluginsList;
		return result.pluginsList;
	}

	function startFallbackPolling(name: string) {
		if (pollingIntervals.has(name)) return;
		const interval = window.setInterval(() => {
			void pollPluginUntilChanged(name);
		}, pollIntervalMs());
		pollingIntervals.set(name, interval);
		void pollPluginUntilChanged(name);
	}

	async function pollPluginUntilChanged(name: string) {
		if (!inFlightWorkers.has(name)) {
			clearInFlight(name);
			return;
		}
		const previous = dispatchSnapshots.get(name);
		if (!previous) return;

		try {
			await refreshPlugins();
			if (hasPluginChanged(name, previous)) {
				clearInFlight(name);
			}
		} catch (err) {
			console.error('[ddx] plugin refresh poll failed:', err);
		}
	}

	function scheduleFallbackPolling(name: string) {
		const timeout = window.setTimeout(() => {
			fallbackTimeouts.delete(name);
			if (inFlightWorkers.has(name)) startFallbackPolling(name);
		}, fallbackDelayMs());
		fallbackTimeouts.set(name, timeout);
	}

	function subscribeToWorker(name: string, workerId: string, action: PluginAction) {
		subscriptions.get(name)?.();
		const dispose = subscribeWorkerProgress(
			workerId,
			(evt) => {
				if (!isTerminalPhase(evt.phase)) return;
				void handleWorkerTerminal(name, workerId, action, evt.phase);
			},
			(err) => {
				console.error('[ddx] plugin workerProgress subscription error:', err);
			}
		);
		subscriptions.set(name, dispose);
	}

	async function handleWorkerTerminal(
		name: string,
		workerId: string,
		action: PluginAction,
		phase: string
	) {
		if (!inFlightWorkers.has(name)) return;
		try {
			await refreshPlugins();
		} catch (err) {
			console.error('[ddx] plugin refresh after worker terminal failed:', err);
		}

		clearInFlight(name);
		if (isFailurePhase(phase)) {
			workerFailures.set(name, { workerId, action });
		}
	}

	async function dispatchPlugin(
		name: string,
		action: PluginAction,
		scope: Scope = 'project'
	): Promise<boolean> {
		if (isBusy(name)) return false;
		dispatchError = null;
		clearFailure(name);
		setDispatching(name, true);
		try {
			const result = await client.request<DispatchResult>(PLUGIN_DISPATCH_MUTATION, {
				name,
				action,
				scope
			});
			const workerId = result.pluginDispatch.id;
			const plugin = pluginByName(name);
			inFlightWorkers.set(name, workerId);
			inFlightActions.set(name, action);

			if (plugin) {
				dispatchSnapshots.set(name, snapshotFor(plugin));
			}

			subscribeToWorker(name, workerId, action);
			scheduleFallbackPolling(name);
			return true;
		} catch (err) {
			dispatchError = err instanceof Error ? err.message : 'Plugin action failed.';
			return false;
		} finally {
			setDispatching(name, false);
		}
	}

	async function confirmInstall() {
		if (!installingPlugin) return;
		const dispatched = await dispatchPlugin(installingPlugin.name, 'install', installScope);
		if (!dispatched) return;
		installingPlugin = null;
		installOpen = false;
	}

	onDestroy(() => {
		for (const dispose of subscriptions.values()) dispose();
		for (const timeout of fallbackTimeouts.values()) window.clearTimeout(timeout);
		for (const interval of pollingIntervals.values()) window.clearInterval(interval);
	});
</script>

<div class="space-y-6">
	<div class="flex flex-wrap items-start justify-between gap-4">
		<div>
			<h1 class="text-xl font-semibold text-gray-950 dark:text-white">Plugins</h1>
			<p class="mt-1 text-sm text-gray-600 dark:text-gray-300">
				{plugins.length} registry entries
			</p>
		</div>
		{#if inFlightWorkers.size > 0}
			<div class="flex max-w-full flex-wrap justify-end gap-2" aria-label="Active plugin workers">
				{#each Array.from(inFlightWorkers.entries()) as [pluginName, id] (pluginName)}
					<a
						href={resolve('/nodes/[nodeId]/projects/[projectId]/workers/[workerId]', {
							nodeId: $page.params.nodeId!,
							projectId: $page.params.projectId!,
							workerId: id
						})}
						class="inline-flex items-center gap-2 rounded-md border border-blue-200 bg-blue-50 px-3 py-2 text-sm font-medium text-blue-700 hover:bg-blue-100 dark:border-blue-900 dark:bg-blue-950 dark:text-blue-300 dark:hover:bg-blue-900"
					>
						<ExternalLink class="h-4 w-4" aria-hidden="true" />
						<span class="font-medium">{pluginName}</span>
						<span class="font-mono">{id}</span>
					</a>
				{/each}
			</div>
		{/if}
	</div>

	{#if dispatchError}
		<div
			class="rounded-md border border-red-200 bg-red-50 px-4 py-3 text-sm text-red-700 dark:border-red-900 dark:bg-red-950 dark:text-red-300"
		>
			{dispatchError}
		</div>
	{/if}

	<div class="grid gap-4 md:grid-cols-2 xl:grid-cols-3">
		{#each plugins as plugin (plugin.name)}
			{@const workerId = inFlightWorkers.get(plugin.name)}
			{@const action = inFlightActions.get(plugin.name)}
			{@const failure = workerFailures.get(plugin.name)}
			{@const busy = isBusy(plugin.name)}
			<article
				aria-label={plugin.name}
				class="flex min-h-72 flex-col rounded-lg border border-gray-200 bg-white p-5 shadow-sm shadow-gray-900/5 dark:border-gray-800 dark:bg-gray-900 dark:shadow-black/20"
			>
				<div class="flex items-start justify-between gap-3">
					<div class="min-w-0">
						<a
							href={resolve('/nodes/[nodeId]/projects/[projectId]/plugins/[name]', {
								nodeId: $page.params.nodeId!,
								projectId: $page.params.projectId!,
								name: plugin.name
							})}
							class="text-lg font-semibold break-words text-gray-950 hover:text-blue-700 dark:text-white dark:hover:text-blue-300"
						>
							{plugin.name}
						</a>
						<div
							class="mt-1 flex flex-wrap items-center gap-2 text-xs text-gray-600 dark:text-gray-300"
						>
							<span>{plugin.type}</span>
							<span aria-hidden="true">/</span>
							<span>{plugin.registrySource}</span>
						</div>
					</div>
					<div class="flex shrink-0 flex-wrap justify-end gap-2">
						{#if workerId && action}
							<span
								class="inline-flex items-center gap-1.5 rounded-full border border-blue-200 bg-blue-50 px-2 py-1 text-xs font-medium text-blue-700 dark:border-blue-900 dark:bg-blue-950 dark:text-blue-300"
							>
								<Loader2 class="h-3.5 w-3.5 animate-spin" aria-hidden="true" />
								{inFlightActionLabel(action)}
							</span>
						{:else}
							<span
								class="shrink-0 rounded-full border px-2 py-1 text-xs font-medium {statusClass(
									plugin.status
								)}"
							>
								{statusLabel(plugin.status)}
							</span>
							{#if failure}
								<a
									href={resolve('/nodes/[nodeId]/projects/[projectId]/workers/[workerId]', {
										nodeId: $page.params.nodeId!,
										projectId: $page.params.projectId!,
										workerId: failure.workerId
									})}
									title="{actionFailedLabel(failure.action)} — view worker"
									class="inline-flex items-center gap-1.5 rounded-full border border-red-200 bg-red-50 px-2 py-1 text-xs font-medium text-red-700 hover:bg-red-100 dark:border-red-900 dark:bg-red-950 dark:text-red-300 dark:hover:bg-red-900"
								>
									<AlertCircle class="h-3.5 w-3.5" aria-hidden="true" />
									{actionFailedLabel(failure.action)}
								</a>
							{/if}
						{/if}
					</div>
				</div>

				<p class="mt-4 flex-1 text-sm leading-6 text-gray-700 dark:text-gray-300">
					{plugin.description}
				</p>

				<div class="mt-4 grid gap-2 text-sm">
					<div class="flex items-center justify-between gap-3">
						<span class="text-gray-500 dark:text-gray-400">Registry</span>
						<span class="font-mono text-gray-900 dark:text-gray-100">{plugin.version}</span>
					</div>
					{#if plugin.installedVersion}
						<div class="flex items-center justify-between gap-3">
							<span class="text-gray-500 dark:text-gray-400">Current</span>
							<span class="font-mono text-gray-900 dark:text-gray-100"
								>{plugin.installedVersion}</span
							>
						</div>
					{/if}
					<div class="flex items-center justify-between gap-3">
						<span class="text-gray-500 dark:text-gray-400">Disk</span>
						<span class="font-mono text-gray-900 dark:text-gray-100"
							>{formatDisk(plugin.diskBytes)}</span
						>
					</div>
				</div>

				{#if plugin.keywords.length > 0}
					<div class="mt-4 flex flex-wrap gap-2">
						{#each plugin.keywords as keyword (keyword)}
							<span
								class="rounded border border-gray-200 px-2 py-1 text-xs text-gray-600 dark:border-gray-700 dark:text-gray-300"
							>
								{keyword}
							</span>
						{/each}
					</div>
				{/if}

				<div class="mt-5 flex items-center gap-2">
					{#if plugin.status === 'available'}
						<Tooltip
							content={workerId ? `Worker ${workerId}` : undefined}
							disabled={!workerId}
							disabledTrigger={Boolean(workerId)}
						>
							<button
								type="button"
								title={workerId ? `Worker ${workerId}` : undefined}
								class="inline-flex items-center gap-2 rounded-md bg-blue-600 px-3 py-2 text-sm font-medium text-white hover:bg-blue-700 disabled:cursor-not-allowed disabled:bg-blue-400 dark:bg-blue-600 dark:hover:bg-blue-500"
								disabled={busy}
								onclick={() => openInstall(plugin)}
							>
								<Download class="h-4 w-4" aria-hidden="true" />
								Install
							</button>
						</Tooltip>
					{:else if plugin.status === 'update-available'}
						<Tooltip
							content={workerId ? `Worker ${workerId}` : undefined}
							disabled={!workerId}
							disabledTrigger={Boolean(workerId)}
						>
							<button
								type="button"
								aria-label="Update plugin"
								title={workerId ? `Worker ${workerId}` : undefined}
								class="inline-flex h-9 w-9 items-center justify-center rounded-md bg-amber-600 text-white hover:bg-amber-700 disabled:cursor-not-allowed disabled:bg-amber-400 dark:bg-amber-600 dark:hover:bg-amber-500"
								disabled={busy}
								onclick={() => dispatchPlugin(plugin.name, 'update')}
							>
								<RefreshCw class="h-4 w-4" aria-hidden="true" />
							</button>
						</Tooltip>
					{:else}
						<span
							class="inline-flex items-center gap-2 text-sm font-medium text-emerald-700 dark:text-emerald-300"
						>
							<PackageCheck class="h-4 w-4" aria-hidden="true" />
							Ready
						</span>
					{/if}
				</div>
			</article>
		{/each}
	</div>
</div>

<ConfirmDialog
	bind:open={installOpen}
	actionLabel="Install plugin"
	title="Install {installingPlugin?.name ?? 'plugin'}"
	onConfirm={confirmInstall}
	onCancel={() => (installingPlugin = null)}
	onOpenChange={(open) => {
		if (!open) installingPlugin = null;
	}}
>
	{#snippet summary()}
		Choose where DDx should install this plugin.
	{/snippet}
	{#if installingPlugin}
		<div class="space-y-4">
			<div class="rounded-md bg-gray-50 p-3 dark:bg-gray-800">
				<div class="text-xs font-medium text-gray-500 uppercase dark:text-gray-400">
					Disk estimate
				</div>
				<div class="mt-1 font-mono text-base text-gray-950 dark:text-white">
					{formatDisk(installingPlugin.diskBytes)}
				</div>
			</div>
			<div role="radiogroup" aria-label="Scope" class="grid gap-2">
				<label
					class="flex items-center gap-3 rounded-md border border-gray-200 p-3 dark:border-gray-700"
				>
					<input type="radio" name="install-scope" value="global" bind:group={installScope} />
					<span>Global</span>
				</label>
				<label
					class="flex items-center gap-3 rounded-md border border-gray-200 p-3 dark:border-gray-700"
				>
					<input type="radio" name="install-scope" value="project" bind:group={installScope} />
					<span>Project</span>
				</label>
			</div>
		</div>
	{/if}
</ConfirmDialog>
