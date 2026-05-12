<script lang="ts">
	interface Props {
		response: string | null | undefined
		excerpt?: string | null
		stderr?: string | null
		path?: string | null
		verdict?: string | null
	}
	let { response, excerpt, stderr, path, verdict }: Props = $props()

	function verdictClass(value: string): string {
		switch (value) {
			case 'success':
				return 'badge-status-closed'
			case 'failure':
				return 'badge-status-failed'
			case 'preserved':
				return 'badge-status-in-progress'
			case 'running':
				return 'badge-status-running'
			default:
				return 'badge-status-open'
		}
	}
</script>

<div class="space-y-2" data-testid="rundetail-response">
	{#if verdict}
		<div class="flex items-center gap-2">
			<span class="font-label-caps text-label-caps inline-block rounded-full px-2 py-0.5 uppercase {verdictClass(
				verdict
			)}">
				{verdict}
			</span>
			<span class="text-body-sm text-fg-muted dark:text-dark-fg-muted">Verdict</span>
		</div>
	{/if}
	{#if excerpt && !response}
		<div class="text-body-sm text-fg-ink dark:text-dark-fg-ink">{excerpt}</div>
	{/if}
	{#if path}
		<div class="font-mono-code text-mono-code text-fg-muted dark:text-dark-fg-muted">{path}</div>
	{/if}
	{#if response}
		<pre
			data-testid="rundetail-response-body"
			class="max-h-[28rem] overflow-auto bg-terminal-bg px-4 py-3 font-mono-code text-mono-code leading-relaxed text-terminal-fg whitespace-pre-wrap"
			>{response}</pre>
	{:else if !excerpt}
		<div class="text-body-sm text-fg-muted dark:text-dark-fg-muted">No response body recorded.</div>
	{/if}
	{#if stderr}
		<div>
			<div class="font-label-caps text-label-caps uppercase text-fg-muted dark:text-dark-fg-muted">Stderr</div>
			<pre
				class="mt-1 max-h-56 overflow-auto bg-terminal-bg px-3 py-2 font-mono-code text-mono-code text-terminal-fg whitespace-pre-wrap"
				>{stderr}</pre>
		</div>
	{/if}
</div>
