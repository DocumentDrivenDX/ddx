<script lang="ts">
	import {
		LayoutDashboard,
		FileText,
		GitBranch,
		Cpu,
		Terminal,
		Users,
		GitCommit,
		Moon,
		Sun,
		Radio,
		Layers,
		BarChart3
	} from 'lucide-svelte';
	import { page } from '$app/stores';
	import { toggleMode, mode } from '$lib/theme';
	import ProjectPicker from './ProjectPicker.svelte';
	import { nodeStore } from '$lib/stores/node.svelte';
	import { projectStore } from '$lib/stores/project.svelte';
	import { wsConnection } from '$lib/stores/connection.svelte';

	let { children } = $props();

	const pages = [
		{ page: 'beads', label: 'Beads', Icon: LayoutDashboard },
		{ page: 'documents', label: 'Documents', Icon: FileText },
		{ page: 'graph', label: 'Graph', Icon: GitBranch },
		{ page: 'workers', label: 'Workers', Icon: Cpu },
		{ page: 'sessions', label: 'Sessions', Icon: Terminal },
		{ page: 'personas', label: 'Personas', Icon: Users },
		{ page: 'commits', label: 'Commits', Icon: GitCommit },
		{ page: 'efficacy', label: 'Efficacy', Icon: BarChart3 }
	];

	const navLinks = $derived(
		pages.map(({ page, label, Icon }) => {
			const nodeId = nodeStore.value?.id;
			const projectId = projectStore.value?.id;
			const href = nodeId && projectId ? `/nodes/${nodeId}/projects/${projectId}/${page}` : null;
			return { href, label, Icon };
		})
	);

	const allBeadsHref = $derived(nodeStore.value?.id ? `/nodes/${nodeStore.value.id}/beads` : null);

	const providersHref = $derived(
		nodeStore.value?.id ? `/nodes/${nodeStore.value.id}/providers` : null
	);

	const nodeName = $derived(nodeStore.value?.name ?? 'localhost');
</script>

<div class="flex h-screen flex-col bg-white dark:bg-gray-950">
	<!-- Top nav -->
	<header
		class="flex shrink-0 items-center gap-4 border-b border-gray-200 px-4 py-2 dark:border-gray-800 dark:bg-gray-900"
	>
		<span class="text-lg font-semibold tracking-tight dark:text-white">DDx</span>
		<span class="text-xs text-gray-700 dark:text-gray-300">Node: {nodeName}</span>
		<div class="mx-2 h-4 w-px bg-gray-200 dark:bg-gray-700"></div>
		<ProjectPicker />
		<div class="ml-auto">
			<button
				onclick={toggleMode}
				class="rounded p-1.5 text-gray-500 hover:bg-gray-100 dark:text-gray-400 dark:hover:bg-gray-800"
				aria-label="Toggle dark mode"
			>
				{#if mode.current === 'dark'}
					<Sun class="h-4 w-4" />
				{:else}
					<Moon class="h-4 w-4" />
				{/if}
			</button>
		</div>
	</header>

	{#if wsConnection.showBanner}
		<div
			data-testid="ws-disconnected-banner"
			class="flex shrink-0 items-center gap-2 border-b border-yellow-300 bg-yellow-50 px-4 py-1 text-xs text-yellow-800 dark:border-yellow-700 dark:bg-yellow-900/30 dark:text-yellow-300"
		>
			<span class="inline-block h-2 w-2 rounded-full bg-yellow-500"></span>
			{wsConnection.state === 'connecting' ? 'reconnecting\u2026' : 'disconnected'}
		</div>
	{/if}

	<div class="flex min-h-0 flex-1">
		<!-- Sidebar -->
		<nav
			class="flex w-48 shrink-0 flex-col gap-1 border-r border-gray-200 p-2 dark:border-gray-800 dark:bg-gray-900"
		>
			{#each navLinks as { href, label, Icon }}
				{#if href}
					{@const active = $page.url.pathname.startsWith(href)}
					<a
						{href}
						aria-current={active ? 'page' : undefined}
						class="flex items-center gap-2 rounded px-3 py-2 text-sm {active
							? 'bg-gray-100 font-medium text-gray-900 dark:bg-gray-800 dark:text-white'
							: 'text-gray-600 hover:bg-gray-100 dark:text-gray-300 dark:hover:bg-gray-800'}"
					>
						<Icon class="h-4 w-4 shrink-0" />
						{label}
					</a>
				{:else}
					<span
						class="flex items-center gap-2 rounded px-3 py-2 text-sm text-gray-700 dark:text-gray-300"
						title="/(no project)"
					>
						<Icon class="h-4 w-4 shrink-0" />
						{label}
					</span>
				{/if}
			{/each}
			<div class="my-1 border-t border-gray-100 dark:border-gray-800"></div>
			{#if allBeadsHref}
				{@const active = $page.url.pathname.startsWith(allBeadsHref)}
				<a
					href={allBeadsHref}
					aria-current={active ? 'page' : undefined}
					class="flex items-center gap-2 rounded px-3 py-2 text-sm {active
						? 'bg-gray-100 font-medium text-gray-900 dark:bg-gray-800 dark:text-white'
						: 'text-gray-600 hover:bg-gray-100 dark:text-gray-300 dark:hover:bg-gray-800'}"
				>
					<Layers class="h-4 w-4 shrink-0" />
					All Beads
				</a>
			{:else}
				<span
					class="flex items-center gap-2 rounded px-3 py-2 text-sm text-gray-700 dark:text-gray-300"
				>
					<Layers class="h-4 w-4 shrink-0" />
					All Beads
				</span>
			{/if}
			{#if providersHref}
				{@const active = $page.url.pathname.startsWith(providersHref)}
				<a
					href={providersHref}
					aria-current={active ? 'page' : undefined}
					class="flex items-center gap-2 rounded px-3 py-2 text-sm {active
						? 'bg-gray-100 font-medium text-gray-900 dark:bg-gray-800 dark:text-white'
						: 'text-gray-600 hover:bg-gray-100 dark:text-gray-300 dark:hover:bg-gray-800'}"
				>
					<Radio class="h-4 w-4 shrink-0" />
					Providers
				</a>
			{:else}
				<span
					class="flex items-center gap-2 rounded px-3 py-2 text-sm text-gray-700 dark:text-gray-300"
				>
					<Radio class="h-4 w-4 shrink-0" />
					Providers
				</span>
			{/if}
		</nav>

		<!-- Page content -->
		<main class="min-w-0 flex-1 overflow-auto p-6">
			{@render children()}
		</main>
	</div>
</div>
