<script lang="ts">
	import { untrack } from 'svelte';
	import { gql } from 'graphql-request';
	import { createClient } from '$lib/gql/client';

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
	}

	let {
		bead = null,
		onSuccess,
		onCancel
	}: {
		bead?: Bead | null;
		onSuccess: (bead: Bead) => void;
		onCancel: () => void;
	} = $props();

	const isUpdate = $derived(bead != null);

	let title = $state(untrack(() => bead?.title ?? ''));
	let status = $state(untrack(() => bead?.status ?? 'open'));
	let priority = $state(untrack(() => bead?.priority ?? 2));
	let issueType = $state(untrack(() => bead?.issueType ?? ''));
	let labelsInput = $state(untrack(() => (bead?.labels ?? []).join(', ')));
	let parent = $state(untrack(() => bead?.parent ?? ''));
	let description = $state(untrack(() => bead?.description ?? ''));
	let acceptance = $state(untrack(() => bead?.acceptance ?? ''));
	let notes = $state(untrack(() => bead?.notes ?? ''));
	let submitting = $state(false);
	let error = $state<string | null>(null);

	const CREATE_MUTATION = gql`
		mutation BeadCreate($input: BeadInput!) {
			beadCreate(input: $input) {
				id title status priority issueType owner createdAt createdBy updatedAt labels parent description acceptance notes
				dependencies { issueId dependsOnId type createdAt createdBy }
			}
		}
	`;

	const UPDATE_MUTATION = gql`
		mutation BeadUpdate($id: ID!, $input: BeadUpdateInput!) {
			beadUpdate(id: $id, input: $input) {
				id title status priority issueType owner createdAt createdBy updatedAt labels parent description acceptance notes
				dependencies { issueId dependsOnId type createdAt createdBy }
			}
		}
	`;

	async function handleSubmit(e: SubmitEvent) {
		e.preventDefault();
		if (!title.trim()) {
			error = 'Title is required';
			return;
		}
		submitting = true;
		error = null;

		const labels = labelsInput
			.split(',')
			.map((l) => l.trim())
			.filter(Boolean);

		try {
			const client = createClient();
			if (isUpdate && bead) {
				const result = await client.request<{ beadUpdate: Bead }>(UPDATE_MUTATION, {
					id: bead.id,
					input: {
						title: title || undefined,
						status: status || undefined,
						priority: priority,
						issueType: issueType || undefined,
						labels: labels.length ? labels : undefined,
						parent: parent || undefined,
						description: description || undefined,
						acceptance: acceptance || undefined,
						notes: notes || undefined
					}
				});
				onSuccess(result.beadUpdate);
			} else {
				const result = await client.request<{ beadCreate: Bead }>(CREATE_MUTATION, {
					input: {
						title,
						status,
						priority,
						issueType: issueType || undefined,
						labels: labels.length ? labels : undefined,
						parent: parent || undefined,
						description: description || undefined,
						acceptance: acceptance || undefined,
						notes: notes || undefined
					}
				});
				onSuccess(result.beadCreate);
			}
		} catch (e) {
			error = e instanceof Error ? e.message : 'Operation failed';
		} finally {
			submitting = false;
		}
	}

	const inputClass =
		'w-full rounded-none border border-border-line bg-bg-elevated px-3 py-2 text-body-sm text-fg-ink placeholder-fg-muted focus:border-accent-lever focus:outline-none focus:ring-1 focus:ring-accent-lever dark:border-dark-border-line dark:bg-dark-bg-elevated dark:text-dark-fg-ink dark:placeholder-dark-fg-muted dark:focus:border-dark-accent-lever';
	const labelClass =
		'mb-1 block font-label-caps text-label-caps uppercase text-fg-muted dark:text-dark-fg-muted';

	function statusBadgeClass(s: string): string {
		switch (s) {
			case 'open':
				return 'badge-status-open';
			case 'in-progress':
				return 'badge-status-in-progress';
			case 'blocked':
				return 'badge-status-blocked';
			case 'closed':
				return 'badge-status-closed';
			default:
				return 'badge-status-neutral';
		}
	}
</script>

<form onsubmit={handleSubmit} class="space-y-4">
	{#if error}
		<div
			class="rounded-none border border-error/30 bg-error/10 px-3 py-2 text-sm text-error dark:border-dark-error/30 dark:bg-dark-error/10 dark:text-dark-error"
		>
			{error}
		</div>
	{/if}

	<div>
		<label for="bead-title" class={labelClass}>Title *</label>
		<input
			id="bead-title"
			type="text"
			bind:value={title}
			placeholder="Short summary of the work"
			class={inputClass}
			required
		/>
	</div>

	<div class="grid grid-cols-2 gap-3">
		<div>
			<label for="bead-status" class={labelClass}>Status</label>
			<select
				id="bead-status"
				bind:value={status}
				class="{inputClass} {statusBadgeClass(status)}"
			>
				<option value="open">open</option>
				<option value="in-progress">in-progress</option>
				<option value="blocked">blocked</option>
				<option value="closed">closed</option>
			</select>
		</div>
		<div>
			<label for="bead-priority" class={labelClass}>Priority</label>
			<input
				id="bead-priority"
				type="number"
				bind:value={priority}
				min="1"
				max="5"
				class={inputClass}
			/>
		</div>
	</div>

	<div class="grid grid-cols-2 gap-3">
		<div>
			<label for="bead-type" class={labelClass}>Type</label>
			<input
				id="bead-type"
				type="text"
				bind:value={issueType}
				placeholder="task, bug, feature…"
				class={inputClass}
			/>
		</div>
		<div>
			<label for="bead-parent" class={labelClass}>Parent</label>
			<input
				id="bead-parent"
				type="text"
				bind:value={parent}
				placeholder="Parent bead ID"
				class={inputClass}
			/>
		</div>
	</div>

	<div>
		<label for="bead-labels" class={labelClass}>Labels</label>
		<input
			id="bead-labels"
			type="text"
			bind:value={labelsInput}
			placeholder="Comma-separated labels"
			class={inputClass}
		/>
	</div>

	<div>
		<label for="bead-description" class={labelClass}>Description</label>
		<textarea
			id="bead-description"
			bind:value={description}
			rows={4}
			placeholder="Full description / body text"
			class="{inputClass} resize-y"
		></textarea>
	</div>

	<div>
		<label for="bead-acceptance" class={labelClass}>Acceptance Criteria</label>
		<textarea
			id="bead-acceptance"
			bind:value={acceptance}
			rows={3}
			placeholder="Acceptance criteria"
			class="{inputClass} resize-y"
		></textarea>
	</div>

	<div>
		<label for="bead-notes" class={labelClass}>Notes</label>
		<textarea
			id="bead-notes"
			bind:value={notes}
			rows={2}
			placeholder="Freeform notes"
			class="{inputClass} resize-y"
		></textarea>
	</div>

	<div class="flex justify-end gap-2 pt-2">
		<button
			type="button"
			onclick={onCancel}
			class="rounded-none border border-border-line px-4 py-2 text-body-sm text-fg-muted hover:bg-bg-canvas dark:border-dark-border-line dark:text-dark-fg-muted dark:hover:bg-dark-bg-canvas"
		>
			Cancel
		</button>
		<button
			type="submit"
			disabled={submitting}
			class="rounded-none bg-accent-lever px-4 py-2 text-body-sm font-medium text-white hover:bg-accent-lever/90 disabled:cursor-not-allowed disabled:opacity-50 dark:bg-dark-accent-lever dark:hover:bg-dark-accent-lever/90"
		>
			{submitting ? 'Saving…' : isUpdate ? 'Save changes' : 'Create bead'}
		</button>
	</div>
</form>
