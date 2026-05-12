<script lang="ts">
	import type { RunDetail } from './types'
	import { fmtDate, fmtDuration } from './format'

	interface Props {
		run: RunDetail
		beadHrefFn?: (id: string) => string
		workerHrefFn?: (id: string) => string
		liveWorkerId?: string | null
	}
	let { run, beadHrefFn, workerHrefFn, liveWorkerId }: Props = $props()

	function badgeClass(status: string): string {
		switch (status) {
			case 'success':
				return 'badge-status-closed'
			case 'failure':
				return 'badge-status-failed'
			case 'running':
				return 'badge-status-running'
			case 'preserved':
				return 'badge-status-in-progress'
			default:
				return 'badge-status-open'
		}
	}

	function layerClass(layer: string): string {
		switch (layer) {
			case 'work':
				return 'badge-layer-work'
			case 'try':
				return 'badge-layer-try'
			case 'run':
				return 'badge-layer-run'
			default:
				return 'badge-status-open'
		}
	}

	function lastUpdated(value: RunDetail): string | null {
		return value.completedAt ?? value.startedAt ?? null
	}
</script>

<div
	class="space-y-4 border border-border-line bg-bg-surface p-4 dark:border-dark-border-line dark:bg-dark-bg-surface"
	data-testid="rundetail-overview"
>
	<div class="flex flex-wrap items-center gap-2">
		<span class="font-label-caps text-label-caps inline-block rounded-full px-2 py-0.5 uppercase {badgeClass(
			run.status
		)}">
			{run.status}
		</span>
		<span class="font-label-caps text-label-caps inline-block rounded-full px-2 py-0.5 uppercase {layerClass(
			run.layer
		)}">
			{run.layer}
		</span>
		{#if liveWorkerId && run.status === 'running' && workerHrefFn}
			<a
				href={workerHrefFn(liveWorkerId)}
				onclick={(e) => e.stopPropagation()}
				class="text-body-sm text-accent-lever hover:underline dark:text-dark-accent-lever"
			>
				View live progress
			</a>
		{/if}
	</div>

	<div class="grid grid-cols-2 gap-4 sm:grid-cols-3">
		<div>
			<div class="mb-1 font-label-caps text-label-caps uppercase text-fg-muted dark:text-dark-fg-muted">Last updated</div>
			<div class="font-mono-code text-mono-code text-fg-ink dark:text-dark-fg-ink">{fmtDate(lastUpdated(run))}</div>
		</div>
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
			<div class="sm:col-span-2">
				<div class="mb-1 font-label-caps text-label-caps uppercase text-fg-muted dark:text-dark-fg-muted">Worktree path</div>
				<span class="font-mono-code text-mono-code text-fg-ink dark:text-dark-fg-ink">{run.worktreePath}</span>
			</div>
		{/if}
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
		{#if run.cachedTokens != null}
			<div>
				<div class="mb-1 font-label-caps text-label-caps uppercase text-fg-muted dark:text-dark-fg-muted">Cached tokens</div>
				<span class="font-mono-code text-mono-code text-fg-ink dark:text-dark-fg-ink">{run.cachedTokens.toLocaleString()}</span>
			</div>
		{/if}
		{#if run.costUsd != null}
			<div>
				<div class="mb-1 font-label-caps text-label-caps uppercase text-fg-muted dark:text-dark-fg-muted">Cost</div>
				<span class="font-mono-code text-mono-code text-fg-ink dark:text-dark-fg-ink">${run.costUsd.toFixed(4)}</span>
			</div>
		{/if}
		{#if run.stopCondition}
			<div>
				<div class="mb-1 font-label-caps text-label-caps uppercase text-fg-muted dark:text-dark-fg-muted">Stop</div>
				<span class="font-mono-code text-mono-code text-fg-ink dark:text-dark-fg-ink">{run.stopCondition}</span>
			</div>
		{/if}
	</div>
</div>
