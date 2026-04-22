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
		<label for={inputId} class="block text-sm font-medium text-gray-700 dark:text-gray-200">
			Type the {expectedLabel} to confirm
		</label>
		<input
			id={inputId}
			type="text"
			bind:value={typedText}
			autocomplete="off"
			autocapitalize="off"
			spellcheck="false"
			class="mt-2 w-full rounded-md border border-gray-300 bg-white px-3 py-2 font-mono text-sm text-gray-900 placeholder-gray-400 focus:border-blue-500 focus:ring-1 focus:ring-blue-500 focus:outline-none dark:border-gray-600 dark:bg-gray-800 dark:text-gray-100 dark:placeholder-gray-500 dark:focus:border-blue-400"
			placeholder={expectedText}
			aria-describedby="typed-confirm-dialog-expected"
		/>
		<p
			id="typed-confirm-dialog-expected"
			class="mt-2 font-mono text-xs text-gray-500 dark:text-gray-400"
		>
			{expectedText}
		</p>
	{/snippet}

	{#if children}
		{@render children()}
	{/if}
</ConfirmDialog>
