<script lang="ts">
	import type { PageData } from './$types'
	import type { RunDetail } from './+page'

	let { data }: { data: PageData } = $props()

	const run: RunDetail | null = data.run

	function runsListHref(): string {
		return `/nodes/${data.nodeId}/projects/${data.projectId}/runs`
	}

	function parentRunHref(parentId: string): string {
		return `/nodes/${data.nodeId}/projects/${data.projectId}/runs/${parentId}`
	}

	function childRunHref(childId: string): string {
		return `/nodes/${data.nodeId}/projects/${data.projectId}/runs/${childId}`
	}

	function beadHref(beadId: string): string {
		return `/nodes/${data.nodeId}/projects/${data.projectId}/beads/${beadId}`
	}

	function fmtDate(iso: string | null): string {
		if (!iso) return '—'
		return new Date(iso).toLocaleString()
	}

	function fmtDuration(ms: number | null): string {
		if (ms == null) return '—'
		if (ms < 1000) return `${ms}ms`
		if (ms < 60_000) return `${(ms / 1000).toFixed(1)}s`
		const m = Math.floor(ms / 60_000)
		const s = Math.floor((ms % 60_000) / 1000)
		return `${m}m ${s}s`
	}

	function layerBadgeClass(layer: string): string {
		switch (layer) {
			case 'work':
				return 'bg-purple-100 text-purple-800 dark:bg-purple-900/30 dark:text-purple-300'
			case 'try':
				return 'bg-blue-100 text-blue-800 dark:bg-blue-900/30 dark:text-blue-300'
			case 'run':
				return 'bg-teal-100 text-teal-800 dark:bg-teal-900/30 dark:text-teal-300'
			default:
				return 'bg-bg-surface text-fg-muted dark:bg-dark-bg-surface dark:text-dark-fg-muted'
		}
	}

	function statusBadgeClass(status: string): string {
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

	function breadcrumbs(): Array<{ label: string; href: string }> {
		if (!run) return []
		const crumbs: Array<{ label: string; href: string }> = [
			{ label: 'Runs', href: runsListHref() }
		]
		if (run.layer === 'run' && data.grandparentRunId && run.parentRunId) {
			crumbs.push({ label: data.grandparentRunId, href: parentRunHref(data.grandparentRunId) })
			crumbs.push({ label: run.parentRunId, href: parentRunHref(run.parentRunId) })
		} else if (run.parentRunId) {
			crumbs.push({ label: run.parentRunId, href: parentRunHref(run.parentRunId) })
		}
		crumbs.push({ label: run.id, href: '#' })
		return crumbs
	}
</script>

<div class="space-y-4">
	<!-- Breadcrumbs -->
	<nav class="flex items-center gap-1 text-xs text-fg-muted dark:text-dark-fg-muted">
		{#each breadcrumbs() as crumb, i}
			{#if i > 0}
				<span>/</span>
			{/if}
			{#if crumb.href === '#'}
				<span class="font-mono-code text-fg-ink dark:text-dark-fg-ink">{crumb.label}</span>
			{:else}
				<a
					href={crumb.href}
					class="font-mono-code hover:text-accent-lever dark:hover:text-dark-accent-lever"
				>
					{crumb.label}
				</a>
			{/if}
		{/each}
	</nav>

	{#if !run}
		<p class="text-body-sm text-fg-muted dark:text-dark-fg-muted">Run not found.</p>
	{:else}
		<!-- Header -->
		<div class="flex items-center gap-3">
			<h1 class="font-mono-code text-lg text-fg-ink dark:text-dark-fg-ink">{run.id}</h1>
			<span class="inline-block rounded-full px-2 py-0.5 font-label-caps text-label-caps uppercase {layerBadgeClass(run.layer)}">
				{run.layer}
			</span>
			<span class="inline-block border px-1.5 py-0.5 font-mono-code text-mono-code uppercase {statusBadgeClass(run.status)}">
				{run.status}
			</span>
		</div>

		<!-- Common fields -->
		<div class="grid grid-cols-2 gap-4 border border-border-line bg-bg-surface p-4 dark:border-dark-border-line dark:bg-dark-bg-surface sm:grid-cols-3">
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
					<a href={beadHref(run.beadId)} class="font-mono-code text-mono-code text-accent-lever hover:underline dark:text-dark-accent-lever">
						{run.beadId}
					</a>
				</div>
			{/if}
		</div>

		<!-- Work-layer fields -->
		{#if run.layer === 'work'}
			<section class="space-y-3 border border-border-line bg-bg-surface p-4 dark:border-dark-border-line dark:bg-dark-bg-surface">
				<h2 class="font-label-caps text-label-caps uppercase text-fg-muted dark:text-dark-fg-muted">Queue Inputs</h2>
				{#if run.stopCondition}
					<div>
						<span class="font-label-caps text-label-caps uppercase text-fg-muted dark:text-dark-fg-muted">Stop condition: </span>
						<span class="font-mono-code text-mono-code text-fg-ink dark:text-dark-fg-ink">{run.stopCondition}</span>
					</div>
				{/if}
				{#if run.selectedBeadIds && run.selectedBeadIds.length > 0}
					<div>
						<div class="mb-1 font-label-caps text-label-caps uppercase text-fg-muted dark:text-dark-fg-muted">Selected beads</div>
						<div class="flex flex-wrap gap-1">
							{#each run.selectedBeadIds as bid}
								<a href={beadHref(bid)} class="font-mono-code text-mono-code text-accent-lever hover:underline dark:text-dark-accent-lever">{bid}</a>
							{/each}
						</div>
					</div>
				{/if}
				{#if run.queueInputs}
					<div>
						<div class="mb-1 font-label-caps text-label-caps uppercase text-fg-muted dark:text-dark-fg-muted">Raw inputs</div>
						<pre class="overflow-auto rounded bg-bg-elevated p-2 font-mono-code text-mono-code text-fg-ink dark:bg-dark-bg-elevated dark:text-dark-fg-ink">{run.queueInputs}</pre>
					</div>
				{/if}
			</section>
		{/if}

		<!-- Try-layer fields -->
		{#if run.layer === 'try'}
			<section class="space-y-3 border border-border-line bg-bg-surface p-4 dark:border-dark-border-line dark:bg-dark-bg-surface">
				<h2 class="font-label-caps text-label-caps uppercase text-fg-muted dark:text-dark-fg-muted">Attempt Details</h2>
				<div class="grid grid-cols-2 gap-4">
					{#if run.baseRevision}
						<div>
							<div class="mb-1 font-label-caps text-label-caps uppercase text-fg-muted dark:text-dark-fg-muted">Base revision</div>
							<span class="font-mono-code text-mono-code text-fg-ink dark:text-dark-fg-ink">{run.baseRevision}</span>
						</div>
					{/if}
					{#if run.resultRevision}
						<div>
							<div class="mb-1 font-label-caps text-label-caps uppercase text-fg-muted dark:text-dark-fg-muted">Result revision</div>
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
				</div>
				{#if run.checkResults}
					<div>
						<div class="mb-1 font-label-caps text-label-caps uppercase text-fg-muted dark:text-dark-fg-muted">Check results</div>
						<pre class="overflow-auto rounded bg-bg-elevated p-2 font-mono-code text-mono-code text-fg-ink dark:bg-dark-bg-elevated dark:text-dark-fg-ink">{run.checkResults}</pre>
					</div>
				{/if}
			</section>
		{/if}

		<!-- Run-layer fields -->
		{#if run.layer === 'run'}
			<section class="space-y-3 border border-border-line bg-bg-surface p-4 dark:border-dark-border-line dark:bg-dark-bg-surface">
				<h2 class="font-label-caps text-label-caps uppercase text-fg-muted dark:text-dark-fg-muted">Execution Details</h2>
				<div class="grid grid-cols-2 gap-4 sm:grid-cols-3">
					{#if run.harness}
						<div>
							<div class="mb-1 font-label-caps text-label-caps uppercase text-fg-muted dark:text-dark-fg-muted">Harness</div>
							<span class="font-mono-code text-mono-code text-fg-ink dark:text-dark-fg-ink">{run.harness}</span>
						</div>
					{/if}
					{#if run.provider}
						<div>
							<div class="mb-1 font-label-caps text-label-caps uppercase text-fg-muted dark:text-dark-fg-muted">Provider</div>
							<span class="font-mono-code text-mono-code text-fg-ink dark:text-dark-fg-ink">{run.provider}</span>
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
					{#if run.powerMin != null || run.powerMax != null}
						<div>
							<div class="mb-1 font-label-caps text-label-caps uppercase text-fg-muted dark:text-dark-fg-muted">Power bounds</div>
							<span class="font-mono-code text-mono-code text-fg-ink dark:text-dark-fg-ink">{run.powerMin ?? '?'}–{run.powerMax ?? '?'}</span>
						</div>
					{/if}
				</div>
				{#if run.promptSummary}
					<div>
						<div class="mb-1 font-label-caps text-label-caps uppercase text-fg-muted dark:text-dark-fg-muted">Prompt summary</div>
						<p class="text-body-sm text-fg-ink dark:text-dark-fg-ink">{run.promptSummary}</p>
					</div>
				{/if}
				{#if run.outputExcerpt}
					<div>
						<div class="mb-1 font-label-caps text-label-caps uppercase text-fg-muted dark:text-dark-fg-muted">Output excerpt</div>
						<p class="text-body-sm text-fg-ink dark:text-dark-fg-ink">{run.outputExcerpt}</p>
					</div>
				{/if}
				{#if run.evidenceLinks && run.evidenceLinks.length > 0}
					<div>
						<div class="mb-1 font-label-caps text-label-caps uppercase text-fg-muted dark:text-dark-fg-muted">Evidence links</div>
						<ul class="space-y-1">
							{#each run.evidenceLinks as link}
								<li class="font-mono-code text-mono-code text-fg-muted dark:text-dark-fg-muted">{link}</li>
							{/each}
						</ul>
					</div>
				{/if}
			</section>
		{/if}

		<!-- Child runs -->
		{#if run.childRunIds && run.childRunIds.length > 0}
			<section class="space-y-2 border border-border-line bg-bg-surface p-4 dark:border-dark-border-line dark:bg-dark-bg-surface">
				<h2 class="font-label-caps text-label-caps uppercase text-fg-muted dark:text-dark-fg-muted">
					{run.layer === 'work' ? 'Try Attempts' : 'Run Invocations'}
				</h2>
				<ul class="space-y-1">
					{#each run.childRunIds as childId}
						<li>
							<a href={childRunHref(childId)} class="font-mono-code text-mono-code text-accent-lever hover:underline dark:text-dark-accent-lever">
								{childId}
							</a>
						</li>
					{/each}
				</ul>
			</section>
		{/if}
	{/if}
</div>
