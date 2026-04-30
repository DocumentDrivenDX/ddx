<script lang="ts">
	import type { Snippet } from 'svelte';

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

	let open = $state(false);
	let timeout: number | null = null;
	const tooltipId = `tooltip-${Math.random().toString(36).slice(2)}`;

	function show() {
		if (disabled) return;
		if (timeout) window.clearTimeout(timeout);
		timeout = window.setTimeout(() => {
			open = true;
			timeout = null;
		}, delayDuration);
	}

	function hide() {
		if (timeout) {
			window.clearTimeout(timeout);
			timeout = null;
		}
		open = false;
	}

	function sideClass(): string {
		switch (side) {
			case 'right':
				return 'top-1/2 left-full ml-2 -translate-y-1/2';
			case 'bottom':
				return 'top-full mt-2';
			case 'left':
				return 'top-1/2 right-full mr-2 -translate-y-1/2';
			default:
				return 'bottom-full mb-2';
		}
	}

	function alignClass(): string {
		if (side === 'left' || side === 'right') {
			return 'translate-x-0';
		}
		switch (align) {
			case 'start':
				return 'left-0';
			case 'end':
				return 'right-0';
			default:
				return 'left-1/2 -translate-x-1/2';
		}
	}
</script>

<span
	role="presentation"
	class="relative inline-flex max-w-full align-middle"
	data-disabled-trigger={disabledTrigger ? '' : undefined}
	aria-describedby={open ? tooltipId : undefined}
	onpointerenter={show}
	onpointerleave={hide}
	onfocusin={show}
	onfocusout={hide}
>
	{@render children?.()}
	{#if open}
		<span
			id={tooltipId}
			role="tooltip"
			class="absolute z-50 max-w-xs rounded-none bg-fg-ink px-2.5 py-1.5 text-xs leading-5 font-medium whitespace-nowrap text-white shadow-lg shadow-fg-ink/20 dark:bg-dark-fg-ink dark:text-fg-ink dark:shadow-black/30 {sideClass()} {alignClass()}"
		>
			{#if tooltip}
				{@render tooltip()}
			{:else}
				{content}
			{/if}
		</span>
	{/if}
</span>
