<script lang="ts">
	import type { SessionDetail } from './types'
	import { fmtCost } from './format'

	interface Props {
		session: SessionDetail | null
		loading?: boolean
		sessionId?: string | null
	}
	let { session, loading = false, sessionId }: Props = $props()
</script>

<div class="space-y-3" data-testid="rundetail-session">
	{#if loading}
		<div class="text-body-sm text-fg-muted dark:text-dark-fg-muted">Loading session…</div>
	{:else if !session}
		<div class="alert-caution border p-3 text-body-sm">
			{#if sessionId}
				Session <code class="font-mono-code text-mono-code">{sessionId}</code> is not (yet) recorded in the session index.
			{:else}
				No session is associated with this run.
			{/if}
		</div>
	{:else}
		<dl class="grid grid-cols-2 gap-3 sm:grid-cols-4">
			<div>
				<dt class="text-label-caps font-label-caps uppercase tracking-wide text-fg-muted dark:text-dark-fg-muted">Session</dt>
				<dd class="mt-1 font-mono-code text-mono-code text-fg-ink dark:text-dark-fg-ink">{session.id}</dd>
			</div>
			<div>
				<dt class="text-label-caps font-label-caps uppercase tracking-wide text-fg-muted dark:text-dark-fg-muted">Harness</dt>
				<dd class="mt-1 text-body-sm text-fg-ink dark:text-dark-fg-ink">{session.harness}</dd>
			</div>
			<div>
				<dt class="text-label-caps font-label-caps uppercase tracking-wide text-fg-muted dark:text-dark-fg-muted">Model</dt>
				<dd class="mt-1 text-body-sm text-fg-ink dark:text-dark-fg-ink">{session.model}</dd>
			</div>
			<div>
				<dt class="text-label-caps font-label-caps uppercase tracking-wide text-fg-muted dark:text-dark-fg-muted">Status</dt>
				<dd class="mt-1 text-body-sm text-fg-ink dark:text-dark-fg-ink">{session.status}</dd>
			</div>
			<div>
				<dt class="text-label-caps font-label-caps uppercase tracking-wide text-fg-muted dark:text-dark-fg-muted">Cost</dt>
				<dd class="mt-1 font-mono-code text-mono-code text-fg-ink dark:text-dark-fg-ink">{fmtCost(session.cost)}</dd>
			</div>
			<div>
				<dt class="text-label-caps font-label-caps uppercase tracking-wide text-fg-muted dark:text-dark-fg-muted">Billing</dt>
				<dd class="mt-1 text-body-sm text-fg-ink dark:text-dark-fg-ink">{session.billingMode}</dd>
			</div>
			<div>
				<dt class="text-label-caps font-label-caps uppercase tracking-wide text-fg-muted dark:text-dark-fg-muted">Prompt tokens</dt>
				<dd class="mt-1 font-mono-code text-mono-code text-fg-ink dark:text-dark-fg-ink">{session.tokens?.prompt?.toLocaleString() ?? '—'}</dd>
			</div>
			<div>
				<dt class="text-label-caps font-label-caps uppercase tracking-wide text-fg-muted dark:text-dark-fg-muted">Completion tokens</dt>
				<dd class="mt-1 font-mono-code text-mono-code text-fg-ink dark:text-dark-fg-ink">{session.tokens?.completion?.toLocaleString() ?? '—'}</dd>
			</div>
			{#if session.outcome}
				<div class="col-span-2 sm:col-span-4">
					<dt class="text-label-caps font-label-caps uppercase tracking-wide text-fg-muted dark:text-dark-fg-muted">Outcome</dt>
					<dd class="mt-1 text-body-sm text-fg-ink dark:text-dark-fg-ink">{session.outcome}</dd>
				</div>
			{/if}
		</dl>
	{/if}
</div>
