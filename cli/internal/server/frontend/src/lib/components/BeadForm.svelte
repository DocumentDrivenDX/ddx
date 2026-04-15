<script lang="ts">
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

	const isUpdate = bead != null;

	let title = $state(bead?.title ?? '');
	let status = $state(bead?.status ?? 'open');
	let priority = $state(bead?.priority ?? 2);
	let issueType = $state(bead?.issueType ?? '');
	let labelsInput = $state((bead?.labels ?? []).join(', '));
	let parent = $state(bead?.parent ?? '');
	let description = $state(bead?.description ?? '');
	let acceptance = $state(bead?.acceptance ?? '');
	let notes = $state(bead?.notes ?? '');
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
		'w-full rounded-md border border-gray-300 bg-white px-3 py-2 text-sm text-gray-900 placeholder-gray-400 focus:border-blue-500 focus:outline-none focus:ring-1 focus:ring-blue-500 dark:border-gray-600 dark:bg-gray-800 dark:text-gray-100 dark:placeholder-gray-500 dark:focus:border-blue-400';
	const labelClass = 'block text-xs font-medium text-gray-700 dark:text-gray-300 mb-1';
</script>

<form onsubmit={handleSubmit} class="space-y-4">
	{#if error}
		<div
			class="rounded-md border border-red-200 bg-red-50 px-3 py-2 text-sm text-red-700 dark:border-red-800 dark:bg-red-900/30 dark:text-red-400"
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
			<select id="bead-status" bind:value={status} class={inputClass}>
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
			class="rounded-md border border-gray-300 px-4 py-2 text-sm text-gray-700 hover:bg-gray-50 dark:border-gray-600 dark:text-gray-300 dark:hover:bg-gray-800"
		>
			Cancel
		</button>
		<button
			type="submit"
			disabled={submitting}
			class="rounded-md bg-blue-600 px-4 py-2 text-sm font-medium text-white hover:bg-blue-700 disabled:cursor-not-allowed disabled:opacity-50"
		>
			{submitting ? 'Saving…' : isUpdate ? 'Save changes' : 'Create bead'}
		</button>
	</div>
</form>
