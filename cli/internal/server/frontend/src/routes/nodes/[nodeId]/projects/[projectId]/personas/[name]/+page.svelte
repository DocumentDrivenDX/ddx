<script lang="ts">
	import type { PageData } from './$types';
	import PersonasView from '../PersonasView.svelte';

	let { data }: { data: PageData } = $props();
</script>

<PersonasView {data} />
