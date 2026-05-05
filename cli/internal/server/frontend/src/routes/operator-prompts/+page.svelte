<script lang="ts">
	import type { PageData } from './$types';
	import OperatorPromptPanel from '$lib/components/OperatorPromptPanel.svelte';

	let { data }: { data: PageData } = $props();
</script>

<svelte:head>
	<title>Operator prompts | DDx</title>
</svelte:head>

<div class="space-y-6">
	<header
		class="border-border-line from-bg-surface to-bg-canvas dark:border-dark-border-line dark:from-dark-bg-surface dark:to-dark-bg-canvas rounded-2xl border bg-gradient-to-br p-6 shadow-sm"
	>
		<div class="max-w-3xl space-y-3">
			<p
				class="text-accent-lever dark:text-dark-accent-lever text-xs font-semibold tracking-[0.22em] uppercase"
			>
				Operator prompts
			</p>
			<h1 class="text-headline-lg text-fg-ink dark:text-dark-fg-ink font-bold tracking-tight">
				Compose a prompt, preview the bead, and approve it when it looks right.
			</h1>
			<p class="text-fg-muted dark:text-dark-fg-muted max-w-2xl text-sm leading-6">
				This page submits operator prompts as proposed beads, shows the exact bead payload before
				approval, and keeps the latest 10 operator-prompt beads in view.
			</p>
			<div class="text-fg-muted dark:text-dark-fg-muted flex flex-wrap gap-3 text-xs">
				<span class="border-border-line dark:border-dark-border-line rounded-full border px-3 py-1">
					Project: {data.project.name}
				</span>
				<span class="border-border-line dark:border-dark-border-line rounded-full border px-3 py-1">
					Recent prompts capped at 10
				</span>
			</div>
		</div>
	</header>

	<OperatorPromptPanel projectId={data.project.id} nodeId={data.nodeInfo.id} />
</div>
