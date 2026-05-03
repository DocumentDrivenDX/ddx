<script lang="ts">
	import {
		LayoutDashboard,
		Archive,
		GitBranch,
		Cpu,
		Users,
		GitCommit,
		Package,
		Moon,
		Sun,
		Radio,
		Layers,
		BarChart3,
		Home,
		Activity,
		Network
	} from 'lucide-svelte';
	import { page } from '$app/stores';
	import { toggleMode, mode } from '$lib/theme';
	import ProjectPicker from './ProjectPicker.svelte';
	import NodePicker from './NodePicker.svelte';
	import DrainIndicator from './DrainIndicator.svelte';
	import { nodeStore } from '$lib/stores/node.svelte';
	import { projectStore } from '$lib/stores/project.svelte';
	import { wsConnection } from '$lib/stores/connection.svelte';

	let { children } = $props();

	const pages = [
		{ page: '', label: 'Overview', Icon: Home, exact: true },
		{ page: 'beads', label: 'Beads', Icon: LayoutDashboard },
		{ page: 'artifacts', label: 'Artifacts', Icon: Archive },
		{ page: 'graph', label: 'Graph', Icon: GitBranch },
		{ page: 'runs', label: 'Runs', Icon: Activity },
		{ page: 'workers', label: 'Workers', Icon: Cpu },
		{ page: 'personas', label: 'Personas', Icon: Users },
		{ page: 'plugins', label: 'Plugins', Icon: Package },
		{ page: 'commits', label: 'Commits', Icon: GitCommit },
		{ page: 'efficacy', label: 'Efficacy', Icon: BarChart3 }
	];

	const navLinks = $derived(
		pages.map(({ page, label, Icon, exact }) => {
			const nodeId = nodeStore.value?.id;
			const projectId = projectStore.value?.id;
			const base = nodeId && projectId ? `/nodes/${nodeId}/projects/${projectId}` : null;
			const href = base ? (page ? `${base}/${page}` : base) : null;
			return { href, label, Icon, exact: Boolean(exact) };
		})
	);

	const brandHref = $derived(
		nodeStore.value?.id && projectStore.value?.id
			? `/nodes/${nodeStore.value.id}/projects/${projectStore.value.id}`
			: '/'
	);

	const allBeadsHref = $derived(nodeStore.value?.id ? `/nodes/${nodeStore.value.id}/beads` : null);

	const allRunsHref = $derived(nodeStore.value?.id ? `/nodes/${nodeStore.value.id}/runs` : null);

	const providersHref = $derived(
		nodeStore.value?.id ? `/nodes/${nodeStore.value.id}/providers` : null
	);

	const federationHref = '/federation';
	const fedActive = $derived($page.url.pathname.startsWith(federationHref));

	const nodeName = $derived(nodeStore.value?.name ?? 'localhost');
</script>

<div class="flex h-screen flex-col bg-bg-canvas dark:bg-dark-bg-canvas">
	<!-- Top nav -->
	<header
		class="flex h-10 shrink-0 items-center gap-4 border-b border-border-line bg-bg-surface px-3 dark:border-dark-border-line dark:bg-dark-bg-surface"
	>
		<a
			href={brandHref}
			class="font-mono-code text-headline-md font-black tracking-tighter text-fg-ink hover:text-accent-lever dark:text-dark-fg-ink dark:hover:text-dark-accent-lever"
		>
			DDx
		</a>
		<span class="font-mono-code text-body-sm text-fg-muted dark:text-dark-fg-muted">Node: {nodeName}</span>
		<div class="mx-1 h-4 w-px bg-border-line dark:bg-dark-border-line"></div>
		<ProjectPicker />
		<NodePicker />
		<div class="ml-auto flex items-center gap-2">
			<DrainIndicator />
			<button
				onclick={toggleMode}
				class="p-1.5 text-fg-muted hover:bg-bg-canvas dark:text-dark-fg-muted dark:hover:bg-dark-bg-elevated"
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
			class="flex shrink-0 items-center gap-2 border-b border-accent-load/40 bg-accent-load/10 px-4 py-1 font-label-caps text-label-caps text-accent-load dark:border-dark-accent-load/40 dark:bg-dark-accent-load/10 dark:text-dark-accent-load"
		>
			<span class="inline-block h-2 w-2 rounded-full bg-yellow-500"></span>
			{wsConnection.state === 'connecting' ? 'reconnecting\u2026' : 'disconnected'}
		</div>
	{/if}

	<div class="flex min-h-0 flex-1">
		<!-- Sidebar -->
		<nav
			class="flex w-64 shrink-0 flex-col gap-px border-r border-border-line bg-bg-surface py-3 dark:border-dark-border-line dark:bg-dark-bg-surface"
		>
			{#each navLinks as { href, label, Icon, exact }}
				{#if href}
					{@const active = exact
						? $page.url.pathname === href || $page.url.pathname === href + '/'
						: $page.url.pathname.startsWith(href)}
					<a
						{href}
						aria-current={active ? 'page' : undefined}
						class="flex items-center gap-3 border-l-2 px-4 py-2.5 font-body-md text-body-sm {active
							? 'border-accent-lever bg-bg-canvas font-bold text-fg-ink dark:border-dark-accent-lever dark:bg-dark-bg-canvas dark:text-dark-fg-ink'
							: 'border-transparent text-fg-muted hover:bg-bg-canvas hover:text-fg-ink dark:text-dark-fg-muted dark:hover:bg-dark-bg-canvas dark:hover:text-dark-fg-ink'}"
					>
						<Icon class="h-[18px] w-[18px] shrink-0" />
						{label}
					</a>
				{:else}
					<span
						class="flex items-center gap-3 border-l-2 border-transparent px-4 py-2.5 font-body-md text-body-sm text-fg-muted/50 dark:text-dark-fg-muted/50"
						title="/(no project)"
					>
						<Icon class="h-[18px] w-[18px] shrink-0" />
						{label}
					</span>
				{/if}
			{/each}
			<div class="my-2 border-t border-border-line dark:border-dark-border-line"></div>
			{#if allBeadsHref}
				{@const active = $page.url.pathname.startsWith(allBeadsHref)}
				<a
					href={allBeadsHref}
					aria-current={active ? 'page' : undefined}
					class="flex items-center gap-3 border-l-2 px-4 py-2.5 font-body-md text-body-sm {active
						? 'border-accent-lever bg-bg-canvas font-bold text-fg-ink dark:border-dark-accent-lever dark:bg-dark-bg-canvas dark:text-dark-fg-ink'
						: 'border-transparent text-fg-muted hover:bg-bg-canvas hover:text-fg-ink dark:text-dark-fg-muted dark:hover:bg-dark-bg-canvas dark:hover:text-dark-fg-ink'}"
				>
					<Layers class="h-4 w-4 shrink-0" />
					All Beads
				</a>
			{:else}
				<span
					class="flex items-center gap-3 border-l-2 border-transparent px-4 py-2.5 font-body-md text-body-sm text-fg-muted/50 dark:text-dark-fg-muted/50"
				>
					<Layers class="h-4 w-4 shrink-0" />
					All Beads
				</span>
			{/if}
			{#if allRunsHref}
				{@const active = $page.url.pathname.startsWith(allRunsHref)}
				<a
					href={allRunsHref}
					aria-current={active ? 'page' : undefined}
					class="flex items-center gap-3 border-l-2 px-4 py-2.5 font-body-md text-body-sm {active
						? 'border-accent-lever bg-bg-canvas font-bold text-fg-ink dark:border-dark-accent-lever dark:bg-dark-bg-canvas dark:text-dark-fg-ink'
						: 'border-transparent text-fg-muted hover:bg-bg-canvas hover:text-fg-ink dark:text-dark-fg-muted dark:hover:bg-dark-bg-canvas dark:hover:text-dark-fg-ink'}"
				>
					<Activity class="h-4 w-4 shrink-0" />
					All Runs
				</a>
			{:else}
				<span
					class="flex items-center gap-3 border-l-2 border-transparent px-4 py-2.5 font-body-md text-body-sm text-fg-muted/50 dark:text-dark-fg-muted/50"
				>
					<Activity class="h-4 w-4 shrink-0" />
					All Runs
				</span>
			{/if}
			<a
				href={federationHref}
				aria-current={fedActive ? 'page' : undefined}
				data-testid="nav-federation"
				class="flex items-center gap-3 border-l-2 px-4 py-2.5 font-body-md text-body-sm {fedActive
					? 'border-accent-lever bg-bg-canvas font-bold text-fg-ink dark:border-dark-accent-lever dark:bg-dark-bg-canvas dark:text-dark-fg-ink'
					: 'border-transparent text-fg-muted hover:bg-bg-canvas hover:text-fg-ink dark:text-dark-fg-muted dark:hover:bg-dark-bg-canvas dark:hover:text-dark-fg-ink'}"
			>
				<Network class="h-4 w-4 shrink-0" />
				Federation
			</a>
			{#if providersHref}
				{@const active = $page.url.pathname.startsWith(providersHref)}
				<a
					href={providersHref}
					aria-current={active ? 'page' : undefined}
					class="flex items-center gap-3 border-l-2 px-4 py-2.5 font-body-md text-body-sm {active
						? 'border-accent-lever bg-bg-canvas font-bold text-fg-ink dark:border-dark-accent-lever dark:bg-dark-bg-canvas dark:text-dark-fg-ink'
						: 'border-transparent text-fg-muted hover:bg-bg-canvas hover:text-fg-ink dark:text-dark-fg-muted dark:hover:bg-dark-bg-canvas dark:hover:text-dark-fg-ink'}"
				>
					<Radio class="h-4 w-4 shrink-0" />
					Agent availability
				</a>
			{:else}
				<span
					class="flex items-center gap-3 border-l-2 border-transparent px-4 py-2.5 font-body-md text-body-sm text-fg-muted/50 dark:text-dark-fg-muted/50"
				>
					<Radio class="h-4 w-4 shrink-0" />
					Agent availability
				</span>
			{/if}
		</nav>

		<!-- Page content -->
		<main class="min-w-0 flex-1 overflow-auto bg-bg-canvas p-gutter dark:bg-dark-bg-canvas">
			{@render children()}
		</main>
	</div>
</div>
