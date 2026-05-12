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
		RUN_EVIDENCE_QUERY,
		RUN_SESSION_QUERY,
		RUN_TOOL_CALLS_QUERY
	} from './queries';
	import type { BundleFile, RunDetail, ToolCall, SessionDetail } from './types';

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

	let session = $state<SessionDetail | null>(null);
	let sessionLoading = $state(false);
	let sessionLoaded = $state(false);

	let evidenceFiles = $state<BundleFile[]>([]);
	let evidenceLoading = $state(false);
	let evidenceLoaded = $state(false);

	let toolCalls = $state<ToolCall[]>([]);
	let toolCallsTotal = $state<number | undefined>(undefined);
	let toolCallsCursor = $state<string | null>(null);
	let toolCallsHasMore = $state(false);
	let toolCallsLoading = $state(false);
	let toolCallsLoaded = $state(false);

	let activeTab = $state<Tab>(initialTab);
	let lastInitialTab = $state<Tab>(initialTab);

	$effect(() => {
		if (initialTab !== lastInitialTab) {
			activeTab = initialTab;
			lastInitialTab = initialTab;
			if (activeTab === 'tools' && !toolCallsLoaded) {
				void fetchToolCalls(false);
			}
		}
	});

	function beadHref(beadId: string): string {
		return `/nodes/${nodeId}/projects/${projectId}/beads/${beadId}`;
	}

	function workerHref(workerId: string): string {
		return `/nodes/${nodeId}/projects/${projectId}/workers/${workerId}`;
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

	async function fetchEvidence() {
		if (evidenceLoaded || evidenceLoading) return;
		evidenceLoading = true;
		try {
			const client = createClient();
			const data = await client.request<{
				run: { id: string; bundleFiles: BundleFile[] } | null;
			}>(RUN_EVIDENCE_QUERY, { id: runId });
			evidenceFiles = data.run?.bundleFiles ?? [];
		} finally {
			evidenceLoading = false;
			evidenceLoaded = true;
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
				runToolCalls: {
					edges: Array<{ node: ToolCall; cursor: string }>;
					pageInfo: { hasNextPage: boolean; endCursor: string | null };
					totalCount: number;
				};
			}>(RUN_TOOL_CALLS_QUERY, {
				id: runId,
				first: 50,
				after: more ? toolCallsCursor : null
			});
			const newCalls = data.runToolCalls.edges.map((e) => e.node);
			toolCalls = more ? [...toolCalls, ...newCalls] : newCalls;
			toolCallsTotal = data.runToolCalls.totalCount;
			toolCallsCursor = data.runToolCalls.pageInfo.endCursor;
			toolCallsHasMore = data.runToolCalls.pageInfo.hasNextPage;
			toolCallsLoaded = true;
		} catch {
			toolCallsLoaded = true;
		} finally {
			toolCallsLoading = false;
		}
	}

	$effect(() => {
		void fetchRun();
		if (layer === 'run') {
			void fetchSession(runId);
		}
		if (activeTab === 'tools' && !toolCallsLoaded && !toolCallsLoading) {
			void fetchToolCalls(false);
		}
		if (activeTab === 'evidence' && !evidenceLoaded && !evidenceLoading) {
			void fetchEvidence();
		}
	});

	function pickTab(tab: Tab) {
		activeTab = tab;
		if (tab === 'tools' && !toolCallsLoaded) {
			void fetchToolCalls(false);
		}
		if (tab === 'evidence' && !evidenceLoaded) {
			void fetchEvidence();
		}
		onTabChange?.(tab);
	}

	const tabs: Array<{ id: Tab; label: string }> = $derived.by(() => {
		const list: Array<{ id: Tab; label: string }> = [{ id: 'overview', label: 'Overview' }];
		if (layer !== 'work') {
			list.push({ id: 'prompt', label: 'Prompt' });
			list.push({ id: 'response', label: 'Response' });
		}
		if (layer === 'run') {
			list.push({ id: 'session', label: 'Session' });
		}
		if (layer === 'run' || layer === 'try') {
			list.push({ id: 'tools', label: 'Tools' });
			list.push({ id: 'evidence', label: 'Evidence' });
		}
		return list;
	});
</script>

<div class="space-y-3" data-testid="rundetail" data-layer={layer}>
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
						: 'border-transparent text-fg-muted hover:text-fg-ink dark:text-dark-fg-muted dark:hover:text-dark-fg-ink'}"
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
				<Overview
					{run}
					beadHrefFn={beadHref}
					workerHrefFn={workerHref}
					liveWorkerId={session?.workerId ?? null}
				/>
			{:else}
				<div class="text-body-sm text-fg-muted dark:text-dark-fg-muted">Run not found.</div>
			{/if}
		{:else if activeTab === 'prompt'}
			<Prompt prompt={layer === 'run' ? session?.prompt ?? run?.prompt : run?.prompt} summary={run?.promptSummary} />
		{:else if activeTab === 'response'}
			<Response
				response={layer === 'run' ? session?.response ?? run?.response : run?.response}
				excerpt={run?.outputExcerpt}
				stderr={layer === 'run' ? session?.stderr ?? run?.stderr : run?.stderr}
				verdict={layer === 'try' ? run?.status : null}
			/>
		{:else if activeTab === 'session'}
			<Session {session} loading={sessionLoading} sessionId={runId} />
		{:else if activeTab === 'tools'}
			<Tools
				calls={toolCalls}
				loading={toolCallsLoading}
				total={toolCallsTotal}
				hasMore={toolCallsHasMore}
				onLoadMore={() => fetchToolCalls(true)}
				sourcePath={session?.stdoutPath ?? null}
			/>
		{:else if activeTab === 'evidence'}
			<Evidence
				runId={runId}
				files={evidenceFiles}
				loading={evidenceLoading && !evidenceLoaded}
			/>
		{/if}
	</div>
</div>
