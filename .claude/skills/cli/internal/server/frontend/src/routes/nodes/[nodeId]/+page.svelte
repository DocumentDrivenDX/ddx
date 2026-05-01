<script lang="ts">
	import { nodeStore } from '$lib/stores/node.svelte';
</script>

{#if nodeStore.value}
	<div class="space-y-2">
		<h1 class="text-xl font-semibold dark:text-white">{nodeStore.value.name}</h1>
		<p class="text-sm text-gray-600 dark:text-gray-300">Node ID: {nodeStore.value.id}</p>
	</div>
{:else}
	<div class="space-y-2">
		<h1 class="text-xl font-semibold dark:text-white">Node</h1>
		<p class="text-sm text-gray-600 dark:text-gray-300">Loading...</p>
	</div>
{/if}
