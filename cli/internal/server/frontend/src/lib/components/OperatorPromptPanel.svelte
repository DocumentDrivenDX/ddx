<script lang="ts">
	import { resolve } from '$app/paths';
	import { onMount, onDestroy } from 'svelte';
	import {
		getCsrfToken,
		OPERATOR_PROMPT_SUBMIT_MUTATION,
		OPERATOR_PROMPT_APPROVE_MUTATION,
		OPERATOR_PROMPT_CANCEL_MUTATION,
		RECENT_OPERATOR_PROMPTS_QUERY,
		type OperatorPromptBead,
		type OperatorPromptSubmitResult,
		type OperatorPromptApproveResult,
		type OperatorPromptCancelResult,
		type RecentOperatorPromptsResult
	} from '$lib/gql/operator-prompts';
	import { subscribeBeadLifecycle } from '$lib/gql/subscriptions';

	type Props = {
		projectId: string;
		nodeId: string;
	};

	let { projectId, nodeId }: Props = $props();

	const PRIORITY_OPTIONS = [0, 1, 2, 3, 4];

	let promptText = $state('');
	let tier = $state(2);
	let submitting = $state(false);
	let approving = $state(false);
	let pendingBead = $state<OperatorPromptBead | null>(null);
	let alertMessage = $state('');
	let recent = $state<OperatorPromptBead[]>([]);
	let liveStatus = $state<Map<string, string>>(new Map());

	function newIdempotencyKey(): string {
		if (typeof crypto !== 'undefined' && typeof crypto.randomUUID === 'function') {
			return crypto.randomUUID();
		}
		return `op-${Date.now()}-${Math.random().toString(36).slice(2, 10)}`;
	}

	async function gqlRequest<T>(query: string, variables: Record<string, unknown>): Promise<T> {
		const token = await getCsrfToken();
		const resp = await fetch('/graphql', {
			method: 'POST',
			credentials: 'same-origin',
			headers: {
				'Content-Type': 'application/json',
				'X-CSRF-Token': token
			},
			body: JSON.stringify({ query, variables })
		});
		const body = (await resp.json()) as {
			data?: T;
			errors?: Array<{ message: string }>;
		};
		if (body.errors && body.errors.length > 0) {
			throw new Error(body.errors.map((e) => e.message).join('; '));
		}
		if (!body.data) {
			throw new Error(`graphql request failed: ${resp.status}`);
		}
		return body.data;
	}

	async function loadRecent() {
		try {
			const data = await gqlRequest<RecentOperatorPromptsResult>(RECENT_OPERATOR_PROMPTS_QUERY, {
				projectID: projectId
			});
			recent = data.beadsByProject.edges.map((e) => e.node);
		} catch (err) {
			alertMessage = `Could not load recent prompts. ${errorText(err)}`;
		}
	}

	async function handleSubmit(event: SubmitEvent) {
		event.preventDefault();
		alertMessage = '';
		const trimmed = promptText.trim();
		if (!trimmed) {
			alertMessage = 'Prompt cannot be empty.';
			return;
		}
		submitting = true;
		try {
			const data = await gqlRequest<OperatorPromptSubmitResult>(
				OPERATOR_PROMPT_SUBMIT_MUTATION,
				{
					input: {
						prompt: trimmed,
						tier,
						idempotencyKey: newIdempotencyKey(),
						autoApprove: false
					}
				}
			);
			pendingBead = data.operatorPromptSubmit.bead;
			await loadRecent();
		} catch (err) {
			alertMessage = errorText(err);
		} finally {
			submitting = false;
		}
	}

	async function handleApprove() {
		if (!pendingBead) return;
		approving = true;
		alertMessage = '';
		try {
			const data = await gqlRequest<OperatorPromptApproveResult>(
				OPERATOR_PROMPT_APPROVE_MUTATION,
				{ id: pendingBead.id }
			);
			pendingBead = data.operatorPromptApprove.bead;
			promptText = '';
			pendingBead = null;
			await loadRecent();
		} catch (err) {
			alertMessage = errorText(err);
		} finally {
			approving = false;
		}
	}

	async function handleCancel() {
		if (!pendingBead) return;
		approving = true;
		alertMessage = '';
		try {
			await gqlRequest<OperatorPromptCancelResult>(OPERATOR_PROMPT_CANCEL_MUTATION, {
				id: pendingBead.id
			});
			pendingBead = null;
			await loadRecent();
		} catch (err) {
			alertMessage = errorText(err);
		} finally {
			approving = false;
		}
	}

	function discardPreview() {
		pendingBead = null;
	}

	function beadHref(beadId: string): string {
		return resolve('/nodes/[nodeId]/projects/[projectId]/beads/[beadId]', {
			nodeId,
			projectId,
			beadId
		});
	}

	function statusOf(bead: OperatorPromptBead): string {
		return liveStatus.get(bead.id) ?? bead.status;
	}

	function statusBadgeClass(status: string): string {
		switch (status) {
			case 'proposed':
				return 'bg-yellow-100 text-yellow-900 dark:bg-yellow-900/30 dark:text-yellow-200';
			case 'open':
				return 'bg-green-100 text-green-900 dark:bg-green-900/30 dark:text-green-200';
			case 'in_progress':
				return 'bg-blue-100 text-blue-900 dark:bg-blue-900/30 dark:text-blue-200';
			case 'closed':
				return 'bg-fg-muted/20 text-fg-muted dark:bg-dark-fg-muted/20 dark:text-dark-fg-muted';
			case 'cancelled':
				return 'bg-fg-muted/20 text-fg-muted line-through dark:bg-dark-fg-muted/20 dark:text-dark-fg-muted';
			default:
				return 'bg-bg-surface text-fg-ink dark:bg-dark-bg-surface dark:text-dark-fg-ink';
		}
	}

	function errorText(err: unknown): string {
		if (err instanceof Error) return err.message;
		if (typeof err === 'string') return err;
		return 'Unknown error.';
	}

	let unsubscribe: (() => void) | null = null;
	onMount(() => {
		void loadRecent();
		unsubscribe = subscribeBeadLifecycle(projectId, (evt) => {
			if (evt.kind === 'status_changed' && evt.summary) {
				const match = evt.summary.match(/status changed from \S+ to (\S+)/);
				if (match) {
					const next = new Map(liveStatus);
					next.set(evt.beadID, match[1]);
					liveStatus = next;
				}
			}
			if (evt.kind === 'created') {
				void loadRecent();
			}
		});
	});
	onDestroy(() => {
		unsubscribe?.();
	});
</script>

<section
	data-testid="operator-prompt-panel"
	class="space-y-4 rounded-md border border-border-line bg-bg-elevated p-4 dark:border-dark-border-line dark:bg-dark-bg-elevated"
>
	<header>
		<h2 class="text-headline-md font-semibold text-fg-ink dark:text-dark-fg-ink">
			Operator prompt
		</h2>
		<p class="mt-1 text-sm text-fg-muted dark:text-dark-fg-muted">
			Submit a prompt that becomes a proposed bead. Approve to queue it for the execute-loop.
		</p>
	</header>

	{#if alertMessage}
		<div
			role="alert"
			data-testid="operator-prompt-alert"
			class="rounded-md border border-error/30 bg-error/10 px-3 py-2 text-sm text-error dark:border-dark-error/30 dark:bg-dark-error/10 dark:text-dark-error"
		>
			{alertMessage}
		</div>
	{/if}

	{#if !pendingBead}
		<form data-testid="operator-prompt-form" onsubmit={handleSubmit} class="space-y-3">
			<label class="block">
				<span class="text-sm font-medium text-fg-ink dark:text-dark-fg-ink">Prompt</span>
				<textarea
					data-testid="operator-prompt-textarea"
					bind:value={promptText}
					required
					rows="5"
					placeholder="Describe the change you want — this becomes the bead description."
					class="mt-1 w-full rounded-md border border-border-line bg-bg-canvas px-3 py-2 text-sm text-fg-ink placeholder-fg-muted focus:border-accent-lever focus:ring-1 focus:ring-accent-lever focus:outline-none dark:border-dark-border-line dark:bg-dark-bg-canvas dark:text-dark-fg-ink dark:placeholder-dark-fg-muted"
				></textarea>
			</label>
			<div class="flex flex-wrap items-center gap-3">
				<label class="flex items-center gap-2 text-sm text-fg-ink dark:text-dark-fg-ink">
					<span class="font-medium">Priority</span>
					<select
						data-testid="operator-prompt-tier"
						bind:value={tier}
						class="rounded-md border border-border-line bg-bg-canvas px-2 py-1 text-sm dark:border-dark-border-line dark:bg-dark-bg-canvas"
					>
						{#each PRIORITY_OPTIONS as p}
							<option value={p}>P{p}</option>
						{/each}
					</select>
				</label>
				<button
					type="submit"
					data-testid="operator-prompt-submit"
					disabled={submitting}
					class="rounded-md bg-accent-lever px-3 py-2 text-sm font-medium text-white hover:opacity-90 disabled:cursor-wait disabled:opacity-60"
				>
					{submitting ? 'Submitting…' : 'Submit prompt'}
				</button>
			</div>
		</form>
	{:else}
		<div data-testid="operator-prompt-preview" class="space-y-3">
			<div class="rounded-md border border-border-line bg-bg-surface p-3 dark:border-dark-border-line dark:bg-dark-bg-surface">
				<div class="flex items-center justify-between">
					<h3 class="text-sm font-semibold text-fg-ink dark:text-dark-fg-ink">
						This is what we will send
					</h3>
					<a
						data-testid="operator-prompt-preview-link"
						href={beadHref(pendingBead.id)}
						class="font-mono-code text-mono-code text-accent-lever hover:underline dark:text-dark-accent-lever"
					>
						{pendingBead.id}
					</a>
				</div>
				<dl class="mt-2 grid grid-cols-3 gap-3 text-xs text-fg-muted dark:text-dark-fg-muted">
					<div>
						<dt>Status</dt>
						<dd
							data-testid="operator-prompt-preview-status"
							class="mt-1 inline-block rounded px-2 py-0.5 text-xs font-semibold {statusBadgeClass(
								statusOf(pendingBead)
							)}"
						>
							{statusOf(pendingBead)}
						</dd>
					</div>
					<div>
						<dt>Priority</dt>
						<dd class="mt-1 font-mono-code text-mono-code text-fg-ink dark:text-dark-fg-ink">
							P{pendingBead.priority}
						</dd>
					</div>
					<div>
						<dt>Type</dt>
						<dd class="mt-1 font-mono-code text-mono-code text-fg-ink dark:text-dark-fg-ink">
							{pendingBead.issueType}
						</dd>
					</div>
				</dl>
				<div class="mt-3">
					<div class="text-xs font-medium text-fg-muted dark:text-dark-fg-muted">Title</div>
					<div
						data-testid="operator-prompt-preview-title"
						class="mt-1 text-sm text-fg-ink dark:text-dark-fg-ink"
					>
						{pendingBead.title}
					</div>
				</div>
				<div class="mt-3">
					<div class="text-xs font-medium text-fg-muted dark:text-dark-fg-muted">Body</div>
					<pre
						data-testid="operator-prompt-preview-body"
						class="mt-1 max-h-60 overflow-auto whitespace-pre-wrap break-words rounded bg-bg-canvas p-2 font-mono-code text-mono-code text-fg-ink dark:bg-dark-bg-canvas dark:text-dark-fg-ink">{pendingBead.description ?? promptText}</pre>
				</div>
			</div>
			<div class="flex flex-wrap gap-2">
				<button
					type="button"
					data-testid="operator-prompt-approve"
					onclick={handleApprove}
					disabled={approving}
					class="rounded-md bg-accent-lever px-3 py-2 text-sm font-medium text-white hover:opacity-90 disabled:cursor-wait disabled:opacity-60"
				>
					{approving ? 'Approving…' : 'Approve & queue'}
				</button>
				<button
					type="button"
					data-testid="operator-prompt-cancel"
					onclick={handleCancel}
					disabled={approving}
					class="rounded-md border border-border-line bg-bg-canvas px-3 py-2 text-sm font-medium text-fg-ink hover:bg-bg-surface disabled:cursor-wait dark:border-dark-border-line dark:bg-dark-bg-canvas dark:text-dark-fg-ink dark:hover:bg-dark-bg-surface"
				>
					Cancel proposal
				</button>
				<button
					type="button"
					data-testid="operator-prompt-discard"
					onclick={discardPreview}
					class="rounded-md border border-border-line px-3 py-2 text-sm font-medium text-fg-muted hover:bg-bg-surface dark:border-dark-border-line dark:text-dark-fg-muted dark:hover:bg-dark-bg-surface"
				>
					Edit another prompt
				</button>
			</div>
		</div>
	{/if}

	<div data-testid="operator-prompt-recent" class="space-y-2">
		<h3 class="text-sm font-semibold text-fg-ink dark:text-dark-fg-ink">Recent prompts</h3>
		{#if recent.length === 0}
			<p class="text-xs text-fg-muted dark:text-dark-fg-muted">No operator prompts yet.</p>
		{:else}
			<ul class="divide-y divide-border-line rounded-md border border-border-line dark:divide-dark-border-line dark:border-dark-border-line">
				{#each recent as bead (bead.id)}
					<li class="flex items-center justify-between gap-3 px-3 py-2">
						<a
							data-testid="operator-prompt-recent-link"
							href={beadHref(bead.id)}
							class="min-w-0 flex-1 truncate text-sm text-fg-ink hover:underline dark:text-dark-fg-ink"
						>
							<span class="font-mono-code text-mono-code text-accent-lever dark:text-dark-accent-lever">{bead.id}</span>
							<span class="ml-2">{bead.title}</span>
						</a>
						<span
							data-testid="operator-prompt-recent-status"
							class="rounded px-2 py-0.5 text-xs font-semibold {statusBadgeClass(statusOf(bead))}"
						>
							{statusOf(bead)}
						</span>
					</li>
				{/each}
			</ul>
		{/if}
	</div>
</section>
