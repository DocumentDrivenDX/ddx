<script lang="ts">
	import { createClient } from '$lib/gql/client';
	import Overview from './Overview.svelte';
	import Prompt from './Prompt.svelte';
	import Response from './Response.svelte';
	import Tools from './Tools.svelte';
	import Session from './Session.svelte';
	import Evidence from './Evidence.svelte';
	import {
		RUN_DETAIL_QUERY,
		RUN_EXECUTION_QUERY,
		RUN_SESSION_QUERY,
		RUN_TOOL_CALLS_QUERY
	} from './queries';
	import type { RunDetail, ToolCall, SessionDetail } from './types';

	type Tab = 'overview' | 'prompt' | 'response' | 'tools' | 'session' | 'evidence';

	interface Props {
		runId: string;
		layer: string;
		nodeId: string;
		projectId: string;
		initialTab?: Tab;
		onTabChange?: (tab: Tab) => void;
	}
	let { runId, layer, nodeId, projectId, initialTab = 'overview', onTabChange }: Props = $props();

	let run = $state<RunDetail | null>(null);
	let runLoading = $state(false);

	let exec = $state<{
		id: string;
		sessionId: string | null;
		bundlePath: string;
		promptPath: string | null;
		manifestPath: string | null;
		resultPath: string | null;
		agentLogPath: string | null;
		prompt: string | null;
		manifest: string | null;
		result: string | null;
		rationale: string | null;
	} | null>(null);
	let execLoading = $state(false);
	let execLoaded = $state(false);

	let session = $state<SessionDetail | null>(null);
	let sessionLoading = $state(false);
	let sessionLoaded = $state(false);

	let toolCalls = $state<ToolCall[]>([]);
	let toolCallsTotal = $state<number | undefined>(undefined);
	let toolCallsCursor = $state<string | null>(null);
	let toolCallsHasMore = $state(false);
	let toolCallsLoading = $state(false);
	let toolCallsLoaded = $state(false);

	let activeTab = $state<Tab>(initialTab);

	$effect(() => {
		if (initialTab !== activeTab) {
			activeTab = initialTab;
			if (activeTab === 'tools' && !toolCallsLoaded) {
				void fetchToolCalls(false);
			}
		}
	});

	function beadHref(beadId: string): string {
		return `/nodes/${nodeId}/projects/${projectId}/beads/${beadId}`;
	}

	async function fetchRun() {
		if (run || runLoading) return;
		runLoading = true;
		try {
			const client = createClient();
			const data = await client.request<{ run: RunDetail | null }>(RUN_DETAIL_QUERY, { id: runId });
			run = data.run;
		} finally {
			runLoading = false;
		}
	}

	function executionIdFor(rid: string): string {
		// Try-layer run IDs are synthesised as "exec-<bundleID>"; strip the prefix.
		return rid.startsWith('exec-') ? rid.slice('exec-'.length) : rid;
	}

	async function fetchExec() {
		if (execLoaded || execLoading) return;
		execLoading = true;
		try {
			const client = createClient();
			const data = await client.request<{ execution: typeof exec }>(RUN_EXECUTION_QUERY, {
				id: executionIdFor(runId)
			});
			exec = data.execution;
			execLoaded = true;
		} catch {
			execLoaded = true;
		} finally {
			execLoading = false;
		}
	}

	async function fetchSession(sessionId: string) {
		if (sessionLoaded || sessionLoading) return;
		sessionLoading = true;
		try {
			const client = createClient();
			const data = await client.request<{ agentSession: SessionDetail | null }>(RUN_SESSION_QUERY, {
				id: sessionId
			});
			session = data.agentSession;
			sessionLoaded = true;
		} catch {
			sessionLoaded = true;
		} finally {
			sessionLoading = false;
		}
	}

	async function fetchToolCalls(more = false) {
		if (toolCallsLoading) return;
		toolCallsLoading = true;
		try {
			const client = createClient();
			const data = await client.request<{
				executionToolCalls: {
					edges: Array<{ node: ToolCall; cursor: string }>;
					pageInfo: { hasNextPage: boolean; endCursor: string | null };
					totalCount: number;
				};
			}>(RUN_TOOL_CALLS_QUERY, {
				id: executionIdFor(runId),
				first: 50,
				after: more ? toolCallsCursor : null
			});
			const newCalls = data.executionToolCalls.edges.map((e) => e.node);
			toolCalls = more ? [...toolCalls, ...newCalls] : newCalls;
			toolCallsTotal = data.executionToolCalls.totalCount;
			toolCallsCursor = data.executionToolCalls.pageInfo.endCursor;
			toolCallsHasMore = data.executionToolCalls.pageInfo.hasNextPage;
			toolCallsLoaded = true;
		} catch {
			toolCallsLoaded = true;
		} finally {
			toolCallsLoading = false;
		}
	}

	$effect(() => {
		void fetchRun();
		// run-layer rows correspond to AgentSession invocations
		if (layer === 'run') {
			void (async () => {
				await fetchExec();
				if (exec?.sessionId) {
					await fetchSession(exec.sessionId);
				}
			})();
		}
		// try-layer rows correspond to execute-bead bundles
		if (layer === 'try') {
			void fetchExec();
		}
		if (activeTab === 'tools' && !toolCallsLoaded && !toolCallsLoading) {
			void fetchToolCalls(false);
		}
	});

	function pickTab(tab: Tab) {
		activeTab = tab;
		if (tab === 'tools' && !toolCallsLoaded) {
			void fetchToolCalls(false);
		}
		onTabChange?.(tab);
	}

	const tabs: Array<{ id: Tab; label: string; show: boolean }> = $derived.by(() => {
		const list: Array<{ id: Tab; label: string; show: boolean }> = [
			{ id: 'overview', label: 'Overview', show: true }
		];
		if (layer !== 'work') {
			list.push({ id: 'prompt', label: 'Prompt', show: true });
			list.push({ id: 'response', label: 'Response', show: true });
		}
		if (layer === 'run') {
			list.push({ id: 'session', label: 'Session', show: true });
		}
		if (layer === 'run' || layer === 'try') {
			list.push({ id: 'tools', label: 'Tool calls', show: true });
			list.push({ id: 'evidence', label: 'Evidence', show: true });
		}
		return list;
	});
</script>

<div class="space-y-3" data-testid="rundetail" data-layer={layer}>
	<!-- Tabs -->
	<div class="border-border-line dark:border-dark-border-line border-b">
		<nav class="flex gap-1">
			{#each tabs as tab (tab.id)}
				<button
					type="button"
					data-tab={tab.id}
					onclick={(e) => {
						e.stopPropagation();
						pickTab(tab.id);
					}}
					class="text-body-sm border-b-2 px-3 py-2 font-medium {activeTab === tab.id
						? 'border-accent-lever text-accent-lever dark:border-dark-accent-lever dark:text-dark-accent-lever'
						: 'text-fg-muted hover:text-fg-ink dark:text-dark-fg-muted dark:hover:text-dark-fg-ink border-transparent'}"
				>
					{tab.label}
				</button>
			{/each}
		</nav>
	</div>

	<div data-active-tab={activeTab}>
		{#if activeTab === 'overview'}
			{#if runLoading && !run}
				<div class="text-body-sm text-fg-muted dark:text-dark-fg-muted">Loading run…</div>
			{:else if run}
				<Overview {run} beadHrefFn={beadHref} />
			{:else}
				<div class="text-body-sm text-fg-muted dark:text-dark-fg-muted">Run not found.</div>
			{/if}
		{:else if activeTab === 'prompt'}
			<Prompt
				prompt={layer === 'run' ? session?.prompt : exec?.prompt}
				summary={run?.promptSummary}
				path={layer === 'try'
					? (exec?.promptPath ?? (exec ? `${exec.bundlePath}/prompt.md` : null))
					: null}
			/>
		{:else if activeTab === 'response'}
			<Response
				response={layer === 'run' ? session?.response : exec?.result}
				excerpt={run?.outputExcerpt}
				stderr={layer === 'run' ? session?.stderr : null}
				path={layer === 'try'
					? (exec?.resultPath ?? (exec ? `${exec.bundlePath}/result.json` : null))
					: null}
			/>
		{:else if activeTab === 'session'}
			<Session {session} loading={sessionLoading} sessionId={exec?.sessionId ?? null} />
		{:else if activeTab === 'tools'}
			<Tools
				calls={toolCalls}
				loading={toolCallsLoading}
				total={toolCallsTotal}
				hasMore={toolCallsHasMore}
				onLoadMore={() => fetchToolCalls(true)}
				sourcePath={exec?.agentLogPath}
			/>
		{:else if activeTab === 'evidence'}
			<Evidence runId={runId} files={run?.bundleFiles ?? []} />
		{/if}
	</div>
</div>
