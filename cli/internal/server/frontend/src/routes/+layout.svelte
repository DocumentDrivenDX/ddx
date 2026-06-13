<script lang="ts">
	import '../app.css';
	import favicon from '$lib/assets/favicon.svg';
	import { ModeWatcher } from 'mode-watcher';
	import CommandPalette from '$lib/components/CommandPalette.svelte';
	import NavShell from '$lib/components/NavShell.svelte';
	import { nodeStore } from '$lib/stores/node.svelte';
	import { page } from '$app/stores';

	let { data, children } = $props();

	$effect(() => {
		if (data.nodeInfo) nodeStore.set(data.nodeInfo);
	});

	// Node-wide workers workbench: full-screen layout without NavShell
	// so project-picker options don't conflict with worker table project badges.
	const isWorkersPanePath = $derived(/^\/nodes\/[^/]+\/workers(\/|$)/.test($page.url.pathname));
</script>

<ModeWatcher />
<svelte:head><link rel="icon" href={favicon} /></svelte:head>

{#if isWorkersPanePath}
	{@render children()}
{:else}
	<NavShell>
		{@render children()}
	</NavShell>
{/if}
<CommandPalette />
