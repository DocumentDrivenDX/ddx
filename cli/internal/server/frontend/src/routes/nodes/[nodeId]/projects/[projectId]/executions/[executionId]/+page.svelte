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
		<a href={listHref()} class="text-body-sm text-accent-lever hover:underline dark:text-dark-accent-lever">← Executions</a>
		<div class="alert-caution border p-4">
			Execution <code class="font-mono-code text-mono-code">{data.executionId}</code> not found.
		</div>
	</div>
{:else}
	{@const exec = data.execution}
	<div class="space-y-4">
		<div class="flex flex-col gap-1">
			<a href={listHref()} class="text-body-sm text-accent-lever hover:underline dark:text-dark-accent-lever">← Executions</a>
			<h1 class="font-mono-code text-headline-md font-headline-md text-fg-ink dark:text-dark-fg-ink">{exec.id}</h1>
			{#if exec.beadTitle}
				<div class="text-body-sm text-fg-muted dark:text-dark-fg-muted">{exec.beadTitle}</div>
			{/if}
		</div>

		<!-- Quick facts row -->
		<div class="grid grid-cols-2 gap-3 sm:grid-cols-3 lg:grid-cols-6">
			<div class="border border-border-line bg-bg-surface p-3 dark:border-dark-border-line dark:bg-dark-bg-surface">
				<div class="text-label-caps font-label-caps uppercase tracking-wide text-fg-muted dark:text-dark-fg-muted">Verdict</div>
				<div class="mt-1 font-mono-code text-mono-code text-fg-ink dark:text-dark-fg-ink">{exec.verdict ?? '—'}</div>
			</div>
			<div class="border border-border-line bg-bg-surface p-3 dark:border-dark-border-line dark:bg-dark-bg-surface">
				<div class="text-label-caps font-label-caps uppercase tracking-wide text-fg-muted dark:text-dark-fg-muted">Bead</div>
				<div class="mt-1 font-mono-code text-mono-code text-fg-ink dark:text-dark-fg-ink">
					{#if exec.beadId}
						<a class="text-accent-lever hover:underline dark:text-dark-accent-lever" href={beadHref(exec.beadId)}>{exec.beadId}</a>
					{:else}
						—
					{/if}
				</div>
			</div>
			<div class="border border-border-line bg-bg-surface p-3 dark:border-dark-border-line dark:bg-dark-bg-surface">
				<div class="text-label-caps font-label-caps uppercase tracking-wide text-fg-muted dark:text-dark-fg-muted">Harness</div>
				<div class="mt-1 text-body-sm text-fg-ink dark:text-dark-fg-ink">{exec.harness ?? '—'}{exec.model ? ` / ${exec.model}` : ''}</div>
			</div>
			<div class="border border-border-line bg-bg-surface p-3 dark:border-dark-border-line dark:bg-dark-bg-surface">
				<div class="text-label-caps font-label-caps uppercase tracking-wide text-fg-muted dark:text-dark-fg-muted">Duration</div>
				<div class="mt-1 font-mono-code text-mono-code text-fg-ink dark:text-dark-fg-ink">{fmtDuration(exec.durationMs)}</div>
			</div>
			<div class="border border-border-line bg-bg-surface p-3 dark:border-dark-border-line dark:bg-dark-bg-surface">
				<div class="text-label-caps font-label-caps uppercase tracking-wide text-fg-muted dark:text-dark-fg-muted">Cost</div>
				<div class="mt-1 font-mono-code text-mono-code text-fg-ink dark:text-dark-fg-ink">{fmtCost(exec.costUsd)}</div>
			</div>
			<div class="border border-border-line bg-bg-surface p-3 dark:border-dark-border-line dark:bg-dark-bg-surface">
				<div class="text-label-caps font-label-caps uppercase tracking-wide text-fg-muted dark:text-dark-fg-muted">Exit code</div>
				<div class="mt-1 font-mono-code text-mono-code text-fg-ink dark:text-dark-fg-ink">{exec.exitCode ?? '—'}</div>
			</div>
		</div>

		<!-- Tabs -->
		<div class="border-b border-border-line dark:border-dark-border-line">
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
						class="border-b-2 px-3 py-2 text-body-sm font-medium {activeTab === tab.id
							? 'border-accent-lever text-accent-lever dark:border-dark-accent-lever dark:text-dark-accent-lever'
							: 'border-transparent text-fg-muted hover:text-fg-ink dark:text-dark-fg-muted dark:hover:text-dark-fg-ink'}"
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
						<div class="font-mono-code text-mono-code text-fg-muted dark:text-dark-fg-muted">
							{exec.manifestPath ?? `${exec.bundlePath}/manifest.json`}
						</div>
						<button
							type="button"
							class="border border-border-line px-2 py-0.5 text-label-caps font-label-caps uppercase tracking-wide text-fg-muted hover:bg-bg-surface dark:border-dark-border-line dark:text-dark-fg-muted dark:hover:bg-dark-bg-surface"
							onclick={() => (manifestPretty = !manifestPretty)}
						>
							{manifestPretty ? 'Raw' : 'Pretty'}
						</button>
					</div>
					<pre data-testid="manifest-body" class="max-h-[28rem] overflow-auto bg-terminal-bg px-4 py-3 font-mono-code text-mono-code leading-relaxed text-terminal-fg whitespace-pre-wrap">{manifestPretty ? tryPretty(exec.manifest) : (exec.manifest ?? '')}</pre>
				</div>
			{:else if activeTab === 'prompt'}
				<div class="space-y-2">
					<div class="font-mono-code text-mono-code text-fg-muted dark:text-dark-fg-muted">
						{exec.promptPath ?? `${exec.bundlePath}/prompt.md`}
					</div>
					<pre data-testid="prompt-body" class="max-h-[40rem] overflow-auto bg-terminal-bg px-4 py-3 font-mono-code text-mono-code leading-relaxed text-terminal-fg whitespace-pre-wrap">{exec.prompt ?? '(no prompt body)'}</pre>
				</div>
			{:else if activeTab === 'result'}
				<div class="space-y-3">
					{#if exec.rationale}
						<div class="border border-border-line bg-bg-surface p-3 text-body-sm text-fg-ink whitespace-pre-wrap dark:border-dark-border-line dark:bg-dark-bg-surface dark:text-dark-fg-ink">
							{exec.rationale}
						</div>
					{/if}
					<div class="flex items-center justify-between">
						<div class="font-mono-code text-mono-code text-fg-muted dark:text-dark-fg-muted">
							{exec.resultPath ?? `${exec.bundlePath}/result.json`}
						</div>
						<button
							type="button"
							class="border border-border-line px-2 py-0.5 text-label-caps font-label-caps uppercase tracking-wide text-fg-muted hover:bg-bg-surface dark:border-dark-border-line dark:text-dark-fg-muted dark:hover:bg-dark-bg-surface"
							onclick={() => (resultPretty = !resultPretty)}
						>
							{resultPretty ? 'Raw' : 'Pretty'}
						</button>
					</div>
					<pre data-testid="result-body" class="max-h-[28rem] overflow-auto bg-terminal-bg px-4 py-3 font-mono-code text-mono-code leading-relaxed text-terminal-fg whitespace-pre-wrap">{resultPretty ? tryPretty(exec.result) : (exec.result ?? '')}</pre>
				</div>
			{:else if activeTab === 'session'}
				<div class="space-y-3">
					{#if !exec.sessionId}
						<div class="border border-border-line bg-bg-surface p-3 text-body-sm text-fg-muted dark:border-dark-border-line dark:bg-dark-bg-surface dark:text-dark-fg-muted">
							No session id was recorded for this execution.
						</div>
					{:else if sessionLoading}
						<div class="text-body-sm text-fg-muted dark:text-dark-fg-muted">Loading session…</div>
					{:else if !sessionDetail}
						<div class="alert-caution border p-3 text-body-sm">
							Session <code class="font-mono-code text-mono-code">{exec.sessionId}</code> referenced by this execution
							is not (yet) recorded in the session index. Cost and token totals are not available.
						</div>
					{:else}
						<dl class="grid grid-cols-2 gap-3 sm:grid-cols-4">
							<div><dt class="text-label-caps font-label-caps uppercase tracking-wide text-fg-muted dark:text-dark-fg-muted">Session</dt><dd class="mt-1 font-mono-code text-mono-code"><a class="text-accent-lever hover:underline dark:text-dark-accent-lever" href={sessionHref(sessionDetail.id)}>{sessionDetail.id}</a></dd></div>
							<div><dt class="text-label-caps font-label-caps uppercase tracking-wide text-fg-muted dark:text-dark-fg-muted">Harness</dt><dd class="mt-1 text-body-sm text-fg-ink dark:text-dark-fg-ink">{sessionDetail.harness}</dd></div>
							<div><dt class="text-label-caps font-label-caps uppercase tracking-wide text-fg-muted dark:text-dark-fg-muted">Model</dt><dd class="mt-1 text-body-sm text-fg-ink dark:text-dark-fg-ink">{sessionDetail.model}</dd></div>
							<div><dt class="text-label-caps font-label-caps uppercase tracking-wide text-fg-muted dark:text-dark-fg-muted">Status</dt><dd class="mt-1 text-body-sm text-fg-ink dark:text-dark-fg-ink">{sessionDetail.status}</dd></div>
							<div><dt class="text-label-caps font-label-caps uppercase tracking-wide text-fg-muted dark:text-dark-fg-muted">Cost</dt><dd class="mt-1 font-mono-code text-mono-code text-fg-ink dark:text-dark-fg-ink">{fmtCost(sessionDetail.cost)}</dd></div>
							<div><dt class="text-label-caps font-label-caps uppercase tracking-wide text-fg-muted dark:text-dark-fg-muted">Billing</dt><dd class="mt-1 text-body-sm text-fg-ink dark:text-dark-fg-ink">{sessionDetail.billingMode}</dd></div>
							<div><dt class="text-label-caps font-label-caps uppercase tracking-wide text-fg-muted dark:text-dark-fg-muted">Prompt tokens</dt><dd class="mt-1 font-mono-code text-mono-code text-fg-ink dark:text-dark-fg-ink">{sessionDetail.tokens?.prompt?.toLocaleString() ?? '—'}</dd></div>
							<div><dt class="text-label-caps font-label-caps uppercase tracking-wide text-fg-muted dark:text-dark-fg-muted">Completion tokens</dt><dd class="mt-1 font-mono-code text-mono-code text-fg-ink dark:text-dark-fg-ink">{sessionDetail.tokens?.completion?.toLocaleString() ?? '—'}</dd></div>
						</dl>
					{/if}
				</div>
			{:else if activeTab === 'tools'}
				<div class="space-y-2">
					<div class="flex items-center justify-between text-label-caps font-label-caps uppercase tracking-wide text-fg-muted dark:text-dark-fg-muted">
						<span>
							{toolCallsLoaded ? `${toolCalls.length} of ${toolCallsTotal} tool calls` : 'Loading…'}
						</span>
						{#if exec.agentLogPath}
							<span>Source: <code class="font-mono-code text-mono-code">{exec.agentLogPath}</code></span>
						{/if}
					</div>
					{#if toolCallsLoaded && toolCalls.length === 0}
						<div class="border border-border-line bg-bg-surface p-3 text-body-sm text-fg-muted dark:border-dark-border-line dark:bg-dark-bg-surface dark:text-dark-fg-muted">
							No tool calls were captured for this execution.
						</div>
					{/if}
					<ul class="space-y-1">
						{#each toolCalls as call (call.seq)}
							{@const open = expanded.has(call.seq)}
							<li class="border border-border-line dark:border-dark-border-line">
								<button
									type="button"
									data-tool-seq={call.seq}
									class="flex w-full items-center justify-between px-3 py-2 text-left hover:bg-bg-surface dark:hover:bg-dark-bg-surface"
									onclick={() => toggleCall(call.seq)}
								>
									<span class="flex items-center gap-2">
										<span class="font-mono-code text-mono-code text-fg-muted dark:text-dark-fg-muted">#{call.seq}</span>
										<span class="text-body-sm font-medium text-fg-ink dark:text-dark-fg-ink">{call.name}</span>
									</span>
									<span class="text-label-caps font-label-caps text-fg-muted dark:text-dark-fg-muted">{open ? '▾' : '▸'}</span>
								</button>
								{#if open}
									<div class="space-y-2 border-t border-border-line px-3 py-2 dark:border-dark-border-line">
										{#if call.inputs}
											<div>
												<div class="text-label-caps font-label-caps uppercase tracking-wide text-fg-muted dark:text-dark-fg-muted">Inputs</div>
												<pre class="mt-1 max-h-56 overflow-auto bg-terminal-bg px-3 py-2 font-mono-code text-mono-code text-terminal-fg whitespace-pre-wrap">{tryPretty(call.inputs)}</pre>
											</div>
										{/if}
										{#if call.output}
											<div>
												<div class="text-label-caps font-label-caps uppercase tracking-wide text-fg-muted dark:text-dark-fg-muted">Output{call.truncated ? ' (truncated)' : ''}</div>
												<pre class="mt-1 max-h-56 overflow-auto bg-terminal-bg px-3 py-2 font-mono-code text-mono-code text-terminal-fg whitespace-pre-wrap">{call.output}</pre>
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
								class="border border-border-line px-3 py-1.5 text-body-sm text-fg-muted hover:bg-bg-surface disabled:opacity-50 dark:border-dark-border-line dark:text-dark-fg-muted dark:hover:bg-dark-bg-surface"
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
