<script lang="ts">
	import type { PageData } from './$types';
	import { resolve } from '$app/paths';
	import { page } from '$app/stores';
	import ConfirmDialog from '$lib/components/ConfirmDialog.svelte';
	import Tooltip from '$lib/components/Tooltip.svelte';
	import { createClient } from '$lib/gql/client';
	import { PROJECT_QUEUE_SUMMARY_QUERY, WORKER_DISPATCH_MUTATION } from '$lib/gql/feat008';
	import { CheckCircle2, Loader2, Play, RefreshCcw, ShieldCheck } from 'lucide-svelte';
	import { onMount } from 'svelte';

	type ActionId = 'drain' | 'align' | 'checks';
	type WorkerKind = 'execute-loop' | 'realign-specs' | 'run-checks';
	type IconComponent = typeof Play;

	interface QueueSummary {
		ready: number;
		blocked: number;
		inProgress: number;
	}

	interface QueueSummaryResult {
		queueSummary: QueueSummary;
	}

	interface WorkerDispatchResult {
		id: string;
		state: string;
		kind: string;
	}

	interface WorkerDispatchMutationResult {
		workerDispatch: WorkerDispatchResult;
	}

	interface ProjectAction {
		id: ActionId;
		kind: WorkerKind;
		label: string;
		shortLabel: string;
		description: string;
		Icon: IconComponent;
		accentClass: string;
	}

	const ACTIONS: ProjectAction[] = [
		{
			id: 'drain',
			kind: 'execute-loop',
			label: 'Drain queue',
			shortLabel: 'Drain',
			description: 'Attempt ready beads with the project execute-loop worker.',
			Icon: Play,
			accentClass: 'bg-blue-600 text-white hover:bg-blue-700 focus-visible:ring-blue-500'
		},
		{
			id: 'align',
			kind: 'realign-specs',
			label: 'Re-align specs',
			shortLabel: 'Align',
			description: 'Run the HELIX alignment action against the project spec tree.',
			Icon: RefreshCcw,
			accentClass: 'bg-emerald-600 text-white hover:bg-emerald-700 focus-visible:ring-emerald-500'
		},
		{
			id: 'checks',
			kind: 'run-checks',
			label: 'Run checks',
			shortLabel: 'Checks',
			description: 'Run the project execution definitions and report their result.',
			Icon: ShieldCheck,
			accentClass:
				'bg-gray-950 text-white hover:bg-gray-800 focus-visible:ring-gray-500 dark:bg-white dark:text-gray-950 dark:hover:bg-gray-200'
		}
	];

	let { data }: { data: PageData } = $props();

	let queueSummary = $state<QueueSummary | null>(null);
	let queueLoading = $state(true);
	let alertMessage = $state('');
	let dialogOpen = $state(false);
	let activeActionId = $state<ActionId>('drain');
	let dispatchingActionId = $state<ActionId | null>(null);
	let dispatchedWorkers = $state<Partial<Record<ActionId, WorkerDispatchResult>>>({});
	let returnFocusTo = $state<HTMLElement | null>(null);

	const activeAction = $derived(actionById(activeActionId));
	const projectName = $derived(data.project?.name ?? $page.params.projectId ?? 'Project');
	const projectPath = $derived(data.project?.path ?? '');

	onMount(() => {
		void loadQueueSummary();
	});

	function actionById(id: ActionId): ProjectAction {
		return ACTIONS.find((action) => action.id === id) ?? ACTIONS[0];
	}

	function projectId(): string {
		return $page.params.projectId ?? data.project?.id ?? '';
	}

	function workerHref(workerId: string): string {
		return resolve('/nodes/[nodeId]/projects/[projectId]/workers/[workerId]', {
			nodeId: $page.params.nodeId!,
			projectId: projectId(),
			workerId
		});
	}

	async function loadQueueSummary() {
		queueLoading = true;
		alertMessage = '';
		try {
			const client = createClient();
			const result = await client.request<QueueSummaryResult>(PROJECT_QUEUE_SUMMARY_QUERY, {
				projectId: projectId()
			});
			queueSummary = result.queueSummary;
		} catch (err) {
			alertMessage = `Could not load queue summary. ${errorText(err)}`;
		} finally {
			queueLoading = false;
		}
	}

	function openDialog(action: ProjectAction, event: MouseEvent) {
		activeActionId = action.id;
		returnFocusTo =
			event.currentTarget instanceof HTMLElement ? event.currentTarget : returnFocusTo;
		alertMessage = '';
		dialogOpen = true;
	}

	async function confirmDispatch() {
		const action = activeAction;
		dispatchingActionId = action.id;
		alertMessage = '';
		try {
			const client = createClient();
			const result = await client.request<WorkerDispatchMutationResult>(WORKER_DISPATCH_MUTATION, {
				kind: action.kind,
				projectId: projectId(),
				args: actionArgs(action)
			});
			dispatchedWorkers = {
				...dispatchedWorkers,
				[action.id]: result.workerDispatch
			};
		} catch (err) {
			alertMessage = `${errorText(err)} Try the Workers page to inspect active ${action.kind} workers before dispatching again.`;
		} finally {
			dispatchingActionId = null;
		}
	}

	function actionArgs(action: ProjectAction): string {
		return JSON.stringify({
			source: 'project-overview-actions',
			action: action.id
		});
	}

	function disabledReason(action: ProjectAction): string {
		if (queueLoading) return 'Loading queue summary';
		if (action.id === 'drain' && (queueSummary?.ready ?? 0) === 0) {
			return 'No ready beads are available to drain.';
		}
		return '';
	}

	function queueContext(): string {
		const summary = queueSummary ?? { ready: 0, blocked: 0, inProgress: 0 };
		return `${summary.ready} ready ${plural(summary.ready, 'bead')}, ${summary.blocked} blocked, ${summary.inProgress} in progress`;
	}

	function actionScope(action: ProjectAction): string {
		if (action.id === 'drain') {
			return `${queueSummary?.ready ?? 0} ready ${plural(queueSummary?.ready ?? 0, 'bead')} will be attempted.`;
		}
		if (action.id === 'align') {
			return `The HELIX alignment worker will run with current queue context: ${queueContext()}.`;
		}
		return `The project check suite will run with current queue context: ${queueContext()}.`;
	}

	function plural(count: number, singular: string): string {
		return count === 1 ? singular : `${singular}s`;
	}

	function errorText(err: unknown): string {
		if (err instanceof Error) return err.message;
		if (typeof err === 'string') return err;
		return 'Unknown error.';
	}
</script>

<svelte:head>
	<title>{projectName} | DDx</title>
</svelte:head>

<div class="space-y-6">
	<header class="border-b border-gray-200 pb-6 dark:border-gray-700">
		<div class="flex flex-wrap items-start justify-between gap-4">
			<div class="min-w-0">
				<p class="text-xs font-semibold tracking-widest text-lever uppercase">
					Project overview
				</p>
				<h1 class="mt-1 text-3xl font-bold tracking-tight text-gray-950 dark:text-white">
					{projectName}
				</h1>
				{#if projectPath}
					<p class="mt-1 truncate font-mono text-xs text-gray-500 dark:text-gray-400">
						{projectPath}
					</p>
				{/if}
			</div>
			<div
				class="grid min-w-60 grid-cols-3 overflow-hidden rounded-md border border-gray-200 text-center dark:border-gray-700"
				aria-label="Queue summary"
			>
				<div class="px-4 py-3">
					<div class="text-xl font-semibold text-gray-950 dark:text-white">
						{queueSummary?.ready ?? '...'}
					</div>
					<div class="text-xs text-gray-500 dark:text-gray-400">Ready</div>
				</div>
				<div class="border-x border-gray-200 px-4 py-3 dark:border-gray-700">
					<div class="text-xl font-semibold text-gray-950 dark:text-white">
						{queueSummary?.blocked ?? '...'}
					</div>
					<div class="text-xs text-gray-500 dark:text-gray-400">Blocked</div>
				</div>
				<div class="px-4 py-3">
					<div class="text-xl font-semibold text-gray-950 dark:text-white">
						{queueSummary?.inProgress ?? '...'}
					</div>
					<div class="text-xs text-gray-500 dark:text-gray-400">In progress</div>
				</div>
			</div>
		</div>
	</header>

	{#if alertMessage}
		<div
			role="alert"
			class="rounded-md border border-red-200 bg-red-50 px-4 py-3 text-sm text-red-900 dark:border-red-900/70 dark:bg-red-950/30 dark:text-red-100"
		>
			{alertMessage}
		</div>
	{/if}

	<section role="region" aria-label="Actions" class="space-y-4">
		<div class="flex flex-wrap items-center justify-between gap-3 border-b border-gray-200 pb-4 dark:border-gray-700">
			<div>
				<h2 class="text-lg font-semibold text-gray-950 dark:text-white">Actions</h2>
				<p class="mt-1 text-sm text-gray-600 dark:text-gray-300">{queueContext()}</p>
			</div>
			<button
				type="button"
				onclick={loadQueueSummary}
				class="rounded-md border border-gray-300 px-3 py-2 text-sm font-medium text-gray-700 hover:bg-gray-50 focus-visible:ring-2 focus-visible:ring-blue-500 focus-visible:outline-none dark:border-gray-700 dark:text-gray-200 dark:hover:bg-gray-800"
			>
				Refresh
			</button>
		</div>

		<div class="grid gap-3 md:grid-cols-3">
			{#each ACTIONS as action}
				{@const worker = dispatchedWorkers[action.id]}
				{@const reason = disabledReason(action)}
				<div class="rounded-md border border-gray-200 p-3 dark:border-gray-800">
					<div class="mb-3 flex items-start gap-3">
						<div
							class="flex h-9 w-9 shrink-0 items-center justify-center rounded-md bg-gray-100 text-gray-700 dark:bg-gray-800 dark:text-gray-200"
						>
							<action.Icon class="h-4 w-4" aria-hidden="true" />
						</div>
						<div class="min-w-0">
							<h3 class="font-medium text-gray-950 dark:text-white">{action.label}</h3>
							<p class="mt-1 text-sm leading-5 text-gray-600 dark:text-gray-300">
								{action.description}
							</p>
						</div>
					</div>

					{#if worker}
						<a
							href={workerHref(worker.id)}
							class="inline-flex min-h-10 w-full items-center justify-center gap-2 rounded-md border border-gray-300 px-3 py-2 text-sm font-medium text-gray-900 hover:bg-gray-50 focus-visible:ring-2 focus-visible:ring-blue-500 focus-visible:outline-none dark:border-gray-700 dark:text-gray-100 dark:hover:bg-gray-800"
						>
							<CheckCircle2 class="h-4 w-4 text-emerald-600 dark:text-emerald-400" />
							<span>{worker.id}</span>
						</a>
					{:else if reason}
						<Tooltip content={reason} disabledTrigger={true}>
							<button
								type="button"
								aria-disabled="true"
								class="inline-flex min-h-10 w-full items-center justify-center gap-2 rounded-md bg-gray-200 px-3 py-2 text-sm font-medium text-gray-500 aria-disabled:cursor-not-allowed dark:bg-gray-800 dark:text-gray-500"
							>
								{action.label}
							</button>
						</Tooltip>
					{:else}
						<button
							type="button"
							onclick={(event) => openDialog(action, event)}
							disabled={dispatchingActionId === action.id}
							class="inline-flex min-h-10 w-full items-center justify-center gap-2 rounded-md px-3 py-2 text-sm font-medium focus-visible:ring-2 focus-visible:ring-offset-2 focus-visible:ring-offset-white focus-visible:outline-none disabled:cursor-wait disabled:opacity-80 dark:focus-visible:ring-offset-gray-900 {action.accentClass}"
						>
							{#if dispatchingActionId === action.id}
								<Loader2 class="h-4 w-4 animate-spin" aria-hidden="true" />
								Starting...
							{:else}
								{action.label}
							{/if}
						</button>
					{/if}
				</div>
			{/each}
		</div>
	</section>
</div>

<ConfirmDialog
	bind:open={dialogOpen}
	actionLabel={`Start ${activeAction.label}`}
	title={activeAction.label}
	{returnFocusTo}
	confirmDisabled={Boolean(disabledReason(activeAction))}
	onConfirm={confirmDispatch}
>
	{#snippet summary()}
		<span>{actionScope(activeAction)}</span>
	{/snippet}

	<div class="space-y-3">
		<p>
			{activeAction.description}
		</p>
		<dl class="grid grid-cols-3 gap-2 text-center">
			<div class="rounded-md bg-gray-100 px-3 py-2 dark:bg-gray-800">
				<dt class="text-xs text-gray-500 dark:text-gray-400">Ready</dt>
				<dd class="text-lg font-semibold text-gray-950 dark:text-white">
					{queueSummary?.ready ?? 0}
				</dd>
			</div>
			<div class="rounded-md bg-gray-100 px-3 py-2 dark:bg-gray-800">
				<dt class="text-xs text-gray-500 dark:text-gray-400">Blocked</dt>
				<dd class="text-lg font-semibold text-gray-950 dark:text-white">
					{queueSummary?.blocked ?? 0}
				</dd>
			</div>
			<div class="rounded-md bg-gray-100 px-3 py-2 dark:bg-gray-800">
				<dt class="text-xs text-gray-500 dark:text-gray-400">In progress</dt>
				<dd class="text-lg font-semibold text-gray-950 dark:text-white">
					{queueSummary?.inProgress ?? 0}
				</dd>
			</div>
		</dl>
	</div>
</ConfirmDialog>
