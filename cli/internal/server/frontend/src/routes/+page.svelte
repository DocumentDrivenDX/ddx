<script lang="ts">
	import { goto } from '$app/navigation';
	import { nodeStore } from '$lib/stores/node.svelte';

	$effect(() => {
		if (nodeStore.value?.id) {
			goto(`/nodes/${nodeStore.value.id}`, { replaceState: true });
		}
	});
</script>
