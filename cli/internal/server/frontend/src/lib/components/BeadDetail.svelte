<script lang="ts">
	import { gql } from 'graphql-request';
	import { createClient } from '$lib/gql/client';
	import { invalidateAll } from '$app/navigation';
	import { nodeStore } from '$lib/stores/node.svelte';
	import { X, UserPlus, UserMinus, Pencil, Trash2 } from 'lucide-svelte';
	import BeadForm from './BeadForm.svelte';
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

	let {
		bead: initialBead,
		onClose,
		executions = [],
		nodeId = '',
		projectId = ''
	}: {
		bead: Bead;
		onClose: () => void;
		executions?: BeadExecutionRow[];
		nodeId?: string;
		projectId?: string;
	} = $props();

	function executionHref(executionId: string): string {
		return `/nodes/${nodeId}/projects/${projectId}/executions/${executionId}`;
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
	let deleteButton = $state<HTMLButtonElement | null>(null);
	const hasChildBeads = $derived((bead.childCount ?? 0) > 0);

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

	async function handleClaim() {
		busy = true;
		actionError = null;
		try {
			const client = createClient();
			const assignee = nodeStore.value?.name ?? 'user';
			const result = await client.request<{ beadClaim: Bead }>(CLAIM_MUTATION, {
				id: bead.id,
				assignee
			});
			bead = result.beadClaim;
			invalidateAll();
		} catch (e) {
			actionError = e instanceof Error ? e.message : 'Claim failed';
		} finally {
			busy = false;
		}
	}

	async function handleUnclaim() {
		busy = true;
		actionError = null;
		try {
			const client = createClient();
			const result = await client.request<{ beadUnclaim: Bead }>(UNCLAIM_MUTATION, {
				id: bead.id
			});
			bead = result.beadUnclaim;
			invalidateAll();
		} catch (e) {
			actionError = e instanceof Error ? e.message : 'Unclaim failed';
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
			const client = createClient();
			await client.request<{ beadClose: Bead }>(CLOSE_MUTATION, {
				id: bead.id,
				reason: 'deleted via UI'
			});
			await invalidateAll();
			onClose();
		} catch (e) {
			actionError = e instanceof Error ? e.message : 'Delete failed';
		} finally {
			busy = false;
		}
	}

	function statusClass(status: string): string {
		switch (status) {
			case 'open':
				return 'text-blue-600 dark:text-blue-400';
			case 'in-progress':
				return 'text-yellow-600 dark:text-yellow-400';
			case 'closed':
				return 'text-green-600 dark:text-green-400';
			case 'blocked':
				return 'text-red-600 dark:text-red-400';
			default:
				return 'text-gray-500 dark:text-gray-400';
		}
	}
</script>

<!-- Right-side detail panel -->
<div
	class="fixed top-0 right-0 z-50 flex h-full w-full max-w-xl flex-col bg-white shadow-xl dark:bg-gray-900"
>
	<!-- Header -->
	<div
		class="flex shrink-0 items-center justify-between border-b border-gray-200 px-6 py-4 dark:border-gray-700"
	>
		<div class="flex min-w-0 items-center gap-3">
			<span class="shrink-0 font-mono text-xs text-gray-500 dark:text-gray-400">{bead.id}</span>
			<span class="shrink-0 font-medium {statusClass(bead.status)}">{bead.status}</span>
			{#if bead.owner}
				<span class="truncate text-xs text-gray-500 dark:text-gray-400">@ {bead.owner}</span>
			{/if}
		</div>
		<div class="ml-3 flex shrink-0 items-center gap-2">
			{#if !editing}
				{#if bead.status === 'open' || bead.status === 'blocked'}
					<button
						onclick={handleClaim}
						disabled={busy}
						class="flex items-center gap-1.5 rounded-md bg-blue-600 px-3 py-1.5 text-sm font-medium text-white hover:bg-blue-700 disabled:cursor-not-allowed disabled:opacity-50"
					>
						<UserPlus class="h-3.5 w-3.5" />
						Claim
					</button>
				{:else if bead.status === 'in-progress'}
					<button
						onclick={handleUnclaim}
						disabled={busy}
						class="flex items-center gap-1.5 rounded-md border border-gray-300 px-3 py-1.5 text-sm font-medium text-gray-700 hover:bg-gray-50 disabled:cursor-not-allowed disabled:opacity-50 dark:border-gray-600 dark:text-gray-300 dark:hover:bg-gray-800"
					>
						<UserMinus class="h-3.5 w-3.5" />
						Unclaim
					</button>
				{/if}
				<button
					onclick={() => (editing = true)}
					disabled={busy}
					class="flex items-center gap-1.5 rounded-md border border-gray-300 px-3 py-1.5 text-sm font-medium text-gray-700 hover:bg-gray-50 dark:border-gray-600 dark:text-gray-300 dark:hover:bg-gray-800"
				>
					<Pencil class="h-3.5 w-3.5" />
					Edit
				</button>
				<button
					bind:this={deleteButton}
					onclick={openDeleteDialog}
					disabled={busy}
					class="flex items-center gap-1.5 rounded-md border border-red-300 px-3 py-1.5 text-sm font-medium text-red-700 hover:bg-red-50 disabled:cursor-not-allowed disabled:opacity-50 dark:border-red-800 dark:text-red-300 dark:hover:bg-red-950/40"
				>
					<Trash2 class="h-3.5 w-3.5" />
					Delete
				</button>
			{/if}
			<button
				onclick={onClose}
				class="rounded p-1.5 text-gray-500 hover:bg-gray-100 dark:text-gray-400 dark:hover:bg-gray-800"
				aria-label="Close panel"
			>
				<X class="h-4 w-4" />
			</button>
		</div>
	</div>

	<!-- Action error banner -->
	{#if actionError}
		<div
			class="shrink-0 border-b border-red-200 bg-red-50 px-6 py-2 text-sm text-red-700 dark:border-red-800 dark:bg-red-900/30 dark:text-red-400"
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
					onSuccess={(updated) => {
						bead = updated;
						editing = false;
					}}
					onCancel={() => (editing = false)}
				/>
			{/key}
		{:else}
			<!-- Read mode -->
			<h2 class="mb-5 text-xl font-semibold text-gray-900 dark:text-white">{bead.title}</h2>

			<dl class="space-y-4 text-sm">
				<div class="grid grid-cols-2 gap-4">
					<div>
						<dt
							class="text-xs font-medium tracking-wide text-gray-500 uppercase dark:text-gray-400"
						>
							Priority
						</dt>
						<dd class="mt-1 text-gray-900 dark:text-gray-100">{bead.priority}</dd>
					</div>
					<div>
						<dt
							class="text-xs font-medium tracking-wide text-gray-500 uppercase dark:text-gray-400"
						>
							Type
						</dt>
						<dd class="mt-1 text-gray-900 dark:text-gray-100">{bead.issueType || '—'}</dd>
					</div>
					{#if bead.parent}
						<div class="col-span-2">
							<dt
								class="text-xs font-medium tracking-wide text-gray-500 uppercase dark:text-gray-400"
							>
								Parent
							</dt>
							<dd class="mt-1 font-mono text-xs text-gray-500 dark:text-gray-400">{bead.parent}</dd>
						</div>
					{/if}
				</div>

				{#if bead.labels && bead.labels.length > 0}
					<div>
						<dt
							class="text-xs font-medium tracking-wide text-gray-500 uppercase dark:text-gray-400"
						>
							Labels
						</dt>
						<dd class="mt-1 flex flex-wrap gap-1">
							{#each bead.labels as label}
								<span
									class="rounded-full bg-gray-100 px-2 py-0.5 text-xs text-gray-700 dark:bg-gray-800 dark:text-gray-300"
									>{label}</span
								>
							{/each}
						</dd>
					</div>
				{/if}

				{#if bead.description}
					<div>
						<dt
							class="text-xs font-medium tracking-wide text-gray-500 uppercase dark:text-gray-400"
						>
							Description
						</dt>
						<dd class="mt-1 whitespace-pre-wrap text-gray-700 dark:text-gray-300">
							{bead.description}
						</dd>
					</div>
				{/if}

				{#if bead.acceptance}
					<div>
						<dt
							class="text-xs font-medium tracking-wide text-gray-500 uppercase dark:text-gray-400"
						>
							Acceptance
						</dt>
						<dd class="mt-1 whitespace-pre-wrap text-gray-700 dark:text-gray-300">
							{bead.acceptance}
						</dd>
					</div>
				{/if}

				{#if bead.notes}
					<div>
						<dt
							class="text-xs font-medium tracking-wide text-gray-500 uppercase dark:text-gray-400"
						>
							Notes
						</dt>
						<dd class="mt-1 whitespace-pre-wrap text-gray-700 dark:text-gray-300">{bead.notes}</dd>
					</div>
				{/if}

				{#if executions.length > 0}
					<div data-testid="bead-executions">
						<dt class="text-xs font-medium tracking-wide text-gray-500 uppercase dark:text-gray-400">
							Executions ({executions.length})
						</dt>
						<dd class="mt-1 space-y-1">
							{#each executions as exec (exec.id)}
								<a
									href={executionHref(exec.id)}
									class="flex items-center justify-between rounded border border-gray-200 px-2 py-1 text-xs hover:bg-gray-50 dark:border-gray-700 dark:hover:bg-gray-800"
								>
									<span class="flex items-center gap-2">
										<span class="font-mono text-gray-700 dark:text-gray-200">{exec.id}</span>
										{#if exec.verdict}
											<span class="rounded border px-1 py-0.5 text-[10px] uppercase">
												{exec.verdict}
											</span>
										{/if}
									</span>
									<span class="text-gray-500 dark:text-gray-400">{fmtExecDate(exec.createdAt)}</span>
								</a>
							{/each}
						</dd>
					</div>
				{/if}

				{#if bead.dependencies && bead.dependencies.length > 0}
					<div>
						<dt
							class="text-xs font-medium tracking-wide text-gray-500 uppercase dark:text-gray-400"
						>
							Dependencies
						</dt>
						<dd class="mt-1 space-y-1">
							{#each bead.dependencies as dep}
								<div class="font-mono text-xs text-gray-500 dark:text-gray-400">
									{dep.dependsOnId}
									<span class="text-gray-400">({dep.type})</span>
								</div>
							{/each}
						</dd>
					</div>
				{/if}

				<div
					class="border-t border-gray-100 pt-4 text-xs text-gray-400 dark:border-gray-800 dark:text-gray-500"
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
				class="mt-4 flex items-start gap-3 rounded-md border border-red-200 bg-red-50 p-3 text-sm text-red-900 dark:border-red-900/70 dark:bg-red-950/30 dark:text-red-100"
			>
				<input
					type="checkbox"
					bind:checked={cascadeToChildren}
					class="mt-1 h-4 w-4 rounded border-red-300 text-red-600 focus:ring-red-500 dark:border-red-700 dark:bg-gray-900"
				/>
				<span>
					<span class="block font-medium">Cascade to child beads</span>
					<span class="block text-xs text-red-700 dark:text-red-300">
						Apply the delete intent to {bead.childCount} child {bead.childCount === 1
							? 'bead'
							: 'beads'}.
					</span>
				</span>
			</label>
		{/if}
	</TypedConfirmDialog>
</div>
