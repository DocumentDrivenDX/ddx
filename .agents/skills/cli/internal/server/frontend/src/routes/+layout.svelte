<script lang="ts">
	import '../app.css';
	import favicon from '$lib/assets/favicon.svg';
	import { ModeWatcher } from 'mode-watcher';
	import CommandPalette from '$lib/components/CommandPalette.svelte';
	import NavShell from '$lib/components/NavShell.svelte';
	import { nodeStore } from '$lib/stores/node.svelte';

	let { data, children } = $props();

	$effect(() => {
		if (data.nodeInfo) nodeStore.set(data.nodeInfo);
	});
</script>

<ModeWatcher />
<svelte:head><link rel="icon" href={favicon} /></svelte:head>

<NavShell>
	{@render children()}
</NavShell>
<CommandPalette />
