<script lang="ts">
	import type { PageData } from './$types';
	import { page } from '$app/stores';
	import { replaceState } from '$app/navigation';
	import { browser } from '$app/environment';
	import { RunRowDetail } from '$lib/runDetail';

	type Tab = 'overview' | 'prompt' | 'response' | 'tools' | 'session' | 'evidence';
	const VALID_TABS: Tab[] = ['overview', 'prompt', 'response', 'tools', 'session', 'evidence'];

	let { data }: { data: PageData } = $props();

	let run = $derived(data.run);

	function runsListHref(): string {
		return `/nodes/${data.nodeId}/projects/${data.projectId}/runs`;
	}

	function parentRunHref(parentId: string): string {
		return `/nodes/${data.nodeId}/projects/${data.projectId}/runs/${parentId}`;
	}

	function artifactHref(artifactId: string): string {
		return `/nodes/${data.nodeId}/projects/${data.projectId}/artifacts/${encodeURIComponent(artifactId)}`;
	}

	function layerBadgeClass(layer: string): string {
		switch (layer) {
			case 'work':
				return 'badge-layer-work';
			case 'try':
				return 'badge-layer-try';
			case 'run':
				return 'badge-layer-run';
			default:
				return 'badge-status-neutral';
		}
	}

	function statusBadgeClass(status: string): string {
		switch (status) {
			case 'success':
				return 'badge-status-closed';
			case 'failure':
				return 'badge-status-failed';
			case 'running':
				return 'badge-status-running';
			case 'preserved':
				return 'badge-status-in-progress';
			default:
				return 'badge-status-open';
		}
	}

	function breadcrumbs(): Array<{ label: string; href: string }> {
		if (!run) return [];
		const crumbs: Array<{ label: string; href: string }> = [
			{ label: 'Runs', href: runsListHref() }
		];
		if (run.layer === 'run' && data.grandparentRunId && run.parentRunId) {
			crumbs.push({ label: data.grandparentRunId, href: parentRunHref(data.grandparentRunId) });
			crumbs.push({ label: run.parentRunId, href: parentRunHref(run.parentRunId) });
		} else if (run.parentRunId) {
			crumbs.push({ label: run.parentRunId, href: parentRunHref(run.parentRunId) });
		}
		crumbs.push({ label: run.id, href: '#' });
		return crumbs;
	}

	let activeTab = $derived.by<Tab>(() => {
		const t = $page.url.searchParams.get('tab');
		return VALID_TABS.includes(t as Tab) ? (t as Tab) : 'overview';
	});

	function handleTabChange(tab: Tab) {
		if (!browser) return;
		const url = new URL($page.url);
		if (tab === 'overview') {
			url.searchParams.delete('tab');
		} else {
			url.searchParams.set('tab', tab);
		}
		replaceState(url, $page.state);
	}
</script>

<div class="space-y-4">
	<!-- Breadcrumbs -->
	<nav class="text-fg-muted dark:text-dark-fg-muted flex items-center gap-1 text-xs">
		{#each breadcrumbs() as crumb, i}
			{#if i > 0}
				<span>/</span>
			{/if}
			{#if crumb.href === '#'}
				<span class="font-mono-code text-fg-ink dark:text-dark-fg-ink">{crumb.label}</span>
			{:else}
				<a
					href={crumb.href}
					class="font-mono-code hover:text-accent-lever dark:hover:text-dark-accent-lever"
				>
					{crumb.label}
				</a>
			{/if}
		{/each}
	</nav>

	{#if !run}
		<p class="text-body-sm text-fg-muted dark:text-dark-fg-muted">Run not found.</p>
	{:else}
		<!-- Header -->
		<div class="flex items-center gap-3">
			<h1 class="font-mono-code text-fg-ink dark:text-dark-fg-ink text-lg">{run.id}</h1>
			<span
				class="font-label-caps text-label-caps inline-block rounded-full px-2 py-0.5 uppercase {layerBadgeClass(
					run.layer
				)}"
			>
				{run.layer}
			</span>
			<span
				class="font-mono-code text-mono-code inline-block border px-1.5 py-0.5 uppercase {statusBadgeClass(
					run.status
				)}"
			>
				{run.status}
			</span>
		</div>

		{#if data.producedArtifact}
			<div
				data-testid="produced-artifact"
				class="border-border-line bg-bg-surface dark:border-dark-border-line dark:bg-dark-bg-surface border p-3"
			>
				<div
					class="font-label-caps text-label-caps text-fg-muted dark:text-dark-fg-muted mb-1 uppercase"
				>
					Produced artifact
				</div>
				<a
					href={artifactHref(data.producedArtifact.id)}
					class="text-mono-code text-accent-lever dark:text-dark-accent-lever hover:underline"
				>
					{data.producedArtifact.title}
				</a>
			</div>
		{/if}

		<RunRowDetail
			runId={run.id}
			layer={run.layer}
			nodeId={data.nodeId}
			projectId={data.projectId}
			initialTab={activeTab}
			onTabChange={handleTabChange}
		/>
	{/if}
</div>
