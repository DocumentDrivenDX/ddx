<script lang="ts">
	import type { ToolCall } from './types'
	import { tryPretty } from './format'

	interface Props {
		calls: ToolCall[]
		loading?: boolean
		total?: number
		hasMore?: boolean
		onLoadMore?: () => void
		sourcePath?: string | null
	}
	let { calls, loading = false, total, hasMore = false, onLoadMore, sourcePath }: Props = $props()

	let expanded = $state<Set<number>>(new Set())

	function toggleCall(seq: number) {
		const next = new Set(expanded)
		if (next.has(seq)) next.delete(seq)
		else next.add(seq)
		expanded = next
	}
</script>

<div class="space-y-2" data-testid="rundetail-tools">
	<div class="flex items-center justify-between text-label-caps font-label-caps uppercase tracking-wide text-fg-muted dark:text-dark-fg-muted">
		<span>
			{#if loading && calls.length === 0}
				Loading…
			{:else}
				{calls.length}{total != null ? ` of ${total}` : ''} tool calls
			{/if}
		</span>
		{#if sourcePath}
			<span>Source: <code class="font-mono-code text-mono-code">{sourcePath}</code></span>
		{/if}
	</div>
	{#if !loading && calls.length === 0}
		<div class="border border-border-line bg-bg-surface p-3 text-body-sm text-fg-muted dark:border-dark-border-line dark:bg-dark-bg-surface dark:text-dark-fg-muted">
			No tool calls were captured.
		</div>
	{/if}
	<ul class="space-y-1">
		{#each calls as call (call.seq)}
			{@const open = expanded.has(call.seq)}
			<li class="border border-border-line dark:border-dark-border-line">
				<button
					type="button"
					data-tool-seq={call.seq}
					class="flex w-full items-center justify-between px-3 py-2 text-left hover:bg-bg-surface dark:hover:bg-dark-bg-surface"
					onclick={(e) => {
						e.stopPropagation()
						toggleCall(call.seq)
					}}
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
	{#if hasMore && onLoadMore}
		<div class="pt-2">
			<button
				type="button"
				onclick={(e) => {
					e.stopPropagation()
					onLoadMore?.()
				}}
				disabled={loading}
				class="border border-border-line px-3 py-1.5 text-body-sm text-fg-muted hover:bg-bg-surface disabled:opacity-50 dark:border-dark-border-line dark:text-dark-fg-muted dark:hover:bg-dark-bg-surface"
			>
				{loading ? 'Loading…' : 'Load more'}
			</button>
		</div>
	{/if}
</div>
