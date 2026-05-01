<script lang="ts">
	import { nodeStore } from '$lib/stores/node.svelte';
</script>

{#if nodeStore.value}
	<div class="space-y-3 border-b border-gray-200 pb-6 dark:border-gray-700">
		<p class="text-xs font-semibold tracking-widest text-accent-lever dark:text-dark-accent-lever uppercase">Node</p>
		<h1 class="text-3xl font-bold tracking-tight text-gray-950 dark:text-white">{nodeStore.value.name}</h1>
		<p class="font-mono text-xs text-gray-500 dark:text-gray-400">ID: {nodeStore.value.id}</p>
	</div>
{:else}
	<div class="space-y-3 border-b border-gray-200 pb-6 dark:border-gray-700">
		<p class="text-xs font-semibold tracking-widest text-accent-lever dark:text-dark-accent-lever uppercase">Node</p>
		<h1 class="text-3xl font-bold tracking-tight text-gray-950 dark:text-white">Loading…</h1>
	</div>
{/if}
