<script lang="ts">
	import type { PageData } from './$types';
	import { goto } from '$app/navigation';
	import { page } from '$app/stores';
	import { subscribeWorkerProgress } from '$lib/gql/subscriptions';
	import { wsConnection, type WsState } from '$lib/stores/connection.svelte';
	import { createClient } from '$lib/gql/client';
	import { gql } from 'graphql-request';
	import type { WorkerRecentEvent } from './+page';

	let { data }: { data: PageData } = $props();

	let logLines = $state<string[]>([]);
	let logContainer = $state<HTMLPreElement | null>(null);
	let autoScroll = $state(true);
	let liveEvents = $state<WorkerRecentEvent[]>([]);
	let reconnecting = $state(false);
	let catchingUp = $state(false);
	let streamTerminal = $state(false);
	let streamCompletedAt = $state<string | null>(null);
	let previousWsState = $state<WsState>('idle');

	type LiveItem =
		| { type: 'text'; text: string }
		| { type: 'tool_call'; event: WorkerRecentEvent; key: string };

	const RECENT_EVENTS_QUERY = gql`
		query WorkerRecentEvents($id: ID!) {
			worker(id: $id) {
				id
				recentEvents {
					kind
					text
					name
					inputs
					output
				}
			}
		}
	`;

	// Initialize log lines from initial captured stdout
	$effect(() => {
		const raw = data.initialLog ?? '';
		logLines = raw.length > 0 ? raw.split('\n') : [];
	});

	$effect(() => {
		liveEvents = data.worker?.recentEvents ?? [];
		streamTerminal = false;
		streamCompletedAt = null;
	});

	// Auto-scroll to bottom when new lines arrive (if autoScroll is enabled)
	$effect(() => {
		// Depend on logLines length to trigger on each new line
		const _len = logLines.length;
		if (autoScroll && logContainer) {
			// Defer so DOM updates before we measure
			Promise.resolve().then(() => {
				if (logContainer) logContainer.scrollTop = logContainer.scrollHeight;
			});
		}
	});

	// Subscribe to live worker progress events
	$effect(() => {
		const workerId = data.worker?.id;
		if (!workerId || isTerminal) return;

		const dispose = subscribeWorkerProgress(workerId, (evt) => {
			if (terminalPhases.has(evt.phase)) {
				streamTerminal = true;
				streamCompletedAt = evt.timestamp;
			}
			if (isTerminal || terminalPhases.has(evt.phase)) return;
			if (evt.logLine != null && evt.logLine.length > 0) {
				logLines = [...logLines, evt.logLine];
				const frame = workerFrameFromProgressLine(evt.logLine);
				if (frame) appendLiveEvent(frame);
			}
		});

		return dispose;
	});

	$effect(() => {
		const state = wsConnection.state;
		reconnecting = wsConnection.showBanner || catchingUp;
		if (
			data.worker?.id &&
			previousWsState !== 'idle' &&
			previousWsState !== 'connected' &&
			state === 'connected'
		) {
			void catchUpRecentEvents(data.worker.id);
		}
		previousWsState = state;
	});

	function handleScroll() {
		if (!logContainer) return;
		const distFromBottom =
			logContainer.scrollHeight - logContainer.scrollTop - logContainer.clientHeight;
		autoScroll = distFromBottom < 20;
	}

	function handleClose() {
		const pathParts = $page.url.pathname.split('/');
		pathParts.pop(); // remove workerId segment
		const basePath = pathParts.join('/');
		goto(basePath);
	}

	function formatElapsed(ms: number): string {
		if (ms < 1000) return `${ms}ms`;
		if (ms < 60000) return `${(ms / 1000).toFixed(1)}s`;
		const m = Math.floor(ms / 60000);
		const s = Math.floor((ms % 60000) / 1000);
		return `${m}m${s}s`;
	}

	function inputText(input: unknown): string {
		if (input == null) return '';
		if (typeof input === 'string') return input;
		return JSON.stringify(input);
	}

	function toolLabel(event: { name: string | null; inputs: unknown }): string {
		const details = inputText(event.inputs);
		const summary = toolInputSummary(event.inputs);
		if (summary && details) return `${event.name ?? 'tool'} path ${summary} ${details}`;
		return details ? `${event.name ?? 'tool'} ${details}` : (event.name ?? 'tool');
	}

	function toolInputSummary(input: unknown): string {
		const parsed = typeof input === 'string' ? parseJSON(input) : input;
		if (!parsed || typeof parsed !== 'object' || Array.isArray(parsed)) return '';
		const value =
			(parsed as Record<string, unknown>).path ?? (parsed as Record<string, unknown>).file;
		if (typeof value !== 'string' || value.length === 0) return '';
		return value.split('/').pop() ?? value;
	}

	function parseJSON(value: string): unknown {
		try {
			return JSON.parse(value);
		} catch {
			return null;
		}
	}

	function evidenceBundleHref(workerId: string): string {
		return `/executions/${encodeURIComponent(workerId)}/result.json`;
	}

	function formatCompletedAt(value: string | null): string {
		if (!value) return 'terminal state';
		const date = new Date(value);
		if (Number.isNaN(date.getTime())) return value;
		return date.toLocaleTimeString([], {
			hour: '2-digit',
			minute: '2-digit',
			second: '2-digit'
		});
	}

	function fmtDate(value: string): string {
		const date = new Date(value);
		if (Number.isNaN(date.getTime())) return value;
		return date.toLocaleString();
	}

	function fmtDuration(ms: number): string {
		if (ms < 1000) return `${ms}ms`;
		if (ms < 60_000) return `${(ms / 1000).toFixed(1)}s`;
		const m = Math.floor(ms / 60_000);
		const s = Math.floor((ms % 60_000) / 1000);
		return `${m}m ${s}s`;
	}

	function fmtCost(cost: number | null): string {
		return cost == null ? '—' : `$${cost.toFixed(4)}`;
	}

	function sessionsHref(): string {
		return `/nodes/${data.nodeId}/projects/${data.projectId}/runs?layer=run`;
	}

	const workerSessions = $derived(data.sessions ?? []);
	const lifecycleEvents = $derived(data.worker?.lifecycleEvents ?? []);

	function appendLiveEvent(event: WorkerRecentEvent) {
		liveEvents = [...liveEvents, event];
	}

	function workerFrameFromProgressLine(line: string): WorkerRecentEvent | null {
		const trimmed = line.trim();
		if (!trimmed.startsWith('{')) return null;
		try {
			const raw = JSON.parse(trimmed) as Record<string, unknown>;
			const kind = String(raw.kind ?? raw.type ?? '');
			const payload =
				raw.data && typeof raw.data === 'object' ? (raw.data as Record<string, unknown>) : raw;
			if (kind === 'text_delta') {
				const text = raw.text ?? payload.text ?? payload.delta;
				return typeof text === 'string'
					? { kind: 'text_delta', text, name: null, inputs: null, output: null }
					: null;
			}
			if (kind === 'tool_call') {
				return {
					kind: 'tool_call',
					text: null,
					name: typeof payload.name === 'string' ? payload.name : null,
					inputs: inputText(payload.inputs ?? payload.input),
					output: typeof payload.output === 'string' ? payload.output : null
				};
			}
		} catch {
			return null;
		}
		return null;
	}

	async function catchUpRecentEvents(workerId: string) {
		catchingUp = true;
		try {
			const client = createClient(fetch);
			const result = await client.request<{
				worker: { recentEvents?: WorkerRecentEvent[] } | null;
			}>(RECENT_EVENTS_QUERY, { id: workerId });
			liveEvents = result.worker?.recentEvents ?? liveEvents;
		} catch (err) {
			console.error('[ddx] worker recentEvents catch-up failed:', err);
		} finally {
			catchingUp = false;
			reconnecting = wsConnection.showBanner;
		}
	}

	const terminalPhases = new Set(['done', 'exited', 'stopped', 'failed', 'error', 'preserved']);

	const isTerminal = $derived(
		data.worker?.state === 'done' ||
			data.worker?.state === 'exited' ||
			data.worker?.state === 'stopped' ||
			data.worker?.state === 'failed' ||
			data.worker?.state === 'error' ||
			streamTerminal ||
			Boolean(data.worker?.finishedAt)
	);

	const completedAt = $derived(data.worker?.finishedAt ?? streamCompletedAt);

	const liveItems = $derived.by(() => {
		const items: LiveItem[] = [];
		for (const event of liveEvents) {
			if (event.kind === 'text_delta' && event.text) {
				const last = items.at(-1);
				if (last?.type === 'text') {
					last.text += event.text;
				} else {
					items.push({ type: 'text', text: event.text });
				}
			} else if (event.kind === 'tool_call') {
				items.push({
					type: 'tool_call',
					event,
					key: `${items.length}-${event.name ?? 'tool'}-${inputText(event.inputs).slice(0, 40)}`
				});
			}
		}
		return items;
	});
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
				<p class="font-mono-code text-mono-code text-fg-muted dark:text-dark-fg-muted">{data.worker.id}</p>
			</div>
			<button
				onclick={handleClose}
				class="p-1.5 text-fg-muted hover:bg-bg-surface hover:text-fg-ink dark:text-dark-fg-muted dark:hover:bg-dark-bg-surface dark:hover:text-dark-fg-ink"
				aria-label="Close"
			>
				✕
			</button>
		</div>

		<!-- Metadata grid -->
		<div
			class="grid shrink-0 grid-cols-2 gap-x-6 gap-y-3 border-b border-border-line px-6 py-4 dark:border-dark-border-line"
		>
			<div>
				<div class="text-label-caps font-label-caps uppercase tracking-wide text-fg-muted dark:text-dark-fg-muted">State</div>
				<div class="mt-0.5 text-body-md font-medium text-fg-ink dark:text-dark-fg-ink">{data.worker.state}</div>
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
					<div class="mt-0.5 font-mono-code text-mono-code text-fg-ink dark:text-dark-fg-ink"
						>{data.worker.currentBead}</div
					>
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
			{#if data.worker.currentAttempt}
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
			{#if data.worker.lastError}
				<div class="col-span-2">
					<div class="text-label-caps font-label-caps uppercase tracking-wide text-fg-muted dark:text-dark-fg-muted">Last error</div>
					<div class="mt-0.5 text-body-md text-status-failed">{data.worker.lastError}</div>
				</div>
			{/if}
		</div>

		<section class="shrink-0 border-b border-border-line px-6 py-4 dark:border-dark-border-line">
			<div class="mb-3 flex items-center justify-between gap-3">
				<h3 class="text-label-caps font-label-caps uppercase tracking-wide text-fg-muted dark:text-dark-fg-muted">Sessions</h3>
				<a class="text-body-sm text-accent-lever hover:underline dark:text-dark-accent-lever" href={sessionsHref()}>
					All sessions
				</a>
			</div>
			{#if workerSessions.length === 0}
				<p class="text-body-sm text-fg-muted dark:text-dark-fg-muted">No sessions recorded yet.</p>
			{:else}
				<div class="overflow-hidden border border-border-line dark:border-dark-border-line">
					<table class="w-full text-body-sm">
						<thead class="bg-bg-surface text-fg-muted dark:bg-dark-bg-surface dark:text-dark-fg-muted">
							<tr>
								<th class="px-3 py-2 text-left text-label-caps font-label-caps uppercase tracking-wide">Session</th>
								<th class="px-3 py-2 text-left text-label-caps font-label-caps uppercase tracking-wide">Bead</th>
								<th class="px-3 py-2 text-left text-label-caps font-label-caps uppercase tracking-wide">Status</th>
								<th class="px-3 py-2 text-right text-label-caps font-label-caps uppercase tracking-wide">Cost</th>
							</tr>
						</thead>
						<tbody>
							{#each workerSessions as session (session.id)}
								<tr class="border-t border-border-line dark:border-dark-border-line">
									<td class="px-3 py-2">
										<div class="font-mono-code text-mono-code text-accent-lever dark:text-dark-accent-lever">
											{session.id.slice(0, 12)}
										</div>
										<div class="text-fg-muted dark:text-dark-fg-muted">
											{session.harness} · {fmtDuration(session.durationMs)}
										</div>
									</td>
									<td class="px-3 py-2 font-mono-code text-mono-code text-fg-muted dark:text-dark-fg-muted">
										{session.beadId ?? '—'}
									</td>
									<td class="px-3 py-2 text-fg-ink dark:text-dark-fg-ink">{session.status}</td>
									<td class="px-3 py-2 text-right font-mono-code text-mono-code text-fg-muted dark:text-dark-fg-muted">
										{fmtCost(session.cost)}
									</td>
								</tr>
							{/each}
						</tbody>
					</table>
				</div>
			{/if}
		</section>

		<section class="shrink-0 border-b border-border-line px-6 py-4 dark:border-dark-border-line">
			<div class="mb-3 text-label-caps font-label-caps uppercase tracking-wide text-fg-muted dark:text-dark-fg-muted">Lifecycle audit</div>
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
									<span class="font-mono-code text-mono-code text-fg-muted dark:text-dark-fg-muted"> · {event.beadId}</span>
								{/if}
								{#if event.detail}
									<div class="mt-0.5 text-fg-muted dark:text-dark-fg-muted">{event.detail}</div>
								{/if}
							</div>
							<time class="shrink-0 text-fg-muted dark:text-dark-fg-muted" datetime={event.timestamp}>
								{fmtDate(event.timestamp)}
							</time>
						</li>
					{/each}
				</ul>
			{/if}
		</section>

		<section
			role="region"
			aria-label="Live response"
			aria-live="polite"
			class="shrink-0 border-b border-border-line px-6 py-4 dark:border-dark-border-line"
		>
			<div class="mb-2 flex items-center justify-between gap-3">
				<div class="text-label-caps font-label-caps uppercase tracking-wide text-fg-muted dark:text-dark-fg-muted">Live response</div>
				{#if reconnecting && !isTerminal}
					<div class="alert-caution border px-2 py-1 text-body-sm">
						Reconnecting…
					</div>
				{/if}
			</div>
			<div aria-live="polite" class="space-y-2 text-fg-ink dark:text-dark-fg-ink">
				{#if liveItems.length === 0}
					<p class="text-body-sm text-fg-muted dark:text-dark-fg-muted">Waiting for response…</p>
				{:else}
					{#each liveItems as item (item.type === 'tool_call' ? item.key : item.text)}
						{#if item.type === 'text'}
							<p class="text-body-md whitespace-pre-wrap">{item.text}</p>
						{:else}
							<details class="border border-border-line dark:border-dark-border-line">
								<summary
									role="button"
									class="cursor-pointer px-3 py-2 font-mono-code text-mono-code text-fg-ink dark:text-dark-fg-ink"
								>
									{toolLabel(item.event)}
								</summary>
								<div class="border-t border-border-line dark:border-dark-border-line">
									<div
										class="px-3 pt-3 pb-1 text-label-caps font-label-caps uppercase tracking-wide text-fg-muted dark:text-dark-fg-muted"
									>
										Inputs
									</div>
									<pre class="overflow-x-auto px-3 pb-3 font-mono-code text-mono-code whitespace-pre-wrap">{inputText(
											item.event.inputs
										)}</pre>
									{#if item.event.output}
										<div
											class="border-t border-border-line px-3 pt-3 pb-1 text-label-caps font-label-caps uppercase tracking-wide text-fg-muted dark:border-dark-border-line dark:text-dark-fg-muted"
										>
											Output
										</div>
										<pre class="overflow-x-auto px-3 pb-3 font-mono-code text-mono-code whitespace-pre-wrap">{item.event
												.output}</pre>
									{/if}
								</div>
							</details>
						{/if}
					{/each}
				{/if}
				{#if isTerminal}
					<p class="text-body-sm text-fg-muted dark:text-dark-fg-muted">
						Completed at {formatCompletedAt(completedAt)}.
						<a
							class="text-accent-lever hover:underline dark:text-dark-accent-lever"
							href={evidenceBundleHref(data.worker.id)}
						>
							Evidence bundle
						</a>
					</p>
				{/if}
			</div>
		</section>

		<!-- Log area -->
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
				class="flex-1 overflow-auto bg-terminal-bg px-4 py-3 font-mono text-xs leading-relaxed text-terminal-fg">{#if logLines.length === 0}<span
						class="text-fg-muted dark:text-dark-fg-muted">No log output yet…</span
					>{:else}{logLines.join('\n')}{/if}</pre>
		</div>
	</div>
{/if}
