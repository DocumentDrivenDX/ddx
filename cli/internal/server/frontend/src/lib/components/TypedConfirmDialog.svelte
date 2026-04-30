<script module lang="ts">
	export function typedConfirmMatches(value: string, expectedText: string): boolean {
		return value === expectedText;
	}
</script>

<script lang="ts">
	import type { Snippet } from 'svelte';
	import ConfirmDialog from './ConfirmDialog.svelte';

	interface TypedConfirmDialogProps {
		open?: boolean;
		actionLabel: string;
		expectedText: string;
		title?: string;
		cancelLabel?: string;
		expectedLabel?: string;
		destructive?: boolean;
		confirmDisabled?: boolean;
		returnFocusTo?: HTMLElement | null;
		summary?: Snippet;
		children?: Snippet;
		onConfirm?: () => void | Promise<void>;
		onCancel?: (reason: 'cancel' | 'dismiss' | 'escape') => void;
		onOpenChange?: (open: boolean) => void;
	}

	let {
		open = $bindable(false),
		actionLabel,
		expectedText,
		title = actionLabel,
		cancelLabel = 'Cancel',
		expectedLabel = 'confirmation text',
		destructive = false,
		confirmDisabled = false,
		returnFocusTo = null,
		summary,
		children,
		onConfirm,
		onCancel,
		onOpenChange
	}: TypedConfirmDialogProps = $props();

	let typedText = $state('');
	const inputId = 'typed-confirm-dialog-input';
	const isMatched = $derived(typedConfirmMatches(typedText, expectedText));

	$effect(() => {
		if (open) {
			typedText = '';
		}
	});
</script>

<ConfirmDialog
	bind:open
	{actionLabel}
	{title}
	{cancelLabel}
	{destructive}
	{returnFocusTo}
	confirmDisabled={confirmDisabled || !isMatched}
	{onConfirm}
	{onCancel}
	{onOpenChange}
>
	{#snippet summary()}
		{#if summary}
			<div class="mb-3">
				{@render summary()}
			</div>
		{/if}
		<label for={inputId} class="block text-sm font-medium text-fg-ink dark:text-dark-fg-ink">
			Type the {expectedLabel} to confirm
		</label>
		<input
			id={inputId}
			type="text"
			bind:value={typedText}
			autocomplete="off"
			autocapitalize="off"
			spellcheck="false"
			class="mt-2 w-full rounded-none border border-border-line bg-bg-elevated px-3 py-2 font-mono text-sm text-fg-ink placeholder-fg-muted focus:border-accent-lever focus:ring-1 focus:ring-accent-lever focus:outline-none dark:border-dark-border-line dark:bg-dark-bg-surface dark:text-dark-fg-ink dark:placeholder-dark-fg-muted dark:focus:border-dark-accent-lever"
			placeholder={expectedText}
			aria-describedby="typed-confirm-dialog-expected"
		/>
		<p
			id="typed-confirm-dialog-expected"
			class="mt-2 font-mono text-xs text-fg-muted dark:text-dark-fg-muted"
		>
			{expectedText}
		</p>
	{/snippet}

	{#if children}
		{@render children()}
	{/if}
</ConfirmDialog>
