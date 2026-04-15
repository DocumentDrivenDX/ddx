<script lang="ts">
	import './layout.css';
	import favicon from '$lib/assets/favicon.svg';
	import { ModeWatcher } from 'mode-watcher';
	import NavShell from '$lib/components/NavShell.svelte';

	let { children } = $props();
</script>

<ModeWatcher />
<svelte:head><link rel="icon" href={favicon} /></svelte:head>

<NavShell>
	{@render children()}
</NavShell>
