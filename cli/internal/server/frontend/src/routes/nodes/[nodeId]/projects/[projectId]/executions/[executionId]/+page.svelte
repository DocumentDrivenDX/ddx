<script lang="ts">
	import type { PageData } from './$types';
	import { createClient } from '$lib/gql/client';
	import { fmtCost } from '$lib/runDetail/format';
	import { gql } from 'graphql-request';

	let { data }: { data: PageData } = $props();
	let activeTab = $state<'manifest' | 'prompt' | 'result' | 'session' | 'tools'>('manifest');
	let sessionBody = $state<string | null>(null);
	let toolCalls: Array<{ id: string; seq: number; inputs: string | null; output: string | null }> = $state([]);
	let toolCount = $state<number | null>(null);
	let toolCallsOpen = $state<Set<number>>(new Set());

	const SESSION_QUERY = gql`
		query ExecutionSession($id: ID!) {
			agentSession(id: $id) {
				id
				harness
				model
				cost
				billingMode
				tokens {
					prompt
					completion
					total
					cached
				}
				status
				outcome
				prompt
				response
				stderr
			}
		}
	`;

	const TOOL_CALLS_QUERY = gql`
		query ExecutionToolCalls($id: ID!, $first: Int, $after: String) {
			executionToolCalls(id: $id, first: $first, after: $after) {
				edges {
					node {
						id
						seq
						name
						inputs
						output
					}
					cursor
				}
				totalCount
			}
		}
	`;

	function tabClass(tab: string): string {
		return activeTab === tab
			? 'border-accent-lever text-accent-lever dark:border-dark-accent-lever dark:text-dark-accent-lever'
			: 'border-transparent text-fg-muted hover:text-fg-ink dark:text-dark-fg-muted dark:hover:text-dark-fg-ink';
	}

	async function openSession() {
		if (!data.execution?.sessionId) return;
		if (sessionBody !== null) return;
		const client = createClient();
		const result = await client.request<{ agentSession: { id: string } | null }>(SESSION_QUERY, {
			id: data.execution.sessionId
		});
		sessionBody = result.agentSession ? result.agentSession.id : 'No session';
	}

	async function openToolCalls() {
		if (toolCount !== null) return;
		const client = createClient();
		const result = await client.request<{
			executionToolCalls: { edges: Array<{ node: { id: string; seq: number; inputs: string | null; output: string | null } }>; totalCount: number };
		}>(TOOL_CALLS_QUERY, {
			id: data.execution?.id ?? '',
			first: 50
		});
		toolCalls = result.executionToolCalls.edges.map((edge) => edge.node);
		toolCount = result.executionToolCalls.totalCount;
	}

	function toggleToolCall(seq: number) {
		const next = new Set(toolCallsOpen);
		if (next.has(seq)) next.delete(seq);
		else next.add(seq);
		toolCallsOpen = next;
	}
</script>

<svelte:head>
	<title>{data.execution?.id ?? 'Execution'} | DDx</title>
</svelte:head>

{#if !data.execution}
	<p>Execution not found.</p>
{:else}
	<div class="space-y-4">
		<div class="flex items-center gap-3">
			<h1 class="text-headline-md font-headline-md text-fg-ink dark:text-dark-fg-ink">{data.execution.id}</h1>
			<span class="rounded-full border px-2 py-0.5 text-xs uppercase">{data.execution.verdict ?? '—'}</span>
		</div>

		<div class="grid grid-cols-1 gap-3 rounded border border-border-line p-3 sm:grid-cols-2 dark:border-dark-border-line">
			<div>
				<div class="mb-1 text-xs font-semibold uppercase tracking-wide text-fg-muted dark:text-dark-fg-muted">Cost</div>
				<div class="font-mono-code text-mono-code text-fg-ink dark:text-dark-fg-ink">{fmtCost(data.execution.costUsd)}</div>
			</div>
		</div>

		<nav class="flex gap-2 border-b border-border-line dark:border-dark-border-line">
			{#each ['manifest', 'prompt', 'result', 'session', 'tools'] as tab}
				<button
					type="button"
					class="border-b-2 px-3 py-2 text-sm font-medium {tabClass(tab)}"
					onclick={async () => {
						activeTab = tab as typeof activeTab;
						if (tab === 'session') await openSession();
						if (tab === 'tools') await openToolCalls();
					}}
				>
					{tab === 'tools' ? 'Tool calls' : tab.charAt(0).toUpperCase() + tab.slice(1)}
				</button>
			{/each}
		</nav>

		{#if activeTab === 'manifest'}
			<pre data-testid="manifest-body" class="whitespace-pre-wrap rounded border border-border-line p-3">
				{data.execution.manifest ?? ''}
			</pre>
		{:else if activeTab === 'prompt'}
			<pre data-testid="prompt-body" class="whitespace-pre-wrap rounded border border-border-line p-3">
				{data.execution.prompt ?? ''}
			</pre>
		{:else if activeTab === 'result'}
			<pre data-testid="result-body" class="whitespace-pre-wrap rounded border border-border-line p-3">
				{data.execution.result ?? ''}
			</pre>
		{:else if activeTab === 'session'}
			<div>{sessionBody ?? 'Loading...'}</div>
		{:else if activeTab === 'tools'}
			<div class="space-y-2">
				<div>{toolCount ?? 0} of {toolCount ?? 0} tool calls</div>
				{#each toolCalls as call (call.id)}
					<div>
						<button type="button" data-tool-seq={call.seq} onclick={() => toggleToolCall(call.seq)}>
							Call {call.seq}
						</button>
						{#if toolCallsOpen.has(call.seq)}
							<div>
								{#if call.inputs}<pre>{call.inputs}</pre>{/if}
								{#if call.output}<pre>{call.output}</pre>{/if}
							</div>
						{/if}
					</div>
				{/each}
			</div>
		{/if}
	</div>
{/if}
