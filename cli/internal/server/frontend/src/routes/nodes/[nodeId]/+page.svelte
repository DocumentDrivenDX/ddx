<script lang="ts">
	import { nodeStore } from '$lib/stores/node.svelte';
</script>

{#if nodeStore.value}
	<p>Node: {nodeStore.value.name} ({nodeStore.value.id})</p>
{:else}
	<p>Loading...</p>
{/if}
