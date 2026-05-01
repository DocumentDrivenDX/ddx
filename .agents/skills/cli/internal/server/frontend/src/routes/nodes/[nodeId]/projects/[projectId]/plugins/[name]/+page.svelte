<script lang="ts">
	import type { PageData } from './$types';
	import { page } from '$app/stores';
	import { createClient } from '$lib/gql/client';
	import { PLUGIN_DISPATCH_MUTATION } from '$lib/gql/feat008';
	import ConfirmDialog from '$lib/components/ConfirmDialog.svelte';
	import { ArrowLeft, ExternalLink, RefreshCw, Trash2 } from 'lucide-svelte';

	interface DispatchResult {
		pluginDispatch: {
			id: string;
			state: string;
			action: string;
		};
	}

	let { data }: { data: PageData } = $props();

	let uninstallOpen = $state(false);
	let workerId = $state<string | null>(null);
	let dispatchError = $state<string | null>(null);
	let busyAction = $state<string | null>(null);

	const client = createClient();
	const plugin = $derived(data.plugin);
	const manifestYaml = $derived(toYaml(plugin.manifest));

	function pluginsHref(): string {
		const p = $page.params as Record<string, string>;
		return `/nodes/${p['nodeId']}/projects/${p['projectId']}/plugins`;
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

	function yamlScalar(value: unknown): string {
		if (typeof value === 'string') return value;
		if (typeof value === 'number' || typeof value === 'boolean') return String(value);
		if (value === null) return 'null';
		return JSON.stringify(value);
	}

	function renderYaml(value: unknown, indent = 0): string {
		const pad = ' '.repeat(indent);
		if (Array.isArray(value)) {
			if (value.length === 0) return `${pad}[]`;
			return value
				.map((item) =>
					item && typeof item === 'object'
						? `${pad}-\n${renderYaml(item, indent + 2)}`
						: `${pad}- ${yamlScalar(item)}`
				)
				.join('\n');
		}
		if (value && typeof value === 'object') {
			const entries = Object.entries(value as Record<string, unknown>);
			if (entries.length === 0) return `${pad}{}`;
			return entries
				.map(([key, item]) =>
					item && typeof item === 'object'
						? `${pad}${key}:\n${renderYaml(item, indent + 2)}`
						: `${pad}${key}: ${yamlScalar(item)}`
				)
				.join('\n');
		}
		return `${pad}${yamlScalar(value)}`;
	}

	function toYaml(value: unknown): string {
		if (value === null || value === undefined || value === '') return 'manifest: {}';
		if (typeof value === 'string') {
			try {
				return renderYaml(JSON.parse(value));
			} catch {
				return value;
			}
		}
		return renderYaml(value);
	}

	async function dispatchPlugin(action: string, scope = 'project') {
		dispatchError = null;
		busyAction = action;
		try {
			const result = await client.request<DispatchResult>(PLUGIN_DISPATCH_MUTATION, {
				name: plugin.name,
				action,
				scope
			});
			workerId = result.pluginDispatch.id;
		} catch (err) {
			dispatchError = err instanceof Error ? err.message : 'Plugin action failed.';
		} finally {
			busyAction = null;
		}
	}

	function sectionItems(items: string[]): string[] {
		return items.length > 0 ? items : ['None'];
	}
</script>

<div class="space-y-6">
	<div class="flex flex-wrap items-start justify-between gap-4">
		<div class="min-w-0">
			<a
				href={pluginsHref()}
				class="inline-flex items-center gap-2 text-sm font-medium text-gray-600 hover:text-gray-950 dark:text-gray-300 dark:hover:text-white"
			>
				<ArrowLeft class="h-4 w-4" aria-hidden="true" />
				Plugins
			</a>
			<h1 class="mt-3 text-2xl font-semibold break-words text-gray-950 dark:text-white">
				{plugin.name}
			</h1>
			<p class="mt-2 max-w-3xl text-sm leading-6 text-gray-700 dark:text-gray-300">
				{plugin.description}
			</p>
		</div>
		<div class="flex flex-wrap items-center gap-2">
			{#if plugin.status === 'update-available'}
				<button
					type="button"
					class="inline-flex items-center gap-2 rounded-md bg-amber-600 px-3 py-2 text-sm font-medium text-white hover:bg-amber-700 disabled:cursor-not-allowed disabled:bg-amber-400 dark:bg-amber-600 dark:hover:bg-amber-500"
					disabled={busyAction === 'update'}
					onclick={() => dispatchPlugin('update')}
				>
					<RefreshCw class="h-4 w-4" aria-hidden="true" />
					Update
				</button>
			{/if}
			{#if plugin.installedVersion}
				<button
					type="button"
					class="inline-flex items-center gap-2 rounded-md bg-red-600 px-3 py-2 text-sm font-medium text-white hover:bg-red-700 disabled:cursor-not-allowed disabled:bg-red-400 dark:bg-red-600 dark:hover:bg-red-500"
					disabled={busyAction === 'uninstall'}
					onclick={() => (uninstallOpen = true)}
				>
					<Trash2 class="h-4 w-4" aria-hidden="true" />
					Uninstall
				</button>
			{/if}
		</div>
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

	{#if dispatchError}
		<div
			class="rounded-md border border-red-200 bg-red-50 px-4 py-3 text-sm text-red-700 dark:border-red-900 dark:bg-red-950 dark:text-red-300"
		>
			{dispatchError}
		</div>
	{/if}

	<div class="grid gap-4 lg:grid-cols-[minmax(0,1.3fr)_minmax(18rem,0.7fr)]">
		<section
			aria-label="Manifest"
			class="rounded-lg border border-gray-200 bg-white p-5 dark:border-gray-800 dark:bg-gray-900"
		>
			<h2 class="text-base font-semibold text-gray-950 dark:text-white">Manifest</h2>
			<pre
				class="mt-4 overflow-auto rounded-md bg-gray-950 p-4 text-sm leading-6 text-gray-100"><code
					>{manifestYaml}</code
				></pre>
		</section>

		<section
			class="rounded-lg border border-gray-200 bg-white p-5 dark:border-gray-800 dark:bg-gray-900"
		>
			<h2 class="text-base font-semibold text-gray-950 dark:text-white">Versions</h2>
			<div class="mt-4 grid gap-3 text-sm">
				<div class="flex items-center justify-between gap-4">
					<span class="text-gray-500 dark:text-gray-400">Registry</span>
					<span class="font-mono text-gray-950 dark:text-white">{plugin.version}</span>
				</div>
				<div class="flex items-center justify-between gap-4">
					<span class="text-gray-500 dark:text-gray-400">Installed</span>
					<span class="font-mono text-gray-950 dark:text-white"
						>{plugin.installedVersion ?? 'Not installed'}</span
					>
				</div>
				<div class="flex items-center justify-between gap-4">
					<span class="text-gray-500 dark:text-gray-400">Type</span>
					<span class="text-gray-950 dark:text-white">{plugin.type}</span>
				</div>
				<div class="flex items-center justify-between gap-4">
					<span class="text-gray-500 dark:text-gray-400">Disk</span>
					<span class="font-mono text-gray-950 dark:text-white">{formatDisk(plugin.diskBytes)}</span
					>
				</div>
			</div>
		</section>
	</div>

	<div class="grid gap-4 md:grid-cols-3">
		<section
			aria-label="Skills"
			class="rounded-lg border border-gray-200 bg-white p-5 dark:border-gray-800 dark:bg-gray-900"
		>
			<h2 class="text-base font-semibold text-gray-950 dark:text-white">Skills</h2>
			<ul class="mt-4 space-y-2 text-sm text-gray-700 dark:text-gray-300">
				{#each sectionItems(plugin.skills) as item}
					<li
						class="rounded border border-gray-200 px-3 py-2 font-mono text-xs dark:border-gray-700"
					>
						{item}
					</li>
				{/each}
			</ul>
		</section>
		<section
			aria-label="Prompts"
			class="rounded-lg border border-gray-200 bg-white p-5 dark:border-gray-800 dark:bg-gray-900"
		>
			<h2 class="text-base font-semibold text-gray-950 dark:text-white">Prompts</h2>
			<ul class="mt-4 space-y-2 text-sm text-gray-700 dark:text-gray-300">
				{#each sectionItems(plugin.prompts) as item}
					<li
						class="rounded border border-gray-200 px-3 py-2 font-mono text-xs dark:border-gray-700"
					>
						{item}
					</li>
				{/each}
			</ul>
		</section>
		<section
			aria-label="Templates"
			class="rounded-lg border border-gray-200 bg-white p-5 dark:border-gray-800 dark:bg-gray-900"
		>
			<h2 class="text-base font-semibold text-gray-950 dark:text-white">Templates</h2>
			<ul class="mt-4 space-y-2 text-sm text-gray-700 dark:text-gray-300">
				{#each sectionItems(plugin.templates) as item}
					<li
						class="rounded border border-gray-200 px-3 py-2 font-mono text-xs dark:border-gray-700"
					>
						{item}
					</li>
				{/each}
			</ul>
		</section>
	</div>
</div>

<ConfirmDialog
	bind:open={uninstallOpen}
	actionLabel="Remove plugin"
	title="Uninstall {plugin.name}"
	destructive
	onConfirm={() => dispatchPlugin('uninstall')}
>
	{#snippet summary()}
		Remove {plugin.name} from this project.
	{/snippet}
	<p>
		This will queue an uninstall worker for {plugin.name}. Installed artifacts and plugin-provided
		surfaces will no longer be available after the worker completes.
	</p>
</ConfirmDialog>
