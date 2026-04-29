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
		return `/nodes/${data.nodeId}/projects/${data.projectId}/sessions`;
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
		class="fixed top-0 right-0 z-50 flex h-full w-full max-w-2xl flex-col bg-white shadow-xl dark:bg-gray-900"
	>
		<!-- Header -->
		<div
			class="flex shrink-0 items-center justify-between border-b border-gray-200 px-6 py-4 dark:border-gray-700"
		>
			<div>
				<h2 class="text-base font-semibold text-gray-900 dark:text-white">
					{data.worker.kind}
				</h2>
				<p class="font-mono text-xs text-gray-500 dark:text-gray-400">{data.worker.id}</p>
			</div>
			<button
				onclick={handleClose}
				class="rounded p-1.5 text-gray-400 hover:bg-gray-100 hover:text-gray-600 dark:hover:bg-gray-800 dark:hover:text-gray-300"
				aria-label="Close"
			>
				✕
			</button>
		</div>

		<!-- Metadata grid -->
		<div
			class="grid shrink-0 grid-cols-2 gap-x-6 gap-y-2 border-b border-gray-200 px-6 py-4 text-sm dark:border-gray-700"
		>
			<div>
				<span class="text-gray-500 dark:text-gray-400">State: </span>
				<span class="font-medium text-gray-900 dark:text-white">{data.worker.state}</span>
			</div>
			{#if data.worker.harness}
				<div>
					<span class="text-gray-500 dark:text-gray-400">Harness: </span>
					<span class="text-gray-900 dark:text-white">{data.worker.harness}</span>
				</div>
			{/if}
			{#if data.worker.model}
				<div>
					<span class="text-gray-500 dark:text-gray-400">Model: </span>
					<span class="text-gray-900 dark:text-white">{data.worker.model}</span>
				</div>
			{/if}
			{#if data.worker.effort}
				<div>
					<span class="text-gray-500 dark:text-gray-400">Effort: </span>
					<span class="text-gray-900 dark:text-white">{data.worker.effort}</span>
				</div>
			{/if}
			{#if data.worker.currentBead}
				<div class="col-span-2">
					<span class="text-gray-500 dark:text-gray-400">Current bead: </span>
					<span class="font-mono text-xs text-gray-900 dark:text-white"
						>{data.worker.currentBead}</span
					>
				</div>
			{/if}
			{#if data.worker.attempts != null}
				<div>
					<span class="text-gray-500 dark:text-gray-400">Attempts: </span>
					<span class="text-gray-900 dark:text-white">
						{data.worker.attempts}
						<span class="text-xs text-gray-500 dark:text-gray-400">
							({data.worker.successes ?? 0}✓ / {data.worker.failures ?? 0}✗)
						</span>
					</span>
				</div>
			{/if}
			{#if data.worker.currentAttempt}
				<div>
					<span class="text-gray-500 dark:text-gray-400">Phase: </span>
					<span class="font-medium text-yellow-600 dark:text-yellow-400">
						{data.worker.currentAttempt.phase}
					</span>
					<span class="ml-1 text-xs text-gray-600 dark:text-gray-400">
						({formatElapsed(data.worker.currentAttempt.elapsedMs)})
					</span>
				</div>
			{/if}
			{#if data.worker.lastError}
				<div class="col-span-2">
					<span class="text-gray-500 dark:text-gray-400">Last error: </span>
					<span class="text-red-600 dark:text-red-400">{data.worker.lastError}</span>
				</div>
			{/if}
		</div>

		<section class="shrink-0 border-b border-gray-200 px-6 py-4 text-sm dark:border-gray-700">
			<div class="mb-3 flex items-center justify-between gap-3">
				<h3 class="text-xs font-medium text-gray-500 dark:text-gray-400">Sessions</h3>
				<a class="text-xs text-blue-600 hover:underline dark:text-blue-400" href={sessionsHref()}>
					All sessions
				</a>
			</div>
			{#if workerSessions.length === 0}
				<p class="text-xs text-gray-500 dark:text-gray-400">No sessions recorded yet.</p>
			{:else}
				<div class="overflow-hidden rounded border border-gray-200 dark:border-gray-700">
					<table class="w-full text-xs">
						<thead class="bg-gray-50 text-gray-500 dark:bg-gray-800 dark:text-gray-400">
							<tr>
								<th class="px-3 py-2 text-left font-medium">Session</th>
								<th class="px-3 py-2 text-left font-medium">Bead</th>
								<th class="px-3 py-2 text-left font-medium">Status</th>
								<th class="px-3 py-2 text-right font-medium">Cost</th>
							</tr>
						</thead>
						<tbody>
							{#each workerSessions as session (session.id)}
								<tr class="border-t border-gray-100 dark:border-gray-700">
									<td class="px-3 py-2">
										<div class="font-mono text-gray-700 dark:text-gray-200">
											{session.id.slice(0, 12)}
										</div>
										<div class="text-gray-400 dark:text-gray-500">
											{session.harness} · {fmtDuration(session.durationMs)}
										</div>
									</td>
									<td class="px-3 py-2 font-mono text-gray-600 dark:text-gray-300">
										{session.beadId ?? '—'}
									</td>
									<td class="px-3 py-2 text-gray-700 dark:text-gray-200">{session.status}</td>
									<td class="px-3 py-2 text-right font-mono text-gray-600 dark:text-gray-300">
										{fmtCost(session.cost)}
									</td>
								</tr>
							{/each}
						</tbody>
					</table>
				</div>
			{/if}
		</section>

		<section class="shrink-0 border-b border-gray-200 px-6 py-4 text-sm dark:border-gray-700">
			<div class="mb-3 text-xs font-medium text-gray-500 dark:text-gray-400">Lifecycle audit</div>
			{#if lifecycleEvents.length === 0}
				<p class="text-xs text-gray-500 dark:text-gray-400">No lifecycle actions recorded.</p>
			{:else}
				<ul class="space-y-2">
					{#each lifecycleEvents as event (`${event.action}-${event.timestamp}`)}
						<li class="flex items-start justify-between gap-3 text-xs">
							<div>
								<span class="font-medium text-gray-800 dark:text-gray-100">{event.action}</span>
								<span class="text-gray-500 dark:text-gray-400"> by {event.actor}</span>
								{#if event.beadId}
									<span class="font-mono text-gray-500 dark:text-gray-400"> · {event.beadId}</span>
								{/if}
								{#if event.detail}
									<div class="mt-0.5 text-gray-500 dark:text-gray-400">{event.detail}</div>
								{/if}
							</div>
							<time class="shrink-0 text-gray-400 dark:text-gray-500" datetime={event.timestamp}>
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
			class="shrink-0 border-b border-gray-200 px-6 py-4 text-sm dark:border-gray-700"
		>
			<div class="mb-2 flex items-center justify-between gap-3">
				<div class="text-xs font-medium text-gray-500 dark:text-gray-400">Live response</div>
				{#if reconnecting && !isTerminal}
					<div
						class="rounded border border-amber-300 bg-amber-50 px-2 py-1 text-xs text-amber-800 dark:border-amber-800 dark:bg-amber-950/30 dark:text-amber-200"
					>
						Reconnecting…
					</div>
				{/if}
			</div>
			<div aria-live="polite" class="space-y-2 text-gray-800 dark:text-gray-200">
				{#if liveItems.length === 0}
					<p class="text-xs text-gray-500 dark:text-gray-400">Waiting for response…</p>
				{:else}
					{#each liveItems as item (item.type === 'tool_call' ? item.key : item.text)}
						{#if item.type === 'text'}
							<p class="whitespace-pre-wrap">{item.text}</p>
						{:else}
							<details class="rounded border border-gray-200 dark:border-gray-700">
								<summary
									role="button"
									class="cursor-pointer px-3 py-2 font-mono text-xs text-gray-700 dark:text-gray-200"
								>
									{toolLabel(item.event)}
								</summary>
								<div class="border-t border-gray-200 dark:border-gray-700">
									<div
										class="px-3 pt-3 pb-1 text-[11px] font-medium text-gray-500 uppercase dark:text-gray-400"
									>
										Inputs
									</div>
									<pre class="overflow-x-auto px-3 pb-3 text-xs whitespace-pre-wrap">{inputText(
											item.event.inputs
										)}</pre>
									{#if item.event.output}
										<div
											class="border-t border-gray-200 px-3 pt-3 pb-1 text-[11px] font-medium text-gray-500 uppercase dark:border-gray-700 dark:text-gray-400"
										>
											Output
										</div>
										<pre class="overflow-x-auto px-3 pb-3 text-xs whitespace-pre-wrap">{item.event
												.output}</pre>
									{/if}
								</div>
							</details>
						{/if}
					{/each}
				{/if}
				{#if isTerminal}
					<p class="text-xs text-gray-600 dark:text-gray-400">
						Completed at {formatCompletedAt(completedAt)}.
						<a
							class="text-blue-600 hover:underline dark:text-blue-400"
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
				class="flex shrink-0 items-center justify-between border-b border-gray-200 px-4 py-2 dark:border-gray-700"
			>
				<span class="text-xs font-medium text-gray-500 dark:text-gray-400">Log output</span>
				<div class="flex items-center gap-3">
					<span class="text-xs text-gray-400 dark:text-gray-500">{logLines.length} lines</span>
					{#if !autoScroll}
						<button
							onclick={() => {
								autoScroll = true;
								if (logContainer) logContainer.scrollTop = logContainer.scrollHeight;
							}}
							class="rounded px-2 py-0.5 text-xs text-blue-600 hover:bg-blue-50 dark:text-blue-400 dark:hover:bg-blue-900/20"
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
				class="flex-1 overflow-auto bg-gray-950 px-4 py-3 font-mono text-xs leading-relaxed text-green-400 dark:text-green-300">{#if logLines.length === 0}<span
						class="text-gray-600 dark:text-gray-500">No log output yet…</span
					>{:else}{logLines.join('\n')}{/if}</pre>
		</div>
	</div>
{/if}
