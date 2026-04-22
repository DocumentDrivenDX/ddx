<script module lang="ts">
	import type { Snippet } from 'svelte';

	export type ConfirmDialogCloseReason = 'cancel' | 'confirm' | 'dismiss' | 'escape';

	export interface ConfirmDialogProps {
		open?: boolean;
		actionLabel: string;
		title?: string;
		cancelLabel?: string;
		destructive?: boolean;
		confirmDisabled?: boolean;
		returnFocusTo?: HTMLElement | null;
		summary?: Snippet;
		children?: Snippet;
		onConfirm?: () => void | Promise<void>;
		onCancel?: (reason: Exclude<ConfirmDialogCloseReason, 'confirm'>) => void;
		onOpenChange?: (open: boolean) => void;
	}
</script>

<script lang="ts">
	import { Dialog } from 'bits-ui';
	import { AlertTriangle, X } from 'lucide-svelte';

	let {
		open = $bindable(false),
		actionLabel,
		title = actionLabel,
		cancelLabel = 'Cancel',
		destructive = false,
		confirmDisabled = false,
		returnFocusTo = null,
		summary,
		children,
		onConfirm,
		onCancel,
		onOpenChange
	}: ConfirmDialogProps = $props();

	let confirming = $state(false);
	let wasOpen = false;
	let restoreFocusTarget: HTMLElement | null = null;

	const iconClass = $derived(
		destructive ? 'text-red-600 dark:text-red-400' : 'text-blue-600 dark:text-blue-400'
	);
	const actionClass = $derived(
		destructive
			? 'bg-red-600 text-white hover:bg-red-700 focus-visible:ring-red-500 disabled:bg-red-400 dark:bg-red-600 dark:hover:bg-red-500 dark:disabled:bg-red-900'
			: 'bg-blue-600 text-white hover:bg-blue-700 focus-visible:ring-blue-500 disabled:bg-blue-400 dark:bg-blue-600 dark:hover:bg-blue-500 dark:disabled:bg-blue-900'
	);

	$effect(() => {
		if (typeof document === 'undefined') {
			wasOpen = open;
			return;
		}

		if (open && !wasOpen) {
			restoreFocusTarget =
				returnFocusTo ??
				(document.activeElement instanceof HTMLElement ? document.activeElement : null);
		}

		if (!open && wasOpen) {
			const target = returnFocusTo ?? restoreFocusTarget;
			if (target) {
				queueMicrotask(() => {
					if (document.contains(target)) {
						target.focus({ preventScroll: true });
					}
				});
			}
		}

		wasOpen = open;
	});

	function setOpen(nextOpen: boolean) {
		open = nextOpen;
		onOpenChange?.(nextOpen);
	}

	function cancel(reason: Exclude<ConfirmDialogCloseReason, 'confirm'>) {
		onCancel?.(reason);
		setOpen(false);
	}

	function handleOpenChange(nextOpen: boolean) {
		if (nextOpen) {
			setOpen(true);
			return;
		}

		cancel('dismiss');
	}

	function handleEscape(event: KeyboardEvent) {
		event.preventDefault();
		cancel('escape');
	}

	async function confirm() {
		if (confirmDisabled || confirming) return;

		confirming = true;
		try {
			await onConfirm?.();
			setOpen(false);
		} finally {
			confirming = false;
		}
	}
</script>

<Dialog.Root {open} onOpenChange={handleOpenChange}>
	<Dialog.Portal>
		<Dialog.Overlay
			class="fixed inset-0 z-50 bg-gray-950/45 backdrop-blur-[2px] dark:bg-black/60"
		/>
		<Dialog.Content
			aria-label={actionLabel}
			onEscapeKeydown={handleEscape}
			class="fixed top-1/2 left-1/2 z-50 w-[calc(100vw-2rem)] max-w-md -translate-x-1/2 -translate-y-1/2 rounded-lg border border-gray-200 bg-white p-0 text-gray-900 shadow-2xl shadow-gray-950/20 focus:outline-none dark:border-gray-700 dark:bg-gray-900 dark:text-gray-100 dark:shadow-black/50"
		>
			<div class="flex items-start gap-3 border-b border-gray-200 px-5 py-4 dark:border-gray-700">
				<div
					class="mt-0.5 flex h-9 w-9 shrink-0 items-center justify-center rounded-md bg-gray-100 dark:bg-gray-800"
				>
					<AlertTriangle class="h-5 w-5 {iconClass}" aria-hidden="true" />
				</div>
				<div class="min-w-0 flex-1">
					<Dialog.Title level={2} class="text-base leading-6 font-semibold">
						{title}
					</Dialog.Title>
					{#if summary}
						<Dialog.Description class="mt-1 text-sm leading-5 text-gray-600 dark:text-gray-300">
							{@render summary()}
						</Dialog.Description>
					{/if}
				</div>
				<button
					type="button"
					class="rounded p-1.5 text-gray-500 hover:bg-gray-100 hover:text-gray-700 focus:outline-none focus-visible:ring-2 focus-visible:ring-blue-500 dark:text-gray-400 dark:hover:bg-gray-800 dark:hover:text-gray-200"
					aria-label="Close dialog"
					onclick={() => cancel('cancel')}
				>
					<X class="h-4 w-4" aria-hidden="true" />
				</button>
			</div>

			{#if children}
				<div class="px-5 py-4 text-sm leading-6 text-gray-700 dark:text-gray-300">
					{@render children()}
				</div>
			{/if}

			<div
				class="flex items-center justify-end gap-2 border-t border-gray-200 bg-gray-50 px-5 py-4 dark:border-gray-700 dark:bg-gray-900/70"
			>
				<button
					type="button"
					class="rounded-md border border-gray-300 bg-white px-3 py-2 text-sm font-medium text-gray-700 hover:bg-gray-50 focus:outline-none focus-visible:ring-2 focus-visible:ring-blue-500 disabled:cursor-not-allowed disabled:opacity-50 dark:border-gray-600 dark:bg-gray-800 dark:text-gray-200 dark:hover:bg-gray-700"
					onclick={() => cancel('cancel')}
				>
					{cancelLabel}
				</button>
				<button
					type="button"
					class="rounded-md px-3 py-2 text-sm font-medium focus:outline-none focus-visible:ring-2 focus-visible:ring-offset-2 focus-visible:ring-offset-white disabled:cursor-not-allowed {actionClass} dark:focus-visible:ring-offset-gray-900"
					disabled={confirmDisabled || confirming}
					onclick={confirm}
				>
					{confirming ? 'Working...' : actionLabel}
				</button>
			</div>
		</Dialog.Content>
	</Dialog.Portal>
</Dialog.Root>
