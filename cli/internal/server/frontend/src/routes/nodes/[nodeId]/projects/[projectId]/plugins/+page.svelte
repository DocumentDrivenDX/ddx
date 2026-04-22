<script lang="ts">
	import type { PageData } from './$types';
	import { page } from '$app/stores';
	import { createClient } from '$lib/gql/client';
	import { PLUGIN_DISPATCH_MUTATION } from '$lib/gql/feat008';
	import ConfirmDialog from '$lib/components/ConfirmDialog.svelte';
	import { Download, ExternalLink, PackageCheck, RefreshCw } from 'lucide-svelte';

	type Scope = 'global' | 'project';

	interface DispatchResult {
		pluginDispatch: {
			id: string;
			state: string;
			action: string;
		};
	}

	let { data }: { data: PageData } = $props();

	let installingPlugin = $state<PageData['plugins'][number] | null>(null);
	let installOpen = $state(false);
	let installScope = $state<Scope>('global');
	let activePlugin = $state<string | null>(null);
	let workerId = $state<string | null>(null);
	let dispatchError = $state<string | null>(null);

	const client = createClient();

	function pluginHref(name: string): string {
		const p = $page.params as Record<string, string>;
		return `/nodes/${p['nodeId']}/projects/${p['projectId']}/plugins/${name}`;
	}

	function workerHref(id: string): string {
		const p = $page.params as Record<string, string>;
		return `/nodes/${p['nodeId']}/projects/${p['projectId']}/workers/${id}`;
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

	function openInstall(plugin: PageData['plugins'][number]) {
		dispatchError = null;
		installScope = 'global';
		installingPlugin = plugin;
		installOpen = true;
	}

	async function dispatchPlugin(name: string, action: string, scope: Scope = 'project') {
		dispatchError = null;
		activePlugin = name;
		try {
			const result = await client.request<DispatchResult>(PLUGIN_DISPATCH_MUTATION, {
				name,
				action,
				scope
			});
			workerId = result.pluginDispatch.id;
		} catch (err) {
			dispatchError = err instanceof Error ? err.message : 'Plugin action failed.';
		} finally {
			activePlugin = null;
		}
	}

	async function confirmInstall() {
		if (!installingPlugin) return;
		await dispatchPlugin(installingPlugin.name, 'install', installScope);
		installingPlugin = null;
		installOpen = false;
	}
</script>

<div class="space-y-6">
	<div class="flex flex-wrap items-start justify-between gap-4">
		<div>
			<h1 class="text-xl font-semibold text-gray-950 dark:text-white">Plugins</h1>
			<p class="mt-1 text-sm text-gray-600 dark:text-gray-300">
				{data.plugins.length} registry entries
			</p>
		</div>
		{#if workerId}
			<a
				href={workerHref(workerId)}
				class="inline-flex items-center gap-2 rounded-md border border-blue-200 bg-blue-50 px-3 py-2 text-sm font-medium text-blue-700 hover:bg-blue-100 dark:border-blue-900 dark:bg-blue-950 dark:text-blue-300 dark:hover:bg-blue-900"
			>
				<ExternalLink class="h-4 w-4" aria-hidden="true" />
				{workerId}
			</a>
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
		{#each data.plugins as plugin (plugin.name)}
			<article
				aria-label={plugin.name}
				class="flex min-h-72 flex-col rounded-lg border border-gray-200 bg-white p-5 shadow-sm shadow-gray-900/5 dark:border-gray-800 dark:bg-gray-900 dark:shadow-black/20"
			>
				<div class="flex items-start justify-between gap-3">
					<div class="min-w-0">
						<a
							href={pluginHref(plugin.name)}
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
					<span
						class="shrink-0 rounded-full border px-2 py-1 text-xs font-medium {statusClass(
							plugin.status
						)}"
					>
						{statusLabel(plugin.status)}
					</span>
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
						{#each plugin.keywords as keyword}
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
						<button
							type="button"
							class="inline-flex items-center gap-2 rounded-md bg-blue-600 px-3 py-2 text-sm font-medium text-white hover:bg-blue-700 disabled:cursor-not-allowed disabled:bg-blue-400 dark:bg-blue-600 dark:hover:bg-blue-500"
							disabled={activePlugin === plugin.name}
							onclick={() => openInstall(plugin)}
						>
							<Download class="h-4 w-4" aria-hidden="true" />
							Install
						</button>
					{:else if plugin.status === 'update-available'}
						<button
							type="button"
							aria-label="Update plugin"
							class="inline-flex h-9 w-9 items-center justify-center rounded-md bg-amber-600 text-white hover:bg-amber-700 disabled:cursor-not-allowed disabled:bg-amber-400 dark:bg-amber-600 dark:hover:bg-amber-500"
							disabled={activePlugin === plugin.name}
							onclick={() => dispatchPlugin(plugin.name, 'update')}
						>
							<RefreshCw class="h-4 w-4" aria-hidden="true" />
						</button>
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
