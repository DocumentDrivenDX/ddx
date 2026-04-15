<script lang="ts">
	import './layout.css';
	import favicon from '$lib/assets/favicon.svg';
	import { ModeWatcher } from 'mode-watcher';
	import NavShell from '$lib/components/NavShell.svelte';
	import { nodeStore } from '$lib/stores/node.svelte';

	let { data, children } = $props();

	// Re-derive on every data change so SvelteKit navigations and Houdini
	// refetches both re-fire the subscription below.
	const NodeInfoStore = $derived(data.NodeInfo);

	$effect(() => {
		const store = NodeInfoStore;
		if (!store) return;
		return store.subscribe((val) => {
			const n = val?.data?.nodeInfo;
			if (n) nodeStore.set(n);
		});
	});
</script>

<ModeWatcher />
<svelte:head><link rel="icon" href={favicon} /></svelte:head>

<NavShell>
	{@render children()}
</NavShell>
