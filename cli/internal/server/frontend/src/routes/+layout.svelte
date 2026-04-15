<script lang="ts">
	import './layout.css';
	import favicon from '$lib/assets/favicon.svg';
	import { ModeWatcher } from 'mode-watcher';
	import NavShell from '$lib/components/NavShell.svelte';
	import { nodeStore } from '$lib/stores/node.svelte';

	let { data, children } = $props();
	const NodeInfo = data.NodeInfo;

	$effect(() => {
		const n = $NodeInfo?.data?.nodeInfo;
		if (n) nodeStore.set(n);
	});
</script>

<ModeWatcher />
<svelte:head><link rel="icon" href={favicon} /></svelte:head>

<NavShell>
	{@render children()}
</NavShell>
