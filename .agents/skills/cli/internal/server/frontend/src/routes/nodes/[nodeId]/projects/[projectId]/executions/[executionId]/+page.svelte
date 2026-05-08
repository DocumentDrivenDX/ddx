<script lang="ts">
	import type { PageData } from './$types';
	import { createClient } from '$lib/gql/client';
	import { TOOL_CALLS_QUERY, SESSION_QUERY } from './+page';

	let { data }: { data: PageData } = $props();

	type Tab = 'manifest' | 'prompt' | 'result' | 'session' | 'tools';
	let activeTab = $state<Tab>('manifest');

	interface ToolCall {
		id: string;
		name: string;
		seq: number;
		ts: string | null;
		inputs: string | null;
		output: string | null;
		truncated: boolean | null;
	}
	interface ToolCallEdge {
		node: ToolCall;
		cursor: string;
	}
	interface ToolCallConnection {
		edges: ToolCallEdge[];
		pageInfo: { hasNextPage: boolean; endCursor: string | null };
		totalCount: number;
	}

	let toolCalls = $state<ToolCall[]>([]);
	let toolCallsLoaded = $state(false);
	let toolCallsLoading = $state(false);
	let toolCallsTotal = $state(0);
	let toolCallsCursor = $state<string | null>(null);
	let toolCallsHasMore = $state(false);
	let expanded = $state<Set<number>>(new Set());

	interface SessionDetail {
		id: string;
		harness: string;
		model: string;
		cost: number | null;
		billingMode: string;
		tokens: { prompt: number | null; completion: number | null; total: number | null; cached: number | null } | null;
		status: string;
		outcome: string | null;
	}
	let sessionDetail = $state<SessionDetail | null>(null);
	let sessionLoaded = $state(false);
	let sessionLoading = $state(false);

	async function loadToolCalls(more = false) {
		if (toolCallsLoading) return;
		toolCallsLoading = true;
		try {
			const client = createClient(fetch);
			const result = await client.request<{ executionToolCalls: ToolCallConnection }>(
				TOOL_CALLS_QUERY,
				{ id: data.executionId, first: 50, after: more ? toolCallsCursor : null }
			);
			const conn = result.executionToolCalls;
			const newCalls = conn.edges.map((e) => e.node);
			toolCalls = more ? [...toolCalls, ...newCalls] : newCalls;
			toolCallsTotal = conn.totalCount;
			toolCallsCursor = conn.pageInfo.endCursor;
			toolCallsHasMore = conn.pageInfo.hasNextPage;
			toolCallsLoaded = true;
		} finally {
			toolCallsLoading = false;
		}
	}

	async function loadSession() {
		if (!data.execution?.sessionId || sessionLoading || sessionLoaded) return;
		sessionLoading = true;
		try {
			const client = createClient(fetch);
			const result = await client.request<{ agentSession: SessionDetail | null }>(
				SESSION_QUERY,
				{ id: data.execution.sessionId }
			);
			sessionDetail = result.agentSession;
			sessionLoaded = true;
		} finally {
			sessionLoading = false;
		}
	}

	function pickTab(tab: Tab) {
		activeTab = tab;
		if (tab === 'tools' && !toolCallsLoaded) {
			void loadToolCalls(false);
		}
		if (tab === 'session' && !sessionLoaded) {
			void loadSession();
		}
	}

	function toggleCall(seq: number) {
		const next = new Set(expanded);
		if (next.has(seq)) next.delete(seq);
		else next.add(seq);
		expanded = next;
	}

	let manifestPretty = $state(true);
	let resultPretty = $state(true);

	function tryPretty(s: string | null | undefined): string {
		if (!s) return '';
		try {
			return JSON.stringify(JSON.parse(s), null, 2);
		} catch {
			return s;
		}
	}

	function fmtDuration(ms: number | null): string {
		if (ms == null) return '—';
		if (ms < 1000) return `${ms}ms`;
		if (ms < 60_000) return `${(ms / 1000).toFixed(1)}s`;
		const m = Math.floor(ms / 60_000);
		const s = Math.floor((ms % 60_000) / 1000);
		return `${m}m ${s}s`;
	}

	function fmtCost(c: number | null): string {
		if (c == null) return '—';
		return `$${c.toFixed(4)}`;
	}

	function beadHref(beadId: string): string {
		return `/nodes/${data.nodeId}/projects/${data.projectId}/beads/${beadId}`;
	}

	function sessionHref(sessionId: string): string {
		return `/nodes/${data.nodeId}/projects/${data.projectId}/sessions#${sessionId}`;
	}

	function listHref(): string {
		return `/nodes/${data.nodeId}/projects/${data.projectId}/executions`;
	}
</script>

{#if !data.execution}
	<div class="space-y-3">
		<a href={listHref()} class="text-sm text-blue-600 hover:underline dark:text-blue-400">← Executions</a>
		<div class="rounded-lg border border-amber-300 bg-amber-50 p-4 text-amber-900 dark:border-amber-700 dark:bg-amber-950 dark:text-amber-200">
			Execution <code class="font-mono">{data.executionId}</code> not found.
		</div>
	</div>
{:else}
	{@const exec = data.execution}
	<div class="space-y-4">
		<div class="flex flex-col gap-1">
			<a href={listHref()} class="text-sm text-blue-600 hover:underline dark:text-blue-400">← Executions</a>
			<h1 class="font-mono text-lg font-semibold dark:text-white">{exec.id}</h1>
			{#if exec.beadTitle}
				<div class="text-sm text-gray-700 dark:text-gray-300">{exec.beadTitle}</div>
			{/if}
		</div>

		<!-- Quick facts row -->
		<div class="grid grid-cols-2 gap-3 sm:grid-cols-3 lg:grid-cols-6">
			<div class="rounded-lg border border-gray-200 bg-gray-50 p-3 dark:border-gray-700 dark:bg-gray-800">
				<div class="text-xs text-gray-500 dark:text-gray-400">Verdict</div>
				<div class="mt-1 font-mono text-sm dark:text-white">{exec.verdict ?? '—'}</div>
			</div>
			<div class="rounded-lg border border-gray-200 bg-gray-50 p-3 dark:border-gray-700 dark:bg-gray-800">
				<div class="text-xs text-gray-500 dark:text-gray-400">Bead</div>
				<div class="mt-1 font-mono text-xs dark:text-white">
					{#if exec.beadId}
						<a class="text-blue-600 hover:underline dark:text-blue-400" href={beadHref(exec.beadId)}>{exec.beadId}</a>
					{:else}
						—
					{/if}
				</div>
			</div>
			<div class="rounded-lg border border-gray-200 bg-gray-50 p-3 dark:border-gray-700 dark:bg-gray-800">
				<div class="text-xs text-gray-500 dark:text-gray-400">Harness</div>
				<div class="mt-1 text-sm dark:text-white">{exec.harness ?? '—'}{exec.model ? ` / ${exec.model}` : ''}</div>
			</div>
			<div class="rounded-lg border border-gray-200 bg-gray-50 p-3 dark:border-gray-700 dark:bg-gray-800">
				<div class="text-xs text-gray-500 dark:text-gray-400">Duration</div>
				<div class="mt-1 text-sm dark:text-white">{fmtDuration(exec.durationMs)}</div>
			</div>
			<div class="rounded-lg border border-gray-200 bg-gray-50 p-3 dark:border-gray-700 dark:bg-gray-800">
				<div class="text-xs text-gray-500 dark:text-gray-400">Cost</div>
				<div class="mt-1 font-mono text-sm dark:text-white">{fmtCost(exec.costUsd)}</div>
			</div>
			<div class="rounded-lg border border-gray-200 bg-gray-50 p-3 dark:border-gray-700 dark:bg-gray-800">
				<div class="text-xs text-gray-500 dark:text-gray-400">Exit code</div>
				<div class="mt-1 font-mono text-sm dark:text-white">{exec.exitCode ?? '—'}</div>
			</div>
		</div>

		<!-- Tabs -->
		<div class="border-b border-gray-200 dark:border-gray-700">
			<nav class="flex gap-1">
				{#each [
					{ id: 'manifest', label: 'Manifest' },
					{ id: 'prompt', label: 'Prompt' },
					{ id: 'result', label: 'Result' },
					{ id: 'session', label: 'Session' },
					{ id: 'tools', label: 'Tool calls' }
				] as tab (tab.id)}
					<button
						type="button"
						data-tab={tab.id}
						onclick={() => pickTab(tab.id as Tab)}
						class="border-b-2 px-3 py-2 text-sm font-medium {activeTab === tab.id
							? 'border-blue-600 text-blue-700 dark:border-blue-400 dark:text-blue-300'
							: 'border-transparent text-gray-600 hover:text-gray-900 dark:text-gray-400 dark:hover:text-gray-200'}"
					>
						{tab.label}
					</button>
				{/each}
			</nav>
		</div>

		<div data-active-tab={activeTab}>
			{#if activeTab === 'manifest'}
				<div class="space-y-3">
					<div class="flex items-center justify-between">
						<div class="text-xs text-gray-500 dark:text-gray-400">
							{exec.manifestPath ?? `${exec.bundlePath}/manifest.json`}
						</div>
						<button
							type="button"
							class="rounded border border-gray-300 px-2 py-0.5 text-xs text-gray-700 hover:bg-gray-100 dark:border-gray-600 dark:text-gray-300 dark:hover:bg-gray-800"
							onclick={() => (manifestPretty = !manifestPretty)}
						>
							{manifestPretty ? 'Raw' : 'Pretty'}
						</button>
					</div>
					<pre data-testid="manifest-body" class="max-h-[28rem] overflow-auto rounded border border-gray-200 bg-white p-3 text-xs whitespace-pre-wrap text-gray-800 dark:border-gray-700 dark:bg-gray-950 dark:text-gray-200">{manifestPretty ? tryPretty(exec.manifest) : (exec.manifest ?? '')}</pre>
				</div>
			{:else if activeTab === 'prompt'}
				<div class="space-y-2">
					<div class="text-xs text-gray-500 dark:text-gray-400">
						{exec.promptPath ?? `${exec.bundlePath}/prompt.md`}
					</div>
					<pre data-testid="prompt-body" class="max-h-[40rem] overflow-auto rounded border border-gray-200 bg-white p-3 text-xs whitespace-pre-wrap text-gray-800 dark:border-gray-700 dark:bg-gray-950 dark:text-gray-200">{exec.prompt ?? '(no prompt body)'}</pre>
				</div>
			{:else if activeTab === 'result'}
				<div class="space-y-3">
					{#if exec.rationale}
						<div class="rounded-lg border border-gray-200 bg-white p-3 text-sm whitespace-pre-wrap dark:border-gray-700 dark:bg-gray-900 dark:text-gray-200">
							{exec.rationale}
						</div>
					{/if}
					<div class="flex items-center justify-between">
						<div class="text-xs text-gray-500 dark:text-gray-400">
							{exec.resultPath ?? `${exec.bundlePath}/result.json`}
						</div>
						<button
							type="button"
							class="rounded border border-gray-300 px-2 py-0.5 text-xs text-gray-700 hover:bg-gray-100 dark:border-gray-600 dark:text-gray-300 dark:hover:bg-gray-800"
							onclick={() => (resultPretty = !resultPretty)}
						>
							{resultPretty ? 'Raw' : 'Pretty'}
						</button>
					</div>
					<pre data-testid="result-body" class="max-h-[28rem] overflow-auto rounded border border-gray-200 bg-white p-3 text-xs whitespace-pre-wrap text-gray-800 dark:border-gray-700 dark:bg-gray-950 dark:text-gray-200">{resultPretty ? tryPretty(exec.result) : (exec.result ?? '')}</pre>
				</div>
			{:else if activeTab === 'session'}
				<div class="space-y-3">
					{#if !exec.sessionId}
						<div class="rounded border border-gray-200 bg-gray-50 p-3 text-sm text-gray-600 dark:border-gray-700 dark:bg-gray-800 dark:text-gray-300">
							No session id was recorded for this execution.
						</div>
					{:else if sessionLoading}
						<div class="text-sm text-gray-500 dark:text-gray-400">Loading session…</div>
					{:else if !sessionDetail}
						<div class="rounded border border-amber-200 bg-amber-50 p-3 text-sm text-amber-800 dark:border-amber-700 dark:bg-amber-950 dark:text-amber-200">
							Session <code class="font-mono">{exec.sessionId}</code> referenced by this execution
							is not (yet) recorded in the session index. Cost and token totals are not available.
						</div>
					{:else}
						<dl class="grid grid-cols-2 gap-3 text-sm sm:grid-cols-4">
							<div><dt class="text-xs text-gray-500 dark:text-gray-400">Session</dt><dd class="mt-1 font-mono text-xs"><a class="text-blue-600 hover:underline dark:text-blue-400" href={sessionHref(sessionDetail.id)}>{sessionDetail.id}</a></dd></div>
							<div><dt class="text-xs text-gray-500 dark:text-gray-400">Harness</dt><dd class="mt-1">{sessionDetail.harness}</dd></div>
							<div><dt class="text-xs text-gray-500 dark:text-gray-400">Model</dt><dd class="mt-1">{sessionDetail.model}</dd></div>
							<div><dt class="text-xs text-gray-500 dark:text-gray-400">Status</dt><dd class="mt-1">{sessionDetail.status}</dd></div>
							<div><dt class="text-xs text-gray-500 dark:text-gray-400">Cost</dt><dd class="mt-1 font-mono">{fmtCost(sessionDetail.cost)}</dd></div>
							<div><dt class="text-xs text-gray-500 dark:text-gray-400">Billing</dt><dd class="mt-1">{sessionDetail.billingMode}</dd></div>
							<div><dt class="text-xs text-gray-500 dark:text-gray-400">Prompt tokens</dt><dd class="mt-1 font-mono">{sessionDetail.tokens?.prompt?.toLocaleString() ?? '—'}</dd></div>
							<div><dt class="text-xs text-gray-500 dark:text-gray-400">Completion tokens</dt><dd class="mt-1 font-mono">{sessionDetail.tokens?.completion?.toLocaleString() ?? '—'}</dd></div>
						</dl>
					{/if}
				</div>
			{:else if activeTab === 'tools'}
				<div class="space-y-2">
					<div class="flex items-center justify-between text-xs text-gray-500 dark:text-gray-400">
						<span>
							{toolCallsLoaded ? `${toolCalls.length} of ${toolCallsTotal} tool calls` : 'Loading…'}
						</span>
						{#if exec.agentLogPath}
							<span>Source: <code class="font-mono">{exec.agentLogPath}</code></span>
						{/if}
					</div>
					{#if toolCallsLoaded && toolCalls.length === 0}
						<div class="rounded border border-gray-200 bg-gray-50 p-3 text-sm text-gray-600 dark:border-gray-700 dark:bg-gray-800 dark:text-gray-300">
							No tool calls were captured for this execution.
						</div>
					{/if}
					<ul class="space-y-1">
						{#each toolCalls as call (call.seq)}
							{@const open = expanded.has(call.seq)}
							<li class="rounded border border-gray-200 dark:border-gray-700">
								<button
									type="button"
									data-tool-seq={call.seq}
									class="flex w-full items-center justify-between px-3 py-2 text-left text-sm hover:bg-gray-50 dark:hover:bg-gray-800"
									onclick={() => toggleCall(call.seq)}
								>
									<span class="flex items-center gap-2">
										<span class="font-mono text-xs text-gray-400 dark:text-gray-500">#{call.seq}</span>
										<span class="font-medium dark:text-white">{call.name}</span>
									</span>
									<span class="text-xs text-gray-500 dark:text-gray-400">{open ? '▾' : '▸'}</span>
								</button>
								{#if open}
									<div class="space-y-2 border-t border-gray-200 px-3 py-2 dark:border-gray-700">
										{#if call.inputs}
											<div>
												<div class="text-xs font-medium text-gray-500 dark:text-gray-400">Inputs</div>
												<pre class="mt-1 max-h-56 overflow-auto rounded border border-gray-200 bg-white p-2 text-xs whitespace-pre-wrap dark:border-gray-700 dark:bg-gray-950 dark:text-gray-200">{tryPretty(call.inputs)}</pre>
											</div>
										{/if}
										{#if call.output}
											<div>
												<div class="text-xs font-medium text-gray-500 dark:text-gray-400">Output{call.truncated ? ' (truncated)' : ''}</div>
												<pre class="mt-1 max-h-56 overflow-auto rounded border border-gray-200 bg-white p-2 text-xs whitespace-pre-wrap dark:border-gray-700 dark:bg-gray-950 dark:text-gray-200">{call.output}</pre>
											</div>
										{/if}
									</div>
								{/if}
							</li>
						{/each}
					</ul>
					{#if toolCallsHasMore}
						<div class="pt-2">
							<button
								type="button"
								onclick={() => loadToolCalls(true)}
								disabled={toolCallsLoading}
								class="rounded border border-gray-300 px-3 py-1.5 text-sm text-gray-700 hover:bg-gray-100 disabled:opacity-50 dark:border-gray-600 dark:text-gray-300 dark:hover:bg-gray-700"
							>
								{toolCallsLoading ? 'Loading…' : 'Load more'}
							</button>
						</div>
					{/if}
				</div>
			{/if}
		</div>
	</div>
{/if}
