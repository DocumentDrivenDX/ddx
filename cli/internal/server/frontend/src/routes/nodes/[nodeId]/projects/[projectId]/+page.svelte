<script lang="ts">
	import type { PageData } from './$types';
	import { resolve } from '$app/paths';
	import { page } from '$app/stores';
	import ConfirmDialog from '$lib/components/ConfirmDialog.svelte';
	import OperatorPromptPanel from '$lib/components/OperatorPromptPanel.svelte';
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
			accentClass:
				'bg-accent-lever text-white hover:opacity-90 focus-visible:ring-accent-lever dark:bg-dark-accent-lever'
		},
		{
			id: 'align',
			kind: 'realign-specs',
			label: 'Re-align specs',
			shortLabel: 'Align',
			description: 'Run the HELIX alignment action against the project spec tree.',
			Icon: RefreshCcw,
			accentClass:
				'bg-accent-fulcrum text-white hover:opacity-90 focus-visible:ring-accent-fulcrum dark:bg-dark-accent-fulcrum'
		},
		{
			id: 'checks',
			kind: 'run-checks',
			label: 'Run checks',
			shortLabel: 'Checks',
			description: 'Run the project execution definitions and report their result.',
			Icon: ShieldCheck,
			accentClass:
				'bg-fg-ink text-bg-elevated hover:opacity-90 focus-visible:ring-fg-ink dark:bg-dark-fg-ink dark:text-dark-bg-canvas'
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
	<header class="border-b border-border-line pb-6 dark:border-dark-border-line">
		<div class="flex flex-wrap items-start justify-between gap-4">
			<div class="min-w-0">
				<p class="text-xs font-semibold tracking-widest text-accent-lever dark:text-dark-accent-lever uppercase">
					Project overview
				</p>
				<h1 class="mt-1 text-headline-lg font-bold tracking-tight text-fg-ink dark:text-dark-fg-ink">
					{projectName}
				</h1>
				{#if projectPath}
					<p class="mt-1 truncate font-mono-code text-mono-code text-fg-muted dark:text-dark-fg-muted">
						{projectPath}
					</p>
				{/if}
			</div>
			<div
				class="grid min-w-60 grid-cols-3 overflow-hidden rounded-md border border-border-line text-center dark:border-dark-border-line"
				aria-label="Queue summary"
			>
				<div class="px-4 py-3">
					<div class="text-headline-lg font-semibold text-fg-ink dark:text-dark-fg-ink">
						{queueSummary?.ready ?? '...'}
					</div>
					<div class="text-xs text-fg-muted dark:text-dark-fg-muted">Ready</div>
				</div>
				<div class="border-x border-border-line px-4 py-3 dark:border-dark-border-line">
					<div class="text-headline-lg font-semibold text-fg-ink dark:text-dark-fg-ink">
						{queueSummary?.blocked ?? '...'}
					</div>
					<div class="text-xs text-fg-muted dark:text-dark-fg-muted">Blocked</div>
				</div>
				<div class="px-4 py-3">
					<div class="text-headline-lg font-semibold text-fg-ink dark:text-dark-fg-ink">
						{queueSummary?.inProgress ?? '...'}
					</div>
					<div class="text-xs text-fg-muted dark:text-dark-fg-muted">In progress</div>
				</div>
			</div>
		</div>
	</header>

	{#if alertMessage}
		<div
			role="alert"
			class="rounded-md border border-error/30 bg-error/10 px-4 py-3 text-sm text-error dark:border-dark-error/30 dark:bg-dark-error/10 dark:text-dark-error"
		>
			{alertMessage}
		</div>
	{/if}

	<section role="region" aria-label="Actions" class="space-y-4">
		<div class="flex flex-wrap items-center justify-between gap-3 border-b border-border-line pb-4 dark:border-dark-border-line">
			<div>
				<h2 class="text-headline-md font-semibold text-fg-ink dark:text-dark-fg-ink">Actions</h2>
				<p class="mt-1 text-sm text-fg-muted dark:text-dark-fg-muted">{queueContext()}</p>
			</div>
			<button
				type="button"
				onclick={loadQueueSummary}
				class="rounded-md border border-border-line px-3 py-2 text-sm font-medium text-fg-ink hover:bg-bg-surface focus-visible:ring-2 focus-visible:ring-accent-lever focus-visible:outline-none dark:border-dark-border-line dark:text-dark-fg-ink dark:hover:bg-dark-bg-surface"
			>
				Refresh
			</button>
		</div>

		<div class="grid gap-3 md:grid-cols-3">
			{#each ACTIONS as action}
				{@const worker = dispatchedWorkers[action.id]}
				{@const reason = disabledReason(action)}
				<div class="rounded-md border border-border-line p-3 dark:border-dark-border-line">
					<div class="mb-3 flex items-start gap-3">
						<div
							class="flex h-9 w-9 shrink-0 items-center justify-center rounded-md bg-bg-surface text-fg-muted dark:bg-dark-bg-surface dark:text-dark-fg-muted"
						>
							<action.Icon class="h-4 w-4" aria-hidden="true" />
						</div>
						<div class="min-w-0">
							<h3 class="font-medium text-fg-ink dark:text-dark-fg-ink">{action.label}</h3>
							<p class="mt-1 text-sm leading-5 text-fg-muted dark:text-dark-fg-muted">
								{action.description}
							</p>
						</div>
					</div>

					{#if worker}
						<a
							href={workerHref(worker.id)}
							class="inline-flex min-h-10 w-full items-center justify-center gap-2 rounded-md border border-border-line px-3 py-2 text-sm font-medium text-fg-ink hover:bg-bg-surface focus-visible:ring-2 focus-visible:ring-accent-lever focus-visible:outline-none dark:border-dark-border-line dark:text-dark-fg-ink dark:hover:bg-dark-bg-surface"
						>
							<CheckCircle2 class="h-4 w-4 text-accent-fulcrum dark:text-dark-accent-fulcrum" />
							<span>{worker.id}</span>
						</a>
					{:else if reason}
						<Tooltip content={reason} disabledTrigger={true}>
							<button
								type="button"
								aria-disabled="true"
								class="inline-flex min-h-10 w-full items-center justify-center gap-2 rounded-md bg-bg-surface px-3 py-2 text-sm font-medium text-fg-muted aria-disabled:cursor-not-allowed dark:bg-dark-bg-surface dark:text-dark-fg-muted"
							>
								{action.label}
							</button>
						</Tooltip>
					{:else}
						<button
							type="button"
							onclick={(event) => openDialog(action, event)}
							disabled={dispatchingActionId === action.id}
							class="inline-flex min-h-10 w-full items-center justify-center gap-2 rounded-md px-3 py-2 text-sm font-medium focus-visible:ring-2 focus-visible:ring-offset-2 focus-visible:ring-offset-bg-canvas focus-visible:outline-none disabled:cursor-wait disabled:opacity-80 dark:focus-visible:ring-offset-dark-bg-canvas {action.accentClass}"
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

	<OperatorPromptPanel projectId={projectId()} nodeId={$page.params.nodeId ?? ''} />
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
			<div class="rounded-md bg-bg-surface px-3 py-2 dark:bg-dark-bg-surface">
				<dt class="text-xs text-fg-muted dark:text-dark-fg-muted">Ready</dt>
				<dd class="text-headline-md font-semibold text-fg-ink dark:text-dark-fg-ink">
					{queueSummary?.ready ?? 0}
				</dd>
			</div>
			<div class="rounded-md bg-bg-surface px-3 py-2 dark:bg-dark-bg-surface">
				<dt class="text-xs text-fg-muted dark:text-dark-fg-muted">Blocked</dt>
				<dd class="text-headline-md font-semibold text-fg-ink dark:text-dark-fg-ink">
					{queueSummary?.blocked ?? 0}
				</dd>
			</div>
			<div class="rounded-md bg-bg-surface px-3 py-2 dark:bg-dark-bg-surface">
				<dt class="text-xs text-fg-muted dark:text-dark-fg-muted">In progress</dt>
				<dd class="text-headline-md font-semibold text-fg-ink dark:text-dark-fg-ink">
					{queueSummary?.inProgress ?? 0}
				</dd>
			</div>
		</dl>
	</div>
</ConfirmDialog>
