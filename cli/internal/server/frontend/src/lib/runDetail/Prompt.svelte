<script lang="ts">
	interface Props {
		prompt: string | null | undefined
		summary?: string | null
		path?: string | null
	}
	let { prompt, summary, path }: Props = $props()
</script>

<div class="space-y-2" data-testid="rundetail-prompt">
	{#if summary}
		<div class="text-body-sm text-fg-ink dark:text-dark-fg-ink">{summary}</div>
	{/if}
	{#if path}
		<div class="font-mono-code text-mono-code text-fg-muted dark:text-dark-fg-muted">{path}</div>
	{/if}
	{#if prompt}
		<pre
			data-testid="rundetail-prompt-body"
			class="max-h-[28rem] overflow-auto bg-terminal-bg px-4 py-3 font-mono-code text-mono-code leading-relaxed text-terminal-fg whitespace-pre-wrap"
			>{prompt}</pre>
	{:else if !summary}
		<div class="text-body-sm text-fg-muted dark:text-dark-fg-muted">No prompt body recorded.</div>
	{/if}
</div>
