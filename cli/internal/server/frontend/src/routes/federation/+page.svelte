<script lang="ts">
	import type { PageData } from './$types'
	import { federationBadgeClass, isVersionSkew } from '$lib/federationStatus'

	let { data }: { data: PageData } = $props()

	function fmtDate(iso: string | null): string {
		if (!iso) return '—'
		return new Date(iso).toLocaleString()
	}
</script>

<div class="space-y-4" data-testid="federation-page">
	<div class="flex items-center justify-between">
		<h1 class="text-xl font-semibold text-fg-ink dark:text-dark-fg-ink">Federation</h1>
		<span class="text-sm text-fg-muted dark:text-dark-fg-muted">
			{data.nodes.length} node{data.nodes.length === 1 ? '' : 's'}
		</span>
	</div>

	{#if data.error}
		<div
			class="border border-accent-load/40 bg-accent-load/10 px-4 py-2 font-label-caps text-label-caps text-accent-load dark:border-dark-accent-load/40 dark:bg-dark-accent-load/10 dark:text-dark-accent-load"
			data-testid="federation-error"
		>
			Federation query failed: {data.error}
		</div>
	{/if}

	<div class="overflow-hidden border border-border-line dark:border-dark-border-line">
		<table class="w-full text-sm">
			<thead>
				<tr
					class="border-b border-border-line bg-bg-surface dark:border-dark-border-line dark:bg-dark-bg-surface"
				>
					<th
						class="px-4 py-3 text-left font-label-caps text-label-caps uppercase tracking-wide text-fg-muted dark:text-dark-fg-muted"
					>Node</th>
					<th
						class="px-4 py-3 text-left font-label-caps text-label-caps uppercase tracking-wide text-fg-muted dark:text-dark-fg-muted"
					>Status</th>
					<th
						class="px-4 py-3 text-left font-label-caps text-label-caps uppercase tracking-wide text-fg-muted dark:text-dark-fg-muted"
					>ddx / schema</th>
					<th
						class="px-4 py-3 text-left font-label-caps text-label-caps uppercase tracking-wide text-fg-muted dark:text-dark-fg-muted"
					>Last heartbeat</th>
					<th
						class="px-4 py-3 text-left font-label-caps text-label-caps uppercase tracking-wide text-fg-muted dark:text-dark-fg-muted"
					>Spoke</th>
				</tr>
			</thead>
			<tbody>
				{#each data.nodes as node (node.id)}
					{@const skew = isVersionSkew(node.lastError)}
					<tr
						data-testid="federation-row"
						data-status={node.status}
						class="border-b border-border-line last:border-0 dark:border-dark-border-line"
						class:opacity-60={node.status === 'offline'}
					>
						<td class="px-4 py-3">
							<div class="font-medium text-fg-ink dark:text-dark-fg-ink">{node.name}</div>
							<div class="font-mono-code text-xs text-fg-muted dark:text-dark-fg-muted">
								{node.nodeId}
							</div>
						</td>
						<td class="px-4 py-3">
							<span
								data-testid="federation-status-badge"
								class="inline-block border px-1.5 py-0.5 font-mono-code text-mono-code uppercase {federationBadgeClass(
									node.status
								)}"
							>
								{node.status}
							</span>
							{#if skew}
								<span
									data-testid="federation-skew-badge"
									class="ml-2 inline-block border px-1.5 py-0.5 font-mono-code text-mono-code uppercase badge-status-blocked"
									title={node.lastError ?? ''}
								>
									version-skew
								</span>
							{/if}
							{#if node.lastError && !skew}
								<div class="mt-1 text-xs text-fg-muted dark:text-dark-fg-muted" title={node.lastError}>
									{node.lastError}
								</div>
							{/if}
						</td>
						<td class="px-4 py-3 font-mono-code text-xs text-fg-muted dark:text-dark-fg-muted">
							{node.ddxVersion} / {node.schemaVersion}
						</td>
						<td class="px-4 py-3 font-mono-code text-xs text-fg-muted dark:text-dark-fg-muted">
							{fmtDate(node.lastHeartbeat)}
						</td>
						<td class="px-4 py-3">
							<a
								data-testid="federation-spoke-link"
								href={node.url}
								target="_blank"
								rel="noopener noreferrer"
								class="font-mono-code text-xs text-accent-lever hover:underline dark:text-dark-accent-lever"
							>
								{node.url}
							</a>
						</td>
					</tr>
				{/each}
				{#if data.nodes.length === 0}
					<tr>
						<td
							colspan="5"
							class="px-4 py-8 text-center text-body-sm text-fg-muted dark:text-dark-fg-muted"
						>
							No federation nodes registered. The server is either not running in hub mode or no
							spokes have registered yet.
						</td>
					</tr>
				{/if}
			</tbody>
		</table>
	</div>
</div>
