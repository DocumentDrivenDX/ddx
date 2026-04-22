<script lang="ts">
	import { goto } from '$app/navigation';
	import { resolve } from '$app/paths';
	import { page } from '$app/stores';
	import { onMount } from 'svelte';

	onMount(() => {
		let paletteOpened = false;
		const cancelForPalette = () => {
			paletteOpened = true;
		};
		const target = resolve('/nodes/[nodeId]/projects/[projectId]/beads', {
			nodeId: $page.params.nodeId!,
			projectId: $page.params.projectId!
		});
		const timer = window.setTimeout(() => {
			window.removeEventListener('ddx-command-palette-open', cancelForPalette);
			if (!paletteOpened) {
				goto(target, { replaceState: true });
			}
		}, 250);

		window.addEventListener('ddx-command-palette-open', cancelForPalette, { once: true });

		return () => {
			window.clearTimeout(timer);
			window.removeEventListener('ddx-command-palette-open', cancelForPalette);
		};
	});
</script>
