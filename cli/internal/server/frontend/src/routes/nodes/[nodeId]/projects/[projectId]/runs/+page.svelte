<script lang="ts">
	import type { PageData } from './$types'
	import { page } from '$app/stores'
	import { goto } from '$app/navigation'
	import { createClient } from '$lib/gql/client'
	import { PROJECT_RUNS_QUERY, PAGE_SIZE } from './+page'
	import type { RunEdge, RunNode, RunConnection, PageInfo } from './+page'
	import { RunRowDetail } from '$lib/runDetail'
	import ConfirmDialog from '$lib/components/ConfirmDialog.svelte'
	import { RUN_REQUEUE_MUTATION, WORKER_DISPATCH_MUTATION } from '$lib/gql/feat008'

	let { data }: { data: PageData } = $props()

	let appendedEdges = $state<RunEdge[]>([])
	let appendedPageInfo = $state<PageInfo | null>(null)
	let loadingMore = $state(false)
	let harnessInput = $state(data.activeHarness ?? '')
	let expanded = $state<Set<string>>(new Set())

	type ActionMode = 'requeue' | 'startWorker'
	let actionDialogOpen = $state(false)
	let actionMode = $state<ActionMode>('requeue')
	let actionRun = $state<RunNode | null>(null)
	let actionIdempotencyKey = $state('')
	let actionLayerOverride = $state('')
	let actionWorkerArgs = $state('')
	let actionAlert = $state('')
	let actionResultMessage = $state('')

	function newIdempotencyKey(): string {
		if (typeof crypto !== 'undefined' && typeof crypto.randomUUID === 'function') {
			return crypto.randomUUID()
		}
		return `requeue-${Date.now()}-${Math.random().toString(36).slice(2, 10)}`
	}

	function openRequeueDialog(run: RunNode) {
		actionMode = 'requeue'
		actionRun = run
		actionIdempotencyKey = newIdempotencyKey()
		actionLayerOverride = run.layer
		actionAlert = ''
		actionResultMessage = ''
		actionDialogOpen = true
	}

	function openStartWorkerDialog(run: RunNode) {
		actionMode = 'startWorker'
		actionRun = run
		actionAlert = ''
		actionResultMessage = ''
		// Prefill the worker args with the original drain's queueInputs JSON.
		// Pretty-print so the operator can review/edit before dispatching.
		if (run.queueInputs) {
			try {
				actionWorkerArgs = JSON.stringify(JSON.parse(run.queueInputs), null, 2)
			} catch {
				actionWorkerArgs = run.queueInputs
			}
		} else {
			actionWorkerArgs = JSON.stringify(
				{ source: 'runs-start-worker', sourceRunId: run.id },
				null,
				2
			)
		}
		actionDialogOpen = true
	}

	async function confirmAction() {
		if (!actionRun) return
		const client = createClient()
		try {
			if (actionMode === 'requeue') {
				if (!actionIdempotencyKey.trim()) {
					actionAlert = 'Idempotency key is required.'
					throw new Error('missing idempotency key')
				}
				const layer = actionLayerOverride.trim()
				const result = await client.request<{
					runRequeue: { bead: { id: string }; deduplicated: boolean }
				}>(RUN_REQUEUE_MUTATION, {
					input: {
						runId: actionRun.id,
						idempotencyKey: actionIdempotencyKey,
						layer: layer ? layer : null
					}
				})
				actionResultMessage = result.runRequeue.deduplicated
					? `Bead ${result.runRequeue.bead.id} already requeued (deduplicated).`
					: `Bead ${result.runRequeue.bead.id} requeued.`
			} else {
				// startWorker: validate JSON before sending so the operator can fix it inline.
				let argsString = actionWorkerArgs.trim()
				if (argsString) {
					try {
						JSON.parse(argsString)
					} catch (err) {
						actionAlert = `Worker args must be valid JSON: ${errorText(err)}`
						throw err
					}
				} else {
					argsString = '{}'
				}
				const projectId = actionRun.projectID ?? data.projectId
				const result = await client.request<{
					workerDispatch: { id: string; state: string; kind: string }
				}>(WORKER_DISPATCH_MUTATION, {
					kind: 'execute-loop',
					projectId,
					args: argsString
				})
				actionResultMessage = `Worker ${result.workerDispatch.id} started (${result.workerDispatch.state}).`
			}
		} catch (err) {
			if (!actionAlert) actionAlert = errorText(err)
			throw err
		}
	}

	function errorText(err: unknown): string {
		if (err instanceof Error) return err.message
		if (typeof err === 'string') return err
		return 'Unknown error.'
	}

	function toggleExpand(runId: string) {
		const next = new Set(expanded)
		if (next.has(runId)) next.delete(runId)
		else next.add(runId)
		expanded = next
	}

	let filterKey = $derived(`${data.activeLayer}::${data.activeStatus}::${data.activeHarness}`)
	let prevFilterKey = $state('')
	$effect(() => {
		if (filterKey !== prevFilterKey) {
			prevFilterKey = filterKey
			appendedEdges = []
			appendedPageInfo = null
		}
	})

	let edges = $derived([...data.runs.edges, ...appendedEdges])
	let pageInfo = $derived<PageInfo>(appendedPageInfo ?? data.runs.pageInfo)

	const LAYER_OPTIONS = ['work', 'try', 'run']
	const STATUS_OPTIONS = ['pending', 'running', 'success', 'failure', 'preserved']

	function setFilter(key: 'layer' | 'status' | 'harness', value: string | null) {
		const params = new URLSearchParams($page.url.searchParams)
		if (value === null || value === '') {
			params.delete(key)
		} else {
			params.set(key, value)
		}
		const search = params.toString()
		goto(search ? `?${search}` : $page.url.pathname, { replaceState: false })
	}

	function toggleLayer(layer: string) {
		setFilter('layer', data.activeLayer === layer ? null : layer)
	}

	function toggleStatus(status: string) {
		setFilter('status', data.activeStatus === status ? null : status)
	}

	function applyHarness() {
		setFilter('harness', harnessInput.trim() || null)
	}

	async function loadMore() {
		if (!pageInfo.hasNextPage || loadingMore) return
		loadingMore = true
		try {
			const client = createClient()
			const result = await client.request<{ runs: RunConnection }>(PROJECT_RUNS_QUERY, {
				projectID: data.projectId,
				first: PAGE_SIZE,
				after: pageInfo.endCursor,
				layer: data.activeLayer ?? undefined,
				status: data.activeStatus ?? undefined,
				harness: data.activeHarness ?? undefined
			})
			appendedEdges = [...appendedEdges, ...result.runs.edges]
			appendedPageInfo = result.runs.pageInfo
		} finally {
			loadingMore = false
		}
	}

	function runDetailHref(runId: string): string {
		return `/nodes/${data.nodeId}/projects/${data.projectId}/runs/${runId}`
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
				return 'badge-layer-work'
			case 'try':
				return 'badge-layer-try'
			case 'run':
				return 'badge-layer-run'
			default:
				return 'badge-status-neutral'
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

	function chipClass(active: boolean): string {
		return active
			? 'rounded-sm border px-3 py-1 text-xs font-medium border-accent-lever bg-accent-lever/10 text-accent-lever dark:border-dark-accent-lever dark:bg-dark-accent-lever/20 dark:text-dark-accent-lever'
			: 'rounded-sm border px-3 py-1 text-xs font-medium border-border-line bg-bg-elevated text-fg-ink hover:border-fg-muted hover:bg-bg-surface dark:border-dark-border-line dark:bg-dark-bg-elevated dark:text-dark-fg-ink dark:hover:bg-dark-bg-surface'
	}
</script>

<div class="space-y-4">
	<div class="flex items-center justify-between">
		<h1 class="text-headline-lg font-headline-lg text-fg-ink dark:text-dark-fg-ink">Runs</h1>
		<span class="text-body-sm text-fg-muted dark:text-dark-fg-muted">
			{edges.length} of {data.runs.totalCount}
		</span>
	</div>

	<!-- Layer filter chips -->
	<div class="flex flex-wrap gap-2">
		<span class="self-center text-xs text-fg-muted dark:text-dark-fg-muted">Layer:</span>
		{#each LAYER_OPTIONS as layer}
			<button class={chipClass(data.activeLayer === layer)} onclick={() => toggleLayer(layer)}>
				{layer}
			</button>
		{/each}
		{#if data.activeLayer}
			<button
				class="rounded-sm border border-border-line bg-bg-elevated px-3 py-1 text-xs text-fg-ink hover:bg-bg-surface dark:border-dark-border-line dark:bg-dark-bg-elevated dark:text-dark-fg-ink dark:hover:bg-dark-bg-surface"
				onclick={() => setFilter('layer', null)}
			>
				clear
			</button>
		{/if}
	</div>

	<!-- Status filter chips -->
	<div class="flex flex-wrap gap-2">
		<span class="self-center text-xs text-fg-muted dark:text-dark-fg-muted">Status:</span>
		{#each STATUS_OPTIONS as status}
			<button class={chipClass(data.activeStatus === status)} onclick={() => toggleStatus(status)}>
				{status}
			</button>
		{/each}
		{#if data.activeStatus}
			<button
				class="rounded-sm border border-border-line bg-bg-elevated px-3 py-1 text-xs text-fg-ink hover:bg-bg-surface dark:border-dark-border-line dark:bg-dark-bg-elevated dark:text-dark-fg-ink dark:hover:bg-dark-bg-surface"
				onclick={() => setFilter('status', null)}
			>
				clear
			</button>
		{/if}
	</div>

	<!-- Harness filter -->
	<form
		class="flex items-center gap-2"
		onsubmit={(e) => {
			e.preventDefault()
			applyHarness()
		}}
	>
		<span class="text-xs text-fg-muted dark:text-dark-fg-muted">Harness:</span>
		<input
			type="text"
			bind:value={harnessInput}
			placeholder="claude / codex / agent"
			class="w-44 border border-border-line bg-bg-elevated px-2 py-1 font-mono-code text-mono-code text-fg-ink dark:border-dark-border-line dark:bg-dark-bg-elevated dark:text-dark-fg-ink"
		/>
		<button
			type="submit"
			class="border border-border-line bg-bg-elevated px-2 py-1 text-xs text-fg-muted hover:bg-bg-surface dark:border-dark-border-line dark:bg-dark-bg-elevated dark:text-dark-fg-muted dark:hover:bg-dark-bg-surface"
		>
			Apply
		</button>
		{#if data.activeHarness}
			<button
				type="button"
				onclick={() => {
					harnessInput = ''
					setFilter('harness', null)
				}}
				class="border border-border-line bg-bg-elevated px-2 py-1 text-xs text-fg-muted hover:bg-bg-surface dark:border-dark-border-line dark:bg-dark-bg-elevated dark:text-dark-fg-muted dark:hover:bg-dark-bg-surface"
			>
				Clear
			</button>
		{/if}
	</form>

	<div class="overflow-hidden border border-border-line dark:border-dark-border-line">
		<table class="w-full text-sm">
			<thead>
				<tr class="border-b border-border-line bg-bg-surface dark:border-dark-border-line dark:bg-dark-bg-surface">
					<th class="w-6 px-4 py-3"></th>
					<th class="px-4 py-3 text-left font-label-caps text-label-caps uppercase tracking-wide text-fg-muted dark:text-dark-fg-muted">Layer</th>
					<th class="px-4 py-3 text-left font-label-caps text-label-caps uppercase tracking-wide text-fg-muted dark:text-dark-fg-muted">Status</th>
					<th class="px-4 py-3 text-left font-label-caps text-label-caps uppercase tracking-wide text-fg-muted dark:text-dark-fg-muted">Bead</th>
					<th class="px-4 py-3 text-left font-label-caps text-label-caps uppercase tracking-wide text-fg-muted dark:text-dark-fg-muted">Harness</th>
					<th class="px-4 py-3 text-left font-label-caps text-label-caps uppercase tracking-wide text-fg-muted dark:text-dark-fg-muted">Started</th>
					<th class="px-4 py-3 text-right font-label-caps text-label-caps uppercase tracking-wide text-fg-muted dark:text-dark-fg-muted">Duration</th>
					<th class="px-4 py-3 text-right font-label-caps text-label-caps uppercase tracking-wide text-fg-muted dark:text-dark-fg-muted">Actions</th>
				</tr>
			</thead>
			<tbody>
				{#each edges as edge (edge.cursor)}
					{@const isExpanded = expanded.has(edge.node.id)}
					<tr
						class="cursor-pointer border-b border-border-line last:border-0 hover:bg-bg-surface dark:border-dark-border-line dark:hover:bg-dark-bg-surface {isExpanded
							? 'bg-accent-lever/10 dark:bg-dark-accent-lever/10'
							: ''}"
						data-run-row={edge.node.id}
						onclick={() => toggleExpand(edge.node.id)}
					>
						<td class="px-4 py-3 text-body-sm text-fg-muted dark:text-dark-fg-muted">
							{isExpanded ? '▾' : '▸'}
						</td>
						<td class="px-4 py-3">
							<span class="inline-block rounded-full px-2 py-0.5 font-label-caps text-label-caps uppercase {layerBadgeClass(edge.node.layer)}">
								{edge.node.layer}
							</span>
						</td>
						<td class="px-4 py-3">
							<span class="inline-block border px-1.5 py-0.5 font-mono-code text-mono-code uppercase {statusBadgeClass(edge.node.status)}">
								{edge.node.status}
							</span>
						</td>
						<td class="px-4 py-3">
							{#if edge.node.beadId}
								<a
									href={beadHref(edge.node.beadId)}
									onclick={(e) => e.stopPropagation()}
									class="font-mono-code text-mono-code text-accent-lever hover:underline dark:text-dark-accent-lever"
								>
									{edge.node.beadId}
								</a>
							{:else}
								<span class="text-fg-muted dark:text-dark-fg-muted">—</span>
							{/if}
						</td>
						<td class="px-4 py-3 font-mono-code text-mono-code text-fg-muted dark:text-dark-fg-muted">
							{edge.node.harness ?? '—'}
						</td>
						<td class="px-4 py-3 font-mono-code text-mono-code text-fg-muted dark:text-dark-fg-muted">
							{fmtDate(edge.node.startedAt)}
						</td>
						<td class="px-4 py-3 text-right font-mono-code text-mono-code text-fg-muted dark:text-dark-fg-muted">
							<div class="flex items-center justify-end gap-2">
								<span>{fmtDuration(edge.node.durationMs)}</span>
								<a
									href={runDetailHref(edge.node.id)}
									onclick={(e) => e.stopPropagation()}
									title="Open detail page"
									class="font-mono-code text-mono-code text-accent-lever hover:underline dark:text-dark-accent-lever"
								>
									↗
								</a>
							</div>
						</td>
						<td class="px-4 py-3 text-right">
							{#if edge.node.layer === 'work'}
								<button
									type="button"
									data-run-action="start-worker"
									data-run-id={edge.node.id}
									onclick={(e) => {
										e.stopPropagation()
										openStartWorkerDialog(edge.node)
									}}
									class="rounded-sm border border-border-line bg-bg-elevated px-2 py-1 text-xs font-medium text-fg-ink hover:bg-bg-surface dark:border-dark-border-line dark:bg-dark-bg-elevated dark:text-dark-fg-ink dark:hover:bg-dark-bg-surface"
								>
									Start worker
								</button>
							{:else}
								<button
									type="button"
									data-run-action="requeue"
									data-run-id={edge.node.id}
									onclick={(e) => {
										e.stopPropagation()
										openRequeueDialog(edge.node)
									}}
									class="rounded-sm border border-border-line bg-bg-elevated px-2 py-1 text-xs font-medium text-fg-ink hover:bg-bg-surface dark:border-dark-border-line dark:bg-dark-bg-elevated dark:text-dark-fg-ink dark:hover:bg-dark-bg-surface"
								>
									Re-queue
								</button>
							{/if}
						</td>
					</tr>
					{#if isExpanded}
						<tr class="border-b border-border-line bg-bg-surface dark:border-dark-border-line dark:bg-dark-bg-surface">
							<td colspan="8" class="px-6 py-4">
								<RunRowDetail
									runId={edge.node.id}
									layer={edge.node.layer}
									nodeId={data.nodeId}
									projectId={data.projectId}
								/>
							</td>
						</tr>
					{/if}
				{/each}
				{#if edges.length === 0}
					<tr>
						<td colspan="8" class="px-4 py-8 text-center text-body-sm text-fg-muted dark:text-dark-fg-muted">
							No runs found.
						</td>
					</tr>
				{/if}
			</tbody>
		</table>
	</div>

	{#if actionResultMessage}
		<div
			role="status"
			class="rounded-md border border-accent-fulcrum/30 bg-accent-fulcrum/10 px-4 py-2 text-sm text-fg-ink dark:border-dark-accent-fulcrum/30 dark:bg-dark-accent-fulcrum/10 dark:text-dark-fg-ink"
			data-run-action-result
		>
			{actionResultMessage}
		</div>
	{/if}

	{#if pageInfo.hasNextPage}
		<div class="flex justify-center">
			<button
				onclick={loadMore}
				disabled={loadingMore}
				class="rounded border border-border-line bg-bg-surface px-4 py-2 text-body-sm text-fg-muted hover:bg-bg-elevated hover:text-fg-ink disabled:opacity-50 dark:border-dark-border-line dark:bg-dark-bg-surface dark:text-dark-fg-muted dark:hover:bg-dark-bg-elevated dark:hover:text-dark-fg-ink"
			>
				{loadingMore ? 'Loading…' : 'Load more'}
			</button>
		</div>
	{/if}
</div>

<ConfirmDialog
	bind:open={actionDialogOpen}
	title={actionMode === 'requeue' ? 'Re-queue run' : 'Start worker from this drain'}
	actionLabel={actionMode === 'requeue' ? 'Re-queue' : 'Start worker'}
	onConfirm={confirmAction}
>
	{#snippet summary()}
		<span>
			{#if actionRun}
				{actionMode === 'requeue'
					? `Re-queue ${actionRun.layer} run ${actionRun.id}`
					: `Start an execute-loop worker prefilled from work run ${actionRun.id}`}
			{/if}
		</span>
	{/snippet}

	{#if actionRun}
		<div class="space-y-3">
			{#if actionAlert}
				<div
					role="alert"
					class="rounded border border-error/30 bg-error/10 px-3 py-2 text-xs text-error dark:border-dark-error/30 dark:bg-dark-error/10 dark:text-dark-error"
					data-run-action-alert
				>
					{actionAlert}
				</div>
			{/if}

			{#if actionMode === 'requeue'}
				<div>
					<label class="mb-1 block font-label-caps text-label-caps uppercase text-fg-muted dark:text-dark-fg-muted" for="requeue-run-id">
						Run ID
					</label>
					<input
						id="requeue-run-id"
						type="text"
						readonly
						value={actionRun.id}
						class="w-full border border-border-line bg-bg-surface px-2 py-1 font-mono-code text-mono-code text-fg-ink dark:border-dark-border-line dark:bg-dark-bg-surface dark:text-dark-fg-ink"
					/>
				</div>
				<div>
					<label class="mb-1 block font-label-caps text-label-caps uppercase text-fg-muted dark:text-dark-fg-muted" for="requeue-layer">
						Layer override
					</label>
					<select
						id="requeue-layer"
						bind:value={actionLayerOverride}
						class="w-full border border-border-line bg-bg-elevated px-2 py-1 font-mono-code text-mono-code text-fg-ink dark:border-dark-border-line dark:bg-dark-bg-elevated dark:text-dark-fg-ink"
					>
						<option value="">(none)</option>
						<option value="work">work</option>
						<option value="try">try</option>
						<option value="run">run</option>
					</select>
				</div>
				<div>
					<label class="mb-1 block font-label-caps text-label-caps uppercase text-fg-muted dark:text-dark-fg-muted" for="requeue-key">
						Idempotency key (client-generated)
					</label>
					<div class="flex gap-2">
						<input
							id="requeue-key"
							type="text"
							bind:value={actionIdempotencyKey}
							data-idempotency-key
							class="w-full border border-border-line bg-bg-elevated px-2 py-1 font-mono-code text-mono-code text-fg-ink dark:border-dark-border-line dark:bg-dark-bg-elevated dark:text-dark-fg-ink"
						/>
						<button
							type="button"
							onclick={() => (actionIdempotencyKey = newIdempotencyKey())}
							class="rounded-sm border border-border-line bg-bg-elevated px-2 py-1 text-xs text-fg-muted hover:bg-bg-surface dark:border-dark-border-line dark:bg-dark-bg-elevated dark:text-dark-fg-muted dark:hover:bg-dark-bg-surface"
						>
							Regenerate
						</button>
					</div>
				</div>
			{:else}
				<div>
					<label class="mb-1 block font-label-caps text-label-caps uppercase text-fg-muted dark:text-dark-fg-muted" for="start-worker-source">
						Source work run
					</label>
					<input
						id="start-worker-source"
						type="text"
						readonly
						value={actionRun.id}
						class="w-full border border-border-line bg-bg-surface px-2 py-1 font-mono-code text-mono-code text-fg-ink dark:border-dark-border-line dark:bg-dark-bg-surface dark:text-dark-fg-ink"
					/>
				</div>
				<div>
					<label class="mb-1 block font-label-caps text-label-caps uppercase text-fg-muted dark:text-dark-fg-muted" for="start-worker-args">
						Worker args (JSON, prefilled from original drain)
					</label>
					<textarea
						id="start-worker-args"
						bind:value={actionWorkerArgs}
						rows="8"
						data-worker-args
						class="w-full border border-border-line bg-bg-elevated px-2 py-1 font-mono-code text-mono-code text-fg-ink dark:border-dark-border-line dark:bg-dark-bg-elevated dark:text-dark-fg-ink"
					></textarea>
				</div>
			{/if}
		</div>
	{/if}
</ConfirmDialog>
