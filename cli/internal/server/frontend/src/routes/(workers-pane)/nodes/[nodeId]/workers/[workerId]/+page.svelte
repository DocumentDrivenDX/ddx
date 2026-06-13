<script lang="ts">
	import type { PageData } from './$types';
	import { goto, invalidateAll } from '$app/navigation';
	import { page } from '$app/stores';
	import { createClient } from '$lib/gql/client';
	import { gql } from 'graphql-request';
	import type { WorkerLifecycleEvent } from './+page';

	let { data }: { data: PageData } = $props();

	let logLines = $state<string[]>([]);
	let logContainer = $state<HTMLPreElement | null>(null);
	let autoScroll = $state(true);
	let stopping = $state(false);
	let stopError = $state<string | null>(null);

	const STOP_WORKER_MUTATION = gql`
		mutation StopWorker($id: ID!) {
			stopWorker(id: $id) {
				id
				state
				kind
			}
		}
	`;

	$effect(() => {
		const raw = data.initialLog ?? '';
		logLines = raw.length > 0 ? raw.split('\n') : [];
	});

	$effect(() => {
		if (autoScroll && logContainer) {
			Promise.resolve().then(() => {
				if (logContainer) logContainer.scrollTop = logContainer.scrollHeight;
			});
		}
	});

	function handleScroll() {
		if (!logContainer) return;
		const distFromBottom =
			logContainer.scrollHeight - logContainer.scrollTop - logContainer.clientHeight;
		autoScroll = distFromBottom < 20;
	}

	function handleClose() {
		const p = $page.params as Record<string, string>;
		goto(`/nodes/${p['nodeId']}/workers`);
	}

	function formatElapsed(ms: number): string {
		if (ms < 1000) return `${ms}ms`;
		if (ms < 60000) return `${(ms / 1000).toFixed(1)}s`;
		const m = Math.floor(ms / 60000);
		const s = Math.floor((ms % 60000) / 1000);
		return `${m}m${s}s`;
	}

	function fmtDate(value: string): string {
		const date = new Date(value);
		if (Number.isNaN(date.getTime())) return value;
		return date.toLocaleString();
	}

	const lifecycleEvents = $derived(data.worker?.lifecycleEvents ?? ([] as WorkerLifecycleEvent[]));

	const isRunning = $derived(data.worker?.state === 'running');

	async function stopWorker() {
		if (!data.worker) return;
		if (!window.confirm(`Stop worker ${data.worker.id}?`)) return;
		stopError = null;
		stopping = true;
		try {
			const client = createClient(fetch);
			await client.request(STOP_WORKER_MUTATION, { id: data.worker.id });
			await invalidateAll();
		} catch (err) {
			stopError = err instanceof Error ? err.message : 'Stop failed.';
		} finally {
			stopping = false;
		}
	}
</script>

{#if data.worker}
	<!-- Backdrop -->
	<div
		class="fixed inset-0 z-40 bg-black/20 dark:bg-black/40"
		onclick={handleClose}
		role="button"
		tabindex="-1"
		aria-label="Dismiss panel"
		onkeydown={(e) => e.key === 'Escape' && handleClose()}
	></div>

	<!-- Detail panel -->
	<div
		class="fixed top-0 right-0 z-50 flex h-full w-full max-w-2xl flex-col bg-bg-elevated shadow-xl dark:bg-dark-bg-elevated"
	>
		<!-- Header -->
		<div
			class="flex shrink-0 items-center justify-between border-b border-border-line px-6 py-4 dark:border-dark-border-line"
		>
			<div>
				<h2 class="text-headline-md font-headline-md text-fg-ink dark:text-dark-fg-ink">
					{data.worker.kind}
				</h2>
				<p class="font-mono-code text-mono-code text-fg-muted dark:text-dark-fg-muted">
					{data.worker.id}
				</p>
			</div>
			<div class="flex items-center gap-2">
				{#if isRunning}
					<button
						type="button"
						onclick={() => void stopWorker()}
						disabled={stopping}
						class="border border-border-line px-3 py-1.5 text-body-sm font-medium text-status-failed hover:bg-bg-canvas disabled:cursor-not-allowed disabled:opacity-60 dark:border-dark-border-line dark:hover:bg-dark-bg-surface"
					>
						{stopping ? 'Stopping…' : 'Stop'}
					</button>
				{/if}
				<button
					onclick={handleClose}
					class="p-1.5 text-fg-muted hover:bg-bg-surface hover:text-fg-ink dark:text-dark-fg-muted dark:hover:bg-dark-bg-surface dark:hover:text-dark-fg-ink"
					aria-label="Close"
				>
					✕
				</button>
			</div>
		</div>

		{#if stopError}
			<div class="shrink-0 border-b border-border-line px-6 py-2 text-body-sm text-status-failed dark:border-dark-border-line">
				{stopError}
			</div>
		{/if}

		<!-- Metadata grid -->
		<div
			class="grid shrink-0 grid-cols-2 gap-x-6 gap-y-3 border-b border-border-line px-6 py-4 dark:border-dark-border-line"
		>
			<div>
				<div class="text-label-caps font-label-caps uppercase tracking-wide text-fg-muted dark:text-dark-fg-muted">State</div>
				<div class="mt-0.5 text-body-md font-medium text-fg-ink dark:text-dark-fg-ink">
					{data.worker.state}
				</div>
			</div>
			{#if data.worker.harness}
				<div>
					<div class="text-label-caps font-label-caps uppercase tracking-wide text-fg-muted dark:text-dark-fg-muted">Harness</div>
					<div class="mt-0.5 text-body-md text-fg-ink dark:text-dark-fg-ink">{data.worker.harness}</div>
				</div>
			{/if}
			{#if data.worker.model}
				<div>
					<div class="text-label-caps font-label-caps uppercase tracking-wide text-fg-muted dark:text-dark-fg-muted">Model</div>
					<div class="mt-0.5 text-body-md text-fg-ink dark:text-dark-fg-ink">{data.worker.model}</div>
				</div>
			{/if}
			{#if data.worker.effort}
				<div>
					<div class="text-label-caps font-label-caps uppercase tracking-wide text-fg-muted dark:text-dark-fg-muted">Effort</div>
					<div class="mt-0.5 text-body-md text-fg-ink dark:text-dark-fg-ink">{data.worker.effort}</div>
				</div>
			{/if}
			{#if data.worker.currentBead}
				<div class="col-span-2">
					<div class="text-label-caps font-label-caps uppercase tracking-wide text-fg-muted dark:text-dark-fg-muted">Current bead</div>
					<div class="mt-0.5 font-mono-code text-mono-code text-fg-ink dark:text-dark-fg-ink">
						{data.worker.currentBead}
					</div>
				</div>
			{/if}
			{#if data.worker.currentAttempt}
				<div>
					<div class="text-label-caps font-label-caps uppercase tracking-wide text-fg-muted dark:text-dark-fg-muted">Attempt ID</div>
					<div class="mt-0.5 font-mono-code text-mono-code text-fg-ink dark:text-dark-fg-ink">
						{data.worker.currentAttempt.attemptId}
					</div>
				</div>
				<div>
					<div class="text-label-caps font-label-caps uppercase tracking-wide text-fg-muted dark:text-dark-fg-muted">Phase</div>
					<div class="mt-0.5">
						<span class="text-body-md font-medium text-status-in-progress">
							{data.worker.currentAttempt.phase}
						</span>
						<span class="ml-1 font-mono-code text-mono-code text-fg-muted dark:text-dark-fg-muted">
							({formatElapsed(data.worker.currentAttempt.elapsedMs)})
						</span>
					</div>
				</div>
			{/if}
			{#if data.worker.attempts != null}
				<div>
					<div class="text-label-caps font-label-caps uppercase tracking-wide text-fg-muted dark:text-dark-fg-muted">Attempts</div>
					<div class="mt-0.5 text-body-md text-fg-ink dark:text-dark-fg-ink">
						{data.worker.attempts}
						<span class="font-mono-code text-mono-code text-fg-muted dark:text-dark-fg-muted">
							({data.worker.successes ?? 0}✓ / {data.worker.failures ?? 0}✗)
						</span>
					</div>
				</div>
			{/if}
			{#if data.worker.lastError}
				<div class="col-span-2">
					<div class="text-label-caps font-label-caps uppercase tracking-wide text-fg-muted dark:text-dark-fg-muted">Last error</div>
					<div class="mt-0.5 text-body-md text-status-failed">{data.worker.lastError}</div>
				</div>
			{/if}
		</div>

		<!-- Lifecycle audit -->
		<section class="shrink-0 border-b border-border-line px-6 py-4 dark:border-dark-border-line">
			<div class="mb-3 text-label-caps font-label-caps uppercase tracking-wide text-fg-muted dark:text-dark-fg-muted">
				Lifecycle audit
			</div>
			{#if lifecycleEvents.length === 0}
				<p class="text-body-sm text-fg-muted dark:text-dark-fg-muted">No lifecycle actions recorded.</p>
			{:else}
				<ul class="space-y-2">
					{#each lifecycleEvents as event (`${event.action}-${event.timestamp}`)}
						<li class="flex items-start justify-between gap-3 text-body-sm">
							<div>
								<span class="font-medium text-fg-ink dark:text-dark-fg-ink">{event.action}</span>
								<span class="text-fg-muted dark:text-dark-fg-muted"> by {event.actor}</span>
								{#if event.beadId}
									<span class="font-mono-code text-mono-code text-fg-muted dark:text-dark-fg-muted">
										· {event.beadId}
									</span>
								{/if}
								{#if event.detail}
									<div class="mt-0.5 text-fg-muted dark:text-dark-fg-muted">{event.detail}</div>
								{/if}
							</div>
							<time
								class="shrink-0 text-fg-muted dark:text-dark-fg-muted"
								datetime={event.timestamp}
							>
								{fmtDate(event.timestamp)}
							</time>
						</li>
					{/each}
				</ul>
			{/if}
		</section>

		<!-- Log output -->
		<div class="flex min-h-0 flex-1 flex-col">
			<div
				class="flex shrink-0 items-center justify-between border-b border-border-line px-4 py-2 dark:border-dark-border-line"
			>
				<span class="text-label-caps font-label-caps uppercase tracking-wide text-fg-muted dark:text-dark-fg-muted">Log output</span>
				<div class="flex items-center gap-3">
					<span class="text-mono-code text-fg-muted dark:text-dark-fg-muted">{logLines.length} lines</span>
					{#if !autoScroll}
						<button
							onclick={() => {
								autoScroll = true;
								if (logContainer) logContainer.scrollTop = logContainer.scrollHeight;
							}}
							class="px-2 py-0.5 text-xs text-accent-lever hover:bg-bg-surface dark:text-dark-accent-lever dark:hover:bg-dark-bg-surface"
						>
							↓ Follow
						</button>
					{/if}
				</div>
			</div>
			<!-- svelte-ignore a11y_no_noninteractive_element_interactions -->
			<pre
				bind:this={logContainer}
				onscroll={handleScroll}
				class="flex-1 overflow-auto bg-terminal-bg px-4 py-3 font-mono text-xs leading-relaxed text-terminal-fg"
			>{#if logLines.length === 0}<span class="text-fg-muted dark:text-dark-fg-muted">No log output yet…</span>{:else}{logLines.join('\n')}{/if}</pre>
		</div>
	</div>
{/if}
