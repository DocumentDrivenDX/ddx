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
		Network,
		MessageSquare
	} from 'lucide-svelte';
	import { page } from '$app/stores';
	import { toggleMode, mode } from '$lib/theme';
	import ProjectPicker from './ProjectPicker.svelte';
	import NodePicker from './NodePicker.svelte';
	import DrainIndicator from './DrainIndicator.svelte';
	import { nodeStore } from '$lib/stores/node.svelte';
	import { projectStore } from '$lib/stores/project.svelte';
	import { wsConnection } from '$lib/stores/connection.svelte';
	import { projectNavPages } from '$lib/routing/shellRoutes';

	let { children } = $props();

	const iconsByPage = {
		'': Home,
		beads: LayoutDashboard,
		artifacts: Archive,
		graph: GitBranch,
		runs: Activity,
		workers: Cpu,
		personas: Users,
		plugins: Package,
		commits: GitCommit,
		efficacy: BarChart3
	};

	const navLinks = $derived(
		projectNavPages.map((navPage) => {
			const { page, label } = navPage;
			const nodeId = nodeStore.value?.id;
			const projectId = projectStore.value?.id;
			const base = nodeId && projectId ? `/nodes/${nodeId}/projects/${projectId}` : null;
			const href = base ? (page ? `${base}/${page}` : base) : null;
			const Icon = iconsByPage[page];
			return { href, label, Icon, exact: 'exact' in navPage && Boolean(navPage.exact) };
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

	const operatorPromptsHref = '/operator-prompts';
	const operatorPromptsActive = $derived($page.url.pathname.startsWith(operatorPromptsHref));

	const federationHref = '/federation';
	const fedActive = $derived($page.url.pathname.startsWith(federationHref));

	const nodeName = $derived(nodeStore.value?.name ?? 'localhost');
</script>

<div class="bg-bg-canvas dark:bg-dark-bg-canvas flex h-screen flex-col">
	<!-- Top nav -->
	<header
		class="border-border-line bg-bg-surface dark:border-dark-border-line dark:bg-dark-bg-surface flex h-10 shrink-0 items-center gap-4 border-b px-3"
	>
		<a
			href={brandHref}
			class="font-mono-code text-headline-md text-fg-ink hover:text-accent-lever dark:text-dark-fg-ink dark:hover:text-dark-accent-lever font-black tracking-tighter"
		>
			DDx
		</a>
		<span class="font-mono-code text-body-sm text-fg-muted dark:text-dark-fg-muted"
			>Node: {nodeName}</span
		>
		<div class="bg-border-line dark:bg-dark-border-line mx-1 h-4 w-px"></div>
		<ProjectPicker />
		<NodePicker />
		<div class="ml-auto flex items-center gap-2">
			<DrainIndicator />
			<button
				onclick={toggleMode}
				class="text-fg-muted hover:bg-bg-canvas dark:text-dark-fg-muted dark:hover:bg-dark-bg-elevated p-1.5"
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
			class="border-accent-load/40 bg-accent-load/10 font-label-caps text-label-caps text-accent-load dark:border-dark-accent-load/40 dark:bg-dark-accent-load/10 dark:text-dark-accent-load flex shrink-0 items-center gap-2 border-b px-4 py-1"
		>
			<span class="inline-block h-2 w-2 rounded-full bg-yellow-500"></span>
			{wsConnection.state === 'connecting' ? 'reconnecting\u2026' : 'disconnected'}
		</div>
	{/if}

	<div class="flex min-h-0 flex-1">
		<!-- Sidebar -->
		<nav
			class="border-border-line bg-bg-surface dark:border-dark-border-line dark:bg-dark-bg-surface flex w-64 shrink-0 flex-col gap-px border-r py-3"
		>
			{#each navLinks as { href, label, Icon, exact }}
				{#if href}
					{@const active = exact
						? $page.url.pathname === href || $page.url.pathname === href + '/'
						: $page.url.pathname.startsWith(href)}
					<a
						{href}
						aria-current={active ? 'page' : undefined}
						class="font-body-md text-body-sm flex items-center gap-3 border-l-2 px-4 py-2.5 {active
							? 'border-accent-lever bg-bg-canvas text-fg-ink dark:border-dark-accent-lever dark:bg-dark-bg-canvas dark:text-dark-fg-ink font-bold'
							: 'text-fg-muted hover:bg-bg-canvas hover:text-fg-ink dark:text-dark-fg-muted dark:hover:bg-dark-bg-canvas dark:hover:text-dark-fg-ink border-transparent'}"
					>
						<Icon class="h-[18px] w-[18px] shrink-0" />
						{label}
					</a>
				{:else}
					<span
						class="font-body-md text-body-sm text-fg-muted/50 dark:text-dark-fg-muted/50 flex items-center gap-3 border-l-2 border-transparent px-4 py-2.5"
						title="/(no project)"
					>
						<Icon class="h-[18px] w-[18px] shrink-0" />
						{label}
					</span>
				{/if}
			{/each}
			<div class="border-border-line dark:border-dark-border-line my-2 border-t"></div>
			{#if allBeadsHref}
				{@const active = $page.url.pathname.startsWith(allBeadsHref)}
				<a
					href={allBeadsHref}
					aria-current={active ? 'page' : undefined}
					class="font-body-md text-body-sm flex items-center gap-3 border-l-2 px-4 py-2.5 {active
						? 'border-accent-lever bg-bg-canvas text-fg-ink dark:border-dark-accent-lever dark:bg-dark-bg-canvas dark:text-dark-fg-ink font-bold'
						: 'text-fg-muted hover:bg-bg-canvas hover:text-fg-ink dark:text-dark-fg-muted dark:hover:bg-dark-bg-canvas dark:hover:text-dark-fg-ink border-transparent'}"
				>
					<Layers class="h-4 w-4 shrink-0" />
					All Beads
				</a>
			{:else}
				<span
					class="font-body-md text-body-sm text-fg-muted/50 dark:text-dark-fg-muted/50 flex items-center gap-3 border-l-2 border-transparent px-4 py-2.5"
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
					class="font-body-md text-body-sm flex items-center gap-3 border-l-2 px-4 py-2.5 {active
						? 'border-accent-lever bg-bg-canvas text-fg-ink dark:border-dark-accent-lever dark:bg-dark-bg-canvas dark:text-dark-fg-ink font-bold'
						: 'text-fg-muted hover:bg-bg-canvas hover:text-fg-ink dark:text-dark-fg-muted dark:hover:bg-dark-bg-canvas dark:hover:text-dark-fg-ink border-transparent'}"
				>
					<Activity class="h-4 w-4 shrink-0" />
					All Runs
				</a>
			{:else}
				<span
					class="font-body-md text-body-sm text-fg-muted/50 dark:text-dark-fg-muted/50 flex items-center gap-3 border-l-2 border-transparent px-4 py-2.5"
				>
					<Activity class="h-4 w-4 shrink-0" />
					All Runs
				</span>
			{/if}
			<a
				href={operatorPromptsHref}
				aria-current={operatorPromptsActive ? 'page' : undefined}
				class="font-body-md text-body-sm flex items-center gap-3 border-l-2 px-4 py-2.5 {operatorPromptsActive
					? 'border-accent-lever bg-bg-canvas text-fg-ink dark:border-dark-accent-lever dark:bg-dark-bg-canvas dark:text-dark-fg-ink font-bold'
					: 'text-fg-muted hover:bg-bg-canvas hover:text-fg-ink dark:text-dark-fg-muted dark:hover:bg-dark-bg-canvas dark:hover:text-dark-fg-ink border-transparent'}"
			>
				<MessageSquare class="h-4 w-4 shrink-0" />
				Operator prompts
			</a>
			<a
				href={federationHref}
				aria-current={fedActive ? 'page' : undefined}
				data-testid="nav-federation"
				class="font-body-md text-body-sm flex items-center gap-3 border-l-2 px-4 py-2.5 {fedActive
					? 'border-accent-lever bg-bg-canvas text-fg-ink dark:border-dark-accent-lever dark:bg-dark-bg-canvas dark:text-dark-fg-ink font-bold'
					: 'text-fg-muted hover:bg-bg-canvas hover:text-fg-ink dark:text-dark-fg-muted dark:hover:bg-dark-bg-canvas dark:hover:text-dark-fg-ink border-transparent'}"
			>
				<Network class="h-4 w-4 shrink-0" />
				Federation
			</a>
			{#if providersHref}
				{@const active = $page.url.pathname.startsWith(providersHref)}
				<a
					href={providersHref}
					aria-current={active ? 'page' : undefined}
					class="font-body-md text-body-sm flex items-center gap-3 border-l-2 px-4 py-2.5 {active
						? 'border-accent-lever bg-bg-canvas text-fg-ink dark:border-dark-accent-lever dark:bg-dark-bg-canvas dark:text-dark-fg-ink font-bold'
						: 'text-fg-muted hover:bg-bg-canvas hover:text-fg-ink dark:text-dark-fg-muted dark:hover:bg-dark-bg-canvas dark:hover:text-dark-fg-ink border-transparent'}"
				>
					<Radio class="h-4 w-4 shrink-0" />
					Agent availability
				</a>
			{:else}
				<span
					class="font-body-md text-body-sm text-fg-muted/50 dark:text-dark-fg-muted/50 flex items-center gap-3 border-l-2 border-transparent px-4 py-2.5"
				>
					<Radio class="h-4 w-4 shrink-0" />
					Agent availability
				</span>
			{/if}
		</nav>

		<!-- Page content -->
		<main class="bg-bg-canvas p-gutter dark:bg-dark-bg-canvas min-w-0 flex-1 overflow-auto">
			{@render children()}
		</main>
	</div>
</div>
