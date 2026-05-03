<script lang="ts">
	import type { RunDetail } from './types'
	import { fmtDate, fmtDuration } from './format'

	interface Props {
		run: RunDetail
		beadHrefFn?: (id: string) => string
	}
	let { run, beadHrefFn }: Props = $props()
</script>

<div
	class="grid grid-cols-2 gap-4 border border-border-line bg-bg-surface p-4 dark:border-dark-border-line dark:bg-dark-bg-surface sm:grid-cols-3"
	data-testid="rundetail-overview"
>
	<div>
		<div class="mb-1 font-label-caps text-label-caps uppercase text-fg-muted dark:text-dark-fg-muted">Started</div>
		<div class="font-mono-code text-mono-code text-fg-ink dark:text-dark-fg-ink">{fmtDate(run.startedAt)}</div>
	</div>
	<div>
		<div class="mb-1 font-label-caps text-label-caps uppercase text-fg-muted dark:text-dark-fg-muted">Completed</div>
		<div class="font-mono-code text-mono-code text-fg-ink dark:text-dark-fg-ink">{fmtDate(run.completedAt)}</div>
	</div>
	<div>
		<div class="mb-1 font-label-caps text-label-caps uppercase text-fg-muted dark:text-dark-fg-muted">Duration</div>
		<div class="font-mono-code text-mono-code text-fg-ink dark:text-dark-fg-ink">{fmtDuration(run.durationMs)}</div>
	</div>
	{#if run.beadId}
		<div>
			<div class="mb-1 font-label-caps text-label-caps uppercase text-fg-muted dark:text-dark-fg-muted">Bead</div>
			{#if beadHrefFn}
				<a
					href={beadHrefFn(run.beadId)}
					onclick={(e) => e.stopPropagation()}
					class="font-mono-code text-mono-code text-accent-lever hover:underline dark:text-dark-accent-lever"
				>
					{run.beadId}
				</a>
			{:else}
				<span class="font-mono-code text-mono-code text-fg-ink dark:text-dark-fg-ink">{run.beadId}</span>
			{/if}
		</div>
	{/if}
	{#if run.layer === 'try'}
		{#if run.baseRevision}
			<div>
				<div class="mb-1 font-label-caps text-label-caps uppercase text-fg-muted dark:text-dark-fg-muted">Base rev</div>
				<span class="font-mono-code text-mono-code text-fg-ink dark:text-dark-fg-ink">{run.baseRevision}</span>
			</div>
		{/if}
		{#if run.resultRevision}
			<div>
				<div class="mb-1 font-label-caps text-label-caps uppercase text-fg-muted dark:text-dark-fg-muted">Result rev</div>
				<span class="font-mono-code text-mono-code text-fg-ink dark:text-dark-fg-ink">{run.resultRevision}</span>
			</div>
		{/if}
		{#if run.mergeOutcome}
			<div>
				<div class="mb-1 font-label-caps text-label-caps uppercase text-fg-muted dark:text-dark-fg-muted">Merge outcome</div>
				<span class="font-mono-code text-mono-code text-fg-ink dark:text-dark-fg-ink">{run.mergeOutcome}</span>
			</div>
		{/if}
		{#if run.worktreePath}
			<div>
				<div class="mb-1 font-label-caps text-label-caps uppercase text-fg-muted dark:text-dark-fg-muted">Worktree</div>
				<span class="font-mono-code text-mono-code text-fg-ink dark:text-dark-fg-ink">{run.worktreePath}</span>
			</div>
		{/if}
	{/if}
	{#if run.layer === 'run'}
		{#if run.harness}
			<div>
				<div class="mb-1 font-label-caps text-label-caps uppercase text-fg-muted dark:text-dark-fg-muted">Harness</div>
				<span class="font-mono-code text-mono-code text-fg-ink dark:text-dark-fg-ink">{run.harness}</span>
			</div>
		{/if}
		{#if run.model}
			<div>
				<div class="mb-1 font-label-caps text-label-caps uppercase text-fg-muted dark:text-dark-fg-muted">Model</div>
				<span class="font-mono-code text-mono-code text-fg-ink dark:text-dark-fg-ink">{run.model}</span>
			</div>
		{/if}
		{#if run.tokensIn != null}
			<div>
				<div class="mb-1 font-label-caps text-label-caps uppercase text-fg-muted dark:text-dark-fg-muted">Tokens in</div>
				<span class="font-mono-code text-mono-code text-fg-ink dark:text-dark-fg-ink">{run.tokensIn.toLocaleString()}</span>
			</div>
		{/if}
		{#if run.tokensOut != null}
			<div>
				<div class="mb-1 font-label-caps text-label-caps uppercase text-fg-muted dark:text-dark-fg-muted">Tokens out</div>
				<span class="font-mono-code text-mono-code text-fg-ink dark:text-dark-fg-ink">{run.tokensOut.toLocaleString()}</span>
			</div>
		{/if}
		{#if run.costUsd != null}
			<div>
				<div class="mb-1 font-label-caps text-label-caps uppercase text-fg-muted dark:text-dark-fg-muted">Cost</div>
				<span class="font-mono-code text-mono-code text-fg-ink dark:text-dark-fg-ink">${run.costUsd.toFixed(4)}</span>
			</div>
		{/if}
	{/if}
	{#if run.layer === 'work' && run.stopCondition}
		<div>
			<div class="mb-1 font-label-caps text-label-caps uppercase text-fg-muted dark:text-dark-fg-muted">Stop</div>
			<span class="font-mono-code text-mono-code text-fg-ink dark:text-dark-fg-ink">{run.stopCondition}</span>
		</div>
	{/if}
</div>
