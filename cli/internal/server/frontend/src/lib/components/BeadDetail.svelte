<script lang="ts">
	import { gql, type RequestDocument } from 'graphql-request';
	import { createClient } from '$lib/gql/client';
	import { extractGraphQLErrorMessage } from '$lib/gql/error';
	import { invalidateAll } from '$app/navigation';
	import { nodeStore } from '$lib/stores/node.svelte';
	import { X, UserPlus, UserMinus, Pencil, Trash2, Copy, Check } from 'lucide-svelte';
	import BeadForm from './BeadForm.svelte';
	import ConfirmDialog from './ConfirmDialog.svelte';
	import TypedConfirmDialog from './TypedConfirmDialog.svelte';

	interface Dependency {
		issueId: string;
		dependsOnId: string;
		type: string;
		createdAt: string | null;
		createdBy: string | null;
	}

	interface Bead {
		id: string;
		title: string;
		status: string;
		priority: number;
		issueType: string;
		owner: string | null;
		createdAt: string;
		createdBy: string | null;
		updatedAt: string;
		labels: string[] | null;
		parent: string | null;
		description: string | null;
		acceptance: string | null;
		notes: string | null;
		dependencies: Dependency[] | null;
		childCount?: number;
	}

	interface BeadExecutionRow {
		id: string;
		verdict: string | null;
		harness: string | null;
		createdAt: string;
		durationMs: number | null;
		costUsd: number | null;
	}

	interface BeadRunRow {
		id: string;
		layer: string;
		status: string;
		harness: string | null;
		startedAt: string | null;
		durationMs: number | null;
	}

	type LifecycleAction = 'approve' | 'block' | 'cancel' | 'reopen';

	interface LifecycleActionConfig {
		title: string;
		description: string;
		actionLabel: string;
		destructive: boolean;
		reasonLabel: string | null;
		reasonPlaceholder: string | null;
		responseField: 'beadApprove' | 'beadBlock' | 'beadCancel' | 'beadReopen';
		query: RequestDocument;
		variableName?: 'note' | 'reason' | 'externalBlockerReason';
	}

	let {
		bead: initialBead,
		onClose,
		executions = [],
		runs = [],
		nodeId = '',
		projectId = ''
	}: {
		bead: Bead;
		onClose: () => void;
		executions?: BeadExecutionRow[];
		runs?: BeadRunRow[];
		nodeId?: string;
		projectId?: string;
	} = $props();

	function executionHref(executionId: string): string {
		return `/nodes/${nodeId}/projects/${projectId}/executions/${executionId}`;
	}

	function runHref(runId: string): string {
		return `/nodes/${nodeId}/projects/${projectId}/runs/${runId}`;
	}

	function fmtExecDate(iso: string): string {
		try {
			return new Date(iso).toLocaleString();
		} catch {
			return iso;
		}
	}

	let bead = $state<Bead>({ ...initialBead });
	let editing = $state(false);
	let busy = $state(false);
	let actionError = $state<string | null>(null);
	let deleteDialogOpen = $state(false);
	let cascadeToChildren = $state(false);
	let lifecycleDialogOpen = $state(false);
	let lifecycleAction = $state<LifecycleAction | null>(null);
	let lifecycleReason = $state('');
	let deleteButton = $state<HTMLButtonElement | null>(null);
	let idCopied = $state(false);
	let idCopyTimer: ReturnType<typeof setTimeout> | null = null;
	const hasChildBeads = $derived((bead.childCount ?? 0) > 0);

	const MUTATION_FIELDS = `
				id
				title
				status
				priority
				issueType
				owner
				createdAt
				createdBy
				updatedAt
				labels
				parent
				description
				acceptance
				notes
				dependencies {
					issueId
					dependsOnId
					type
					createdAt
					createdBy
				}
	`;

	async function handleCopyId() {
		try {
			await navigator.clipboard.writeText(bead.id);
			idCopied = true;
			if (idCopyTimer) clearTimeout(idCopyTimer);
			idCopyTimer = setTimeout(() => {
				idCopied = false;
			}, 1500);
		} catch {
			// clipboard may be unavailable; silently fail
		}
	}

	function openLifecycleDialog(action: LifecycleAction) {
		lifecycleAction = action;
		lifecycleReason = '';
		actionError = null;
		lifecycleDialogOpen = true;
	}

	function closeLifecycleDialog() {
		lifecycleDialogOpen = false;
		lifecycleAction = null;
		lifecycleReason = '';
		actionError = null;
	}

	async function submitLifecycleAction() {
		if (!lifecycleAction) return;

		const config = LIFECYCLE_ACTIONS[lifecycleAction];
		if (config.variableName && !lifecycleReason.trim()) {
			actionError = `${config.reasonLabel ?? 'Note'} is required`;
			return;
		}

		const client = createClient(undefined, projectId);
		const variables: Record<string, string> = { id: bead.id };
		if (config.variableName) {
			variables[config.variableName] = lifecycleReason;
		}

		busy = true;
		actionError = null;
		try {
			const result = await client.request<Record<string, Bead>>(config.query, variables);
			const updated = result[config.responseField];
			if (updated) {
				bead = updated;
				actionError = null;
				await invalidateAll();
			}
		} catch (error) {
			actionError = extractGraphQLErrorMessage(error);
		} finally {
			busy = false;
		}
	}

	const CLAIM_MUTATION = gql`
		mutation BeadClaim($id: ID!, $assignee: String!) {
			beadClaim(id: $id, assignee: $assignee) {
				id
				title
				status
				priority
				issueType
				owner
				createdAt
				createdBy
				updatedAt
				labels
				parent
				description
				acceptance
				notes
				dependencies {
					issueId
					dependsOnId
					type
					createdAt
					createdBy
				}
			}
		}
	`;

	const UNCLAIM_MUTATION = gql`
		mutation BeadUnclaim($id: ID!) {
			beadUnclaim(id: $id) {
				id
				title
				status
				priority
				issueType
				owner
				createdAt
				createdBy
				updatedAt
				labels
				parent
				description
				acceptance
				notes
				dependencies {
					issueId
					dependsOnId
					type
					createdAt
					createdBy
				}
			}
		}
	`;

	const CLOSE_MUTATION = gql`
		mutation BeadClose($id: ID!, $reason: String) {
			beadClose(id: $id, reason: $reason) {
				${MUTATION_FIELDS}
			}
		}
	`;

	const APPROVE_MUTATION = gql`
		mutation BeadApprove($id: ID!, $note: String!) {
			beadApprove(id: $id, note: $note) {
				${MUTATION_FIELDS}
			}
		}
	`;

	const BLOCK_MUTATION = gql`
		mutation BeadBlock($id: ID!, $externalBlockerReason: String!) {
			beadBlock(id: $id, externalBlockerReason: $externalBlockerReason) {
				${MUTATION_FIELDS}
			}
		}
	`;

	const CANCEL_MUTATION = gql`
		mutation BeadCancel($id: ID!, $reason: String!) {
			beadCancel(id: $id, reason: $reason) {
				${MUTATION_FIELDS}
			}
		}
	`;

	const REOPEN_MUTATION = gql`
		mutation BeadReopen($id: ID!) {
			beadReopen(id: $id) {
				${MUTATION_FIELDS}
			}
		}
	`;

	const LIFECYCLE_ACTIONS: Record<LifecycleAction, LifecycleActionConfig> = {
		approve: {
			title: 'Approve bead',
			description: 'Approve this bead and record a note on the lifecycle event.',
			actionLabel: 'Confirm',
			destructive: false,
			reasonLabel: 'Approval note',
			reasonPlaceholder: 'Explain why this bead is ready',
			responseField: 'beadApprove',
			query: APPROVE_MUTATION,
			variableName: 'note'
		},
		block: {
			title: 'Block bead',
			description: 'Block this bead and record the external blocker reason.',
			actionLabel: 'Confirm',
			destructive: true,
			reasonLabel: 'Blocker reason',
			reasonPlaceholder: 'Describe the blocker or dependency',
			responseField: 'beadBlock',
			query: BLOCK_MUTATION,
			variableName: 'externalBlockerReason'
		},
		cancel: {
			title: 'Cancel bead',
			description: 'Cancel this bead and record why the work is no longer needed.',
			actionLabel: 'Confirm',
			destructive: true,
			reasonLabel: 'Cancellation reason',
			reasonPlaceholder: 'Describe why the bead is being cancelled',
			responseField: 'beadCancel',
			query: CANCEL_MUTATION,
			variableName: 'reason'
		},
		reopen: {
			title: 'Reopen bead',
			description: 'Reopen this bead without adding a note.',
			actionLabel: 'Confirm',
			destructive: false,
			reasonLabel: null,
			reasonPlaceholder: null,
			responseField: 'beadReopen',
			query: REOPEN_MUTATION
		}
	};
	const lifecycleConfig = $derived(lifecycleAction ? LIFECYCLE_ACTIONS[lifecycleAction] : null);

	async function handleClaim() {
		busy = true;
		actionError = null;
		try {
			const client = createClient(undefined, projectId);
			const assignee = nodeStore.value?.name ?? 'user';
			const result = await client.request<{ beadClaim: Bead }>(CLAIM_MUTATION, {
				id: bead.id,
				assignee
			});
			bead = result.beadClaim;
			actionError = null;
			await invalidateAll();
		} catch (e) {
			actionError = extractGraphQLErrorMessage(e, 'Claim failed');
		} finally {
			busy = false;
		}
	}

	async function handleUnclaim() {
		busy = true;
		actionError = null;
		try {
			const client = createClient(undefined, projectId);
			const result = await client.request<{ beadUnclaim: Bead }>(UNCLAIM_MUTATION, {
				id: bead.id
			});
			bead = result.beadUnclaim;
			actionError = null;
			await invalidateAll();
		} catch (e) {
			actionError = extractGraphQLErrorMessage(e, 'Unclaim failed');
		} finally {
			busy = false;
		}
	}

	function openDeleteDialog() {
		cascadeToChildren = false;
		deleteDialogOpen = true;
	}

	async function handleDeleteConfirm() {
		busy = true;
		actionError = null;
		try {
			const client = createClient(undefined, projectId);
			await client.request<{ beadClose: Bead }>(CLOSE_MUTATION, {
				id: bead.id,
				reason: 'deleted via UI'
			});
			await invalidateAll();
			onClose();
		} catch (e) {
			actionError = extractGraphQLErrorMessage(e, 'Delete failed');
		} finally {
			busy = false;
		}
	}

	function statusClass(status: string): string {
		switch (status) {
			case 'open':
				return 'text-accent-lever dark:text-dark-accent-lever';
			case 'in-progress':
			case 'in_progress':
				return 'text-accent-load dark:text-dark-accent-load';
			case 'closed':
				return 'text-status-closed dark:text-status-closed';
			case 'blocked':
				return 'text-error dark:text-dark-error';
			case 'proposed':
				return 'text-status-proposed dark:text-dark-status-proposed';
			case 'cancelled':
				return 'text-fg-muted dark:text-dark-fg-muted';
			default:
				return 'text-fg-muted dark:text-dark-fg-muted';
		}
	}
</script>

<!-- Right-side detail panel -->
<div
	class="fixed top-0 right-0 z-50 flex h-full w-full max-w-xl flex-col bg-bg-elevated shadow-xl dark:bg-dark-bg-canvas"
	style="max-width: 36rem;"
>
	<!-- Header -->
	<div
		class="flex shrink-0 items-center justify-between border-b border-border-line px-6 py-4 dark:border-dark-border-line"
	>
		<div class="flex min-w-0 items-center gap-3">
			<span
				title={bead.id}
				data-testid="bead-detail-id"
				class="min-w-0 truncate font-mono-code text-body-sm text-fg-muted dark:text-dark-fg-muted"
				>{bead.id}</span
			>
			<button
				type="button"
				onclick={handleCopyId}
				aria-label="Copy bead id"
				data-testid="bead-detail-copy-id"
				class="shrink-0 rounded-none p-1 text-fg-muted hover:bg-bg-canvas hover:text-fg-ink dark:hover:bg-dark-bg-elevated dark:hover:text-dark-fg-ink"
			>
				{#if idCopied}
					<Check class="h-3.5 w-3.5 text-status-closed" />
				{:else}
					<Copy class="h-3.5 w-3.5" />
				{/if}
			</button>
			<span data-testid="bead-status-badge" class="shrink-0 font-medium {statusClass(bead.status)}">{bead.status}</span>
			{#if bead.owner}
				<span class="shrink-0 truncate text-body-sm text-fg-muted dark:text-dark-fg-muted">@ {bead.owner}</span>
			{/if}
		</div>
		<div class="ml-3 flex flex-wrap items-center justify-end gap-2">
			{#if !editing}
				{#if bead.status === 'open' || bead.status === 'blocked'}
					<button
						onclick={handleClaim}
						disabled={busy}
						class="flex items-center gap-1.5 rounded-none bg-accent-lever px-3 py-1.5 text-body-sm font-medium text-white hover:bg-accent-lever/90 disabled:cursor-not-allowed disabled:opacity-50"
					>
						<UserPlus class="h-3.5 w-3.5" />
						Claim
					</button>
				{:else if bead.status === 'in-progress'}
					<button
						onclick={handleUnclaim}
						disabled={busy}
						class="flex items-center gap-1.5 rounded-none border border-border-line px-3 py-1.5 text-body-sm font-medium text-fg-muted hover:bg-bg-surface disabled:cursor-not-allowed disabled:opacity-50 dark:border-dark-border-line dark:text-dark-fg-ink dark:hover:bg-dark-bg-elevated"
					>
						<UserMinus class="h-3.5 w-3.5" />
						Unclaim
					</button>
				{/if}
				<button
					onclick={() => (editing = true)}
					disabled={busy}
					class="flex items-center gap-1.5 rounded-none border border-border-line px-3 py-1.5 text-body-sm font-medium text-fg-muted hover:bg-bg-surface dark:border-dark-border-line dark:text-dark-fg-ink dark:hover:bg-dark-bg-elevated"
				>
					<Pencil class="h-3.5 w-3.5" />
					Edit
				</button>
				<button
					bind:this={deleteButton}
					onclick={openDeleteDialog}
					disabled={busy}
					class="flex items-center gap-1.5 rounded-none border border-error/30 px-3 py-1.5 text-body-sm font-medium text-error hover:bg-error/10 disabled:cursor-not-allowed disabled:opacity-50 dark:border-dark-error/30 dark:text-dark-error dark:hover:bg-dark-error/10"
				>
					<Trash2 class="h-3.5 w-3.5" />
					Delete
				</button>
				<button
					type="button"
					onclick={() => openLifecycleDialog('approve')}
					disabled={busy}
					class="rounded-none border border-border-line px-3 py-1.5 text-body-sm font-medium text-fg-muted hover:bg-bg-surface disabled:cursor-not-allowed disabled:opacity-50 dark:border-dark-border-line dark:text-dark-fg-ink dark:hover:bg-dark-bg-elevated"
				>
					Approve
				</button>
				<button
					type="button"
					onclick={() => openLifecycleDialog('block')}
					disabled={busy}
					class="rounded-none border border-error/30 px-3 py-1.5 text-body-sm font-medium text-error hover:bg-error/10 disabled:cursor-not-allowed disabled:opacity-50 dark:border-dark-error/30 dark:text-dark-error dark:hover:bg-dark-error/10"
				>
					Block
				</button>
				<button
					type="button"
					onclick={() => openLifecycleDialog('cancel')}
					disabled={busy}
					class="rounded-none border border-error/30 px-3 py-1.5 text-body-sm font-medium text-error hover:bg-error/10 disabled:cursor-not-allowed disabled:opacity-50 dark:border-dark-error/30 dark:text-dark-error dark:hover:bg-dark-error/10"
				>
					Cancel
				</button>
				<button
					type="button"
					onclick={() => openLifecycleDialog('reopen')}
					disabled={busy}
					class="rounded-none border border-border-line px-3 py-1.5 text-body-sm font-medium text-fg-muted hover:bg-bg-surface disabled:cursor-not-allowed disabled:opacity-50 dark:border-dark-border-line dark:text-dark-fg-ink dark:hover:bg-dark-bg-elevated"
				>
					Reopen
				</button>
			{/if}
			<button
				onclick={onClose}
				class="rounded-none p-1.5 text-fg-muted hover:bg-bg-canvas dark:text-dark-fg-muted dark:hover:bg-dark-bg-elevated"
				aria-label="Close panel"
			>
				<X class="h-4 w-4" />
			</button>
		</div>
	</div>

	<!-- Action error banner -->
	{#if actionError}
		<div
			role="alert"
			data-testid="error-message"
			class="shrink-0 border-b border-error/30 bg-error/10 px-6 py-2 text-body-sm text-error dark:border-dark-error/30 dark:bg-dark-error/10 dark:text-dark-error"
		>
			{actionError}
		</div>
	{/if}

	<!-- Scrollable content -->
	<div class="flex-1 overflow-auto p-6">
		{#if editing}
			{#key bead?.id}
				<BeadForm
					{bead}
					{projectId}
					onSuccess={(updated) => {
						bead = updated;
						editing = false;
					}}
					onCancel={() => (editing = false)}
				/>
			{/key}
		{:else}
			<!-- Read mode -->
			<h2 class="mb-5 text-headline-lg font-headline-lg text-fg-ink dark:text-dark-fg-ink">{bead.title}</h2>

			<dl class="space-y-4 text-body-sm">
				<div class="grid grid-cols-2 gap-4">
					<div>
						<dt
							class="text-label-caps font-label-caps text-fg-muted uppercase dark:text-dark-fg-muted"
						>
							Priority
						</dt>
						<dd class="mt-1 text-fg-ink dark:text-dark-fg-ink">{bead.priority}</dd>
					</div>
					<div>
						<dt
							class="text-label-caps font-label-caps text-fg-muted uppercase dark:text-dark-fg-muted"
						>
							Type
						</dt>
						<dd class="mt-1 text-fg-ink dark:text-dark-fg-ink">{bead.issueType || '—'}</dd>
					</div>
					{#if bead.parent}
						<div class="col-span-2">
							<dt
								class="text-label-caps font-label-caps text-fg-muted uppercase dark:text-dark-fg-muted"
							>
								Parent
							</dt>
							<dd class="mt-1 font-mono-code text-body-sm text-fg-muted dark:text-dark-fg-muted">{bead.parent}</dd>
						</div>
					{/if}
				</div>

				{#if bead.labels && bead.labels.length > 0}
					<div>
						<dt
							class="text-label-caps font-label-caps text-fg-muted uppercase dark:text-dark-fg-muted"
						>
							Labels
						</dt>
						<dd class="mt-1 flex flex-wrap gap-1">
							{#each bead.labels as label}
								<span
									class="rounded-none bg-bg-canvas px-2 py-0.5 text-body-sm text-fg-muted dark:bg-dark-bg-elevated dark:text-dark-fg-ink"
									>{label}</span
								>
							{/each}
						</dd>
					</div>
				{/if}

				{#if bead.description}
					<div>
						<dt
							class="text-label-caps font-label-caps text-fg-muted uppercase dark:text-dark-fg-muted"
						>
							Description
						</dt>
						<dd class="mt-1 whitespace-pre-wrap text-fg-muted dark:text-dark-fg-ink">
							{bead.description}
						</dd>
					</div>
				{/if}

				{#if bead.acceptance}
					<div>
						<dt
							class="text-label-caps font-label-caps text-fg-muted uppercase dark:text-dark-fg-muted"
						>
							Acceptance
						</dt>
						<dd class="mt-1 whitespace-pre-wrap text-fg-muted dark:text-dark-fg-ink">
							{bead.acceptance}
						</dd>
					</div>
				{/if}

				{#if bead.notes}
					<div>
						<dt
							class="text-label-caps font-label-caps text-fg-muted uppercase dark:text-dark-fg-muted"
						>
							Notes
						</dt>
						<dd class="mt-1 whitespace-pre-wrap text-fg-muted dark:text-dark-fg-ink">{bead.notes}</dd>
					</div>
				{/if}

				{#if runs.length > 0}
					<div data-testid="bead-linked-runs">
						<dt class="text-label-caps font-label-caps text-fg-muted uppercase dark:text-dark-fg-muted">
							Linked Runs ({runs.length})
						</dt>
						<dd class="mt-1 space-y-1">
							{#each runs as run (run.id)}
								<a
									href={runHref(run.id)}
									data-testid="bead-linked-run"
									class="flex items-center justify-between rounded-none border border-border-line px-2 py-1 text-body-sm hover:bg-bg-surface dark:border-dark-border-line dark:hover:bg-dark-bg-elevated"
								>
									<span class="flex items-center gap-2">
										<span class="font-mono-code text-accent-lever dark:text-dark-accent-lever">{run.id}</span>
										<span class="rounded-none border border-border-line bg-bg-canvas px-1 py-0.5 text-label-caps font-label-caps uppercase text-fg-muted dark:border-dark-border-line dark:bg-dark-bg-elevated dark:text-dark-fg-muted">
											{run.layer}
										</span>
										<span class="rounded-none border border-border-line bg-bg-canvas px-1 py-0.5 text-label-caps font-label-caps uppercase text-fg-muted dark:border-dark-border-line dark:bg-dark-bg-elevated dark:text-dark-fg-muted">
											{run.status}
										</span>
										{#if run.harness}
											<span class="font-mono-code text-fg-muted dark:text-dark-fg-muted">{run.harness}</span>
										{/if}
									</span>
									<span class="text-fg-muted dark:text-dark-fg-muted">
										{run.startedAt ? new Date(run.startedAt).toLocaleString() : '—'}
									</span>
								</a>
							{/each}
						</dd>
					</div>
				{/if}

				{#if executions.length > 0}
					<div data-testid="bead-executions">
						<dt class="text-label-caps font-label-caps text-fg-muted uppercase dark:text-dark-fg-muted">
							Executions ({executions.length})
						</dt>
						<dd class="mt-1 space-y-1">
							{#each executions as exec (exec.id)}
								<a
									href={executionHref(exec.id)}
									class="flex items-center justify-between rounded-none border border-border-line px-2 py-1 text-body-sm hover:bg-bg-surface dark:border-dark-border-line dark:hover:bg-dark-bg-elevated"
								>
									<span class="flex items-center gap-2">
										<span class="font-mono-code text-accent-lever dark:text-dark-accent-lever">{exec.id}</span>
										{#if exec.verdict}
											<span class="rounded-none border border-border-line bg-bg-canvas px-1 py-0.5 text-label-caps font-label-caps uppercase text-fg-muted dark:border-dark-border-line dark:bg-dark-bg-elevated dark:text-dark-fg-muted">
												{exec.verdict}
											</span>
										{/if}
									</span>
									<span class="text-fg-muted dark:text-dark-fg-muted">{fmtExecDate(exec.createdAt)}</span>
								</a>
							{/each}
						</dd>
					</div>
				{/if}

				{#if bead.dependencies && bead.dependencies.length > 0}
					<div>
						<dt
							class="text-label-caps font-label-caps text-fg-muted uppercase dark:text-dark-fg-muted"
						>
							Dependencies
						</dt>
						<dd class="mt-1 space-y-1">
							{#each bead.dependencies as dep}
								<div class="font-mono-code text-body-sm text-fg-muted dark:text-dark-fg-muted">
									{dep.dependsOnId}
									<span class="text-fg-muted dark:text-dark-fg-muted">({dep.type})</span>
								</div>
							{/each}
						</dd>
					</div>
				{/if}

				<div
					class="border-t border-border-line pt-4 text-body-sm text-fg-muted dark:border-dark-border-line dark:text-dark-fg-muted"
				>
					<div>
						Created: {new Date(bead.createdAt).toLocaleString()}{bead.createdBy
							? ` by ${bead.createdBy}`
							: ''}
					</div>
					<div>Updated: {new Date(bead.updatedAt).toLocaleString()}</div>
				</div>
			</dl>
	{/if}
	</div>

	{#if lifecycleConfig}
		<ConfirmDialog
			bind:open={lifecycleDialogOpen}
			actionLabel={lifecycleConfig.actionLabel}
			title={lifecycleConfig.title}
			destructive={lifecycleConfig.destructive}
			confirmDisabled={busy || (lifecycleConfig.variableName != null && !lifecycleReason.trim())}
			onConfirm={submitLifecycleAction}
			onOpenChange={(open) => {
				if (!open) closeLifecycleDialog();
			}}
		>
			{#snippet summary()}
				<span>
					{lifecycleConfig.description} <span class="font-mono">{bead.id}</span>.
				</span>
			{/snippet}

			{#if lifecycleConfig.reasonLabel}
				<div class="space-y-2">
					<label
						for="bead-lifecycle-reason"
						class="block text-sm font-medium text-fg-ink dark:text-dark-fg-ink"
					>
						{lifecycleConfig.reasonLabel} <span class="text-error dark:text-dark-error" aria-hidden="true">*</span>
					</label>
					<textarea
						id="bead-lifecycle-reason"
						bind:value={lifecycleReason}
						rows={4}
						placeholder={lifecycleConfig.reasonPlaceholder ?? ''}
						class="w-full rounded-none border border-border-line bg-bg-elevated px-3 py-2 text-body-sm text-fg-ink placeholder-fg-muted focus:border-accent-lever focus:ring-1 focus:ring-accent-lever focus:outline-none dark:border-dark-border-line dark:bg-dark-bg-surface dark:text-dark-fg-ink dark:placeholder-dark-fg-muted dark:focus:border-dark-accent-lever"
					></textarea>
					{#if !lifecycleReason.trim()}
						<p data-testid="lifecycle-reason-required-hint" class="text-xs text-fg-muted dark:text-dark-fg-muted">Required</p>
					{/if}
				</div>
			{/if}
		</ConfirmDialog>
	{/if}

	<TypedConfirmDialog
		bind:open={deleteDialogOpen}
		actionLabel="Delete bead"
		title="Delete bead"
		expectedText={bead.id}
		expectedLabel="bead id"
		destructive
		confirmDisabled={busy}
		returnFocusTo={deleteButton}
		onConfirm={handleDeleteConfirm}
	>
		{#snippet summary()}
			<span>
				This closes <span class="font-mono">{bead.id}</span> as deleted.
			</span>
		{/snippet}

		{#if hasChildBeads}
			<label
				class="mt-4 flex items-start gap-3 rounded-none border border-error/30 bg-error/10 p-3 text-body-sm text-error dark:border-dark-error/30 dark:bg-dark-error/10 dark:text-dark-error"
			>
				<input
					type="checkbox"
					bind:checked={cascadeToChildren}
					class="mt-1 h-4 w-4 rounded-none border-error/50 text-error focus:ring-error dark:border-dark-error/50 dark:bg-dark-bg-elevated"
				/>
				<span>
					<span class="block font-medium">Cascade to child beads</span>
					<span class="block text-body-sm text-error dark:text-dark-error">
						Apply the delete intent to {bead.childCount} child {bead.childCount === 1
							? 'bead'
							: 'beads'}.
					</span>
				</span>
			</label>
		{/if}
	</TypedConfirmDialog>
</div>
