<script lang="ts">
	import { nodeStore } from '$lib/stores/node.svelte';
</script>

{#if nodeStore.value}
	<div class="space-y-3 border-b border-gray-200 pb-6 dark:border-gray-700">
		<p class="font-label-caps text-label-caps text-accent-lever dark:text-dark-accent-lever uppercase">Node</p>
		<h1 class="text-3xl font-bold tracking-tight text-gray-950 dark:text-white">{nodeStore.value.name}</h1>
		<p class="font-mono-code text-mono-code text-fg-muted dark:text-dark-fg-muted">ID: {nodeStore.value.id}</p>
	</div>
{:else}
	<div class="space-y-3 border-b border-gray-200 pb-6 dark:border-gray-700">
		<p class="font-label-caps text-label-caps text-accent-lever dark:text-dark-accent-lever uppercase">Node</p>
		<h1 class="text-3xl font-bold tracking-tight text-gray-950 dark:text-white">Loading…</h1>
	</div>
{/if}
