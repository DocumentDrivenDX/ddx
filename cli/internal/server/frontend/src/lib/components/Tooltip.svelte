<script lang="ts">
	import type { Snippet } from 'svelte';
	import { Tooltip as BitsTooltip } from 'bits-ui';

	interface TooltipProps {
		content?: string;
		tooltip?: Snippet;
		children?: Snippet;
		disabled?: boolean;
		disabledTrigger?: boolean;
		delayDuration?: number;
		side?: 'top' | 'right' | 'bottom' | 'left';
		align?: 'start' | 'center' | 'end';
	}

	let {
		content,
		tooltip,
		children,
		disabled = false,
		disabledTrigger = false,
		delayDuration = 250,
		side = 'top',
		align = 'center'
	}: TooltipProps = $props();
</script>

<BitsTooltip.Root {delayDuration} {disabled}>
	<BitsTooltip.Trigger>
		{#snippet child({ props })}
			<span
				{...props}
				class="inline-flex max-w-full align-middle"
				data-disabled-trigger={disabledTrigger ? '' : undefined}
			>
				{@render children?.()}
			</span>
		{/snippet}
	</BitsTooltip.Trigger>
	<BitsTooltip.Portal>
		<BitsTooltip.Content
			{side}
			{align}
			sideOffset={8}
			class="z-50 max-w-xs rounded-md bg-gray-950 px-2.5 py-1.5 text-xs leading-5 font-medium text-white shadow-lg shadow-gray-950/20 dark:bg-gray-100 dark:text-gray-950 dark:shadow-black/30"
		>
			{#if tooltip}
				{@render tooltip()}
			{:else}
				{content}
			{/if}
			<BitsTooltip.Arrow class="fill-gray-950 dark:fill-gray-100" />
		</BitsTooltip.Content>
	</BitsTooltip.Portal>
</BitsTooltip.Root>
