<script lang="ts">
	import type { PageData } from './$types';
	import { goto } from '$app/navigation';
	import { page } from '$app/stores';
	import BeadDetail from '$lib/components/BeadDetail.svelte';

	let { data }: { data: PageData } = $props();

	function handleClose() {
		const pathParts = $page.url.pathname.split('/');
		pathParts.pop(); // remove beadId segment
		const basePath = pathParts.join('/');
		const searchStr = $page.url.searchParams.toString();
		goto(searchStr ? `${basePath}?${searchStr}` : basePath);
	}
</script>

{#if data.bead}
	<!-- Backdrop -->
	<div
		class="fixed inset-0 z-40 bg-black/20 dark:bg-black/40"
		onclick={handleClose}
		role="button"
		tabindex="-1"
		aria-label="Close panel"
		onkeydown={(e) => e.key === 'Escape' && handleClose()}
	></div>

	<!-- Detail panel -->
	<BeadDetail bead={data.bead} onClose={handleClose} />
{/if}
