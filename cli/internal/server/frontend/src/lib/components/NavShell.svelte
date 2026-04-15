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
		Sun
	} from 'lucide-svelte';
	import { toggleMode, mode } from '$lib/theme';
	import ProjectPicker from './ProjectPicker.svelte';
	import { nodeStore } from '$lib/stores/node.svelte';
	import { projectStore } from '$lib/stores/project.svelte';

	let { children } = $props();

	const pages = [
		{ page: 'beads', label: 'Beads', Icon: LayoutDashboard },
		{ page: 'documents', label: 'Documents', Icon: FileText },
		{ page: 'graph', label: 'Graph', Icon: GitBranch },
		{ page: 'workers', label: 'Workers', Icon: Cpu },
		{ page: 'sessions', label: 'Sessions', Icon: Terminal },
		{ page: 'personas', label: 'Personas', Icon: Users },
		{ page: 'commits', label: 'Commits', Icon: GitCommit }
	];

	const navLinks = $derived(
		pages.map(({ page, label, Icon }) => {
			const nodeId = nodeStore.value?.id;
			const projectId = projectStore.value?.id;
			const href =
				nodeId && projectId ? `/nodes/${nodeId}/projects/${projectId}/${page}` : null;
			return { href, label, Icon };
		})
	);

	const nodeName = $derived(nodeStore.value?.name ?? 'localhost');
</script>

<div class="flex h-screen flex-col bg-white dark:bg-gray-950">
	<!-- Top nav -->
	<header
		class="flex shrink-0 items-center gap-4 border-b border-gray-200 px-4 py-2 dark:border-gray-800 dark:bg-gray-900"
	>
		<span class="text-lg font-semibold tracking-tight dark:text-white">DDx</span>
		<span class="text-xs text-gray-400 dark:text-gray-500">Node: {nodeName}</span>
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

	<div class="flex min-h-0 flex-1">
		<!-- Sidebar -->
		<nav
			class="flex w-48 shrink-0 flex-col gap-1 border-r border-gray-200 p-2 dark:border-gray-800 dark:bg-gray-900"
		>
			{#each navLinks as { href, label, Icon }}
				{#if href}
					<a
						{href}
						class="flex items-center gap-2 rounded px-3 py-2 text-sm text-gray-600 hover:bg-gray-100 dark:text-gray-300 dark:hover:bg-gray-800"
					>
						<Icon class="h-4 w-4 shrink-0" />
						{label}
					</a>
				{:else}
					<span
						class="flex items-center gap-2 rounded px-3 py-2 text-sm text-gray-400 dark:text-gray-600"
						title="/(no project)"
					>
						<Icon class="h-4 w-4 shrink-0" />
						{label}
					</span>
				{/if}
			{/each}
		</nav>

		<!-- Page content -->
		<main class="min-w-0 flex-1 overflow-auto p-6">
			{@render children()}
		</main>
	</div>
</div>
