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
		destructive ? 'text-error dark:text-dark-error' : 'text-accent-lever dark:text-dark-accent-lever'
	);
	const actionClass = $derived(
		destructive
			? 'bg-error text-white hover:bg-error/90 focus-visible:ring-error disabled:bg-error/60 dark:bg-dark-error dark:text-fg-ink dark:hover:bg-dark-error/90 dark:disabled:bg-dark-error/40'
			: 'bg-accent-lever text-white hover:bg-accent-lever/90 focus-visible:ring-accent-lever disabled:bg-accent-lever/60 dark:bg-dark-accent-lever dark:text-fg-ink dark:hover:bg-dark-accent-lever/90 dark:disabled:bg-dark-accent-lever/40'
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
			class="fixed inset-0 z-50 bg-fg-ink/45 backdrop-blur-[2px] dark:bg-black/60"
		/>
		<Dialog.Content
			aria-label={title}
			onEscapeKeydown={handleEscape}
			class="fixed top-1/2 left-1/2 z-50 w-[calc(100vw-2rem)] max-w-md -translate-x-1/2 -translate-y-1/2 rounded-none border border-border-line bg-bg-elevated p-0 text-fg-ink shadow-2xl shadow-fg-ink/20 focus:outline-none dark:border-dark-border-line dark:bg-dark-bg-elevated dark:text-dark-fg-ink dark:shadow-black/50"
		>
			<div class="flex items-start gap-3 border-b border-border-line px-5 py-4 dark:border-dark-border-line">
				<div
					class="mt-0.5 flex h-9 w-9 shrink-0 items-center justify-center rounded-none bg-bg-surface dark:bg-dark-bg-surface"
				>
					<AlertTriangle class="h-5 w-5 {iconClass}" aria-hidden="true" />
				</div>
				<div class="min-w-0 flex-1">
					<Dialog.Title level={2} class="text-base leading-6 font-semibold">
						{title}
					</Dialog.Title>
					{#if summary}
						<Dialog.Description class="mt-1 text-sm leading-5 text-fg-muted dark:text-dark-fg-muted">
							{@render summary()}
						</Dialog.Description>
					{/if}
				</div>
				<button
					type="button"
					class="rounded-none p-1.5 text-fg-muted hover:bg-bg-surface hover:text-fg-ink focus:outline-none focus-visible:ring-2 focus-visible:ring-accent-lever dark:text-dark-fg-muted dark:hover:bg-dark-bg-surface dark:hover:text-dark-fg-ink"
					aria-label="Close dialog"
					onclick={() => cancel('cancel')}
				>
					<X class="h-4 w-4" aria-hidden="true" />
				</button>
			</div>

			{#if children}
				<div class="px-5 py-4 text-sm leading-6 text-fg-ink dark:text-dark-fg-ink">
					{@render children()}
				</div>
			{/if}

			<div
				class="flex items-center justify-end gap-2 border-t border-border-line bg-bg-canvas px-5 py-4 dark:border-dark-border-line dark:bg-dark-bg-canvas/70"
			>
				<button
					type="button"
					class="rounded-none border border-border-line bg-bg-elevated px-3 py-2 text-sm font-medium text-fg-ink hover:bg-bg-canvas focus:outline-none focus-visible:ring-2 focus-visible:ring-accent-lever disabled:cursor-not-allowed disabled:opacity-50 dark:border-dark-border-line dark:bg-dark-bg-surface dark:text-dark-fg-ink dark:hover:bg-dark-bg-canvas"
					onclick={() => cancel('cancel')}
				>
					{cancelLabel}
				</button>
				<button
					type="button"
					class="rounded-none px-3 py-2 text-sm font-medium focus:outline-none focus-visible:ring-2 focus-visible:ring-offset-2 focus-visible:ring-offset-bg-elevated disabled:cursor-not-allowed {actionClass} dark:focus-visible:ring-offset-dark-bg-elevated"
					disabled={confirmDisabled || confirming}
					onclick={confirm}
				>
					{confirming ? 'Working...' : actionLabel}
				</button>
			</div>
		</Dialog.Content>
	</Dialog.Portal>
</Dialog.Root>
