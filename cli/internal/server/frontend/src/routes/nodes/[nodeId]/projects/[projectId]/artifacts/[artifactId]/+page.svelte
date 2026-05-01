<script lang="ts">
	import type { PageData } from './$types'
	import { goto } from '$app/navigation'
	import { ArrowLeft } from 'lucide-svelte'

	let { data }: { data: PageData } = $props()

	const listHref = `/nodes/${data.nodeId}/projects/${data.projectId}/artifacts`

	function stalenessBadge(staleness: string): { label: string; cls: string } {
		switch (staleness) {
			case 'fresh':
				return {
					label: 'fresh',
					cls: 'bg-green-100 text-green-800 dark:bg-green-900/30 dark:text-green-400'
				}
			case 'stale':
				return {
					label: 'stale',
					cls: 'bg-amber-100 text-amber-800 dark:bg-amber-900/30 dark:text-amber-400'
				}
			case 'missing':
				return {
					label: 'missing',
					cls: 'bg-red-100 text-red-800 dark:bg-red-900/30 dark:text-red-400'
				}
			default:
				return {
					label: staleness,
					cls: 'bg-bg-surface text-fg-muted dark:bg-dark-bg-surface dark:text-dark-fg-muted'
				}
		}
	}
</script>

<div class="space-y-4">
	<a
		href={listHref}
		class="inline-flex items-center gap-1 text-body-sm text-fg-muted hover:text-fg-ink dark:text-dark-fg-muted dark:hover:text-dark-fg-ink"
	>
		<ArrowLeft class="h-4 w-4" />
		Back to Artifacts
	</a>

	{#if data.artifact}
		{@const badge = stalenessBadge(data.artifact.staleness)}
		<div class="space-y-2">
			<div class="flex items-start gap-3">
				<h1 class="text-headline-md font-headline-md text-fg-ink dark:text-dark-fg-ink">
					{data.artifact.title}
				</h1>
				<span
					class="mt-1 inline-block rounded-full px-2 py-0.5 font-label-caps text-label-caps uppercase {badge.cls}"
				>
					{badge.label}
				</span>
			</div>

			<dl class="space-y-2 text-body-sm">
				<div class="flex gap-4">
					<dt class="w-24 shrink-0 text-fg-muted dark:text-dark-fg-muted">Path</dt>
					<dd class="font-mono-code text-mono-code text-fg-ink dark:text-dark-fg-ink">
						{data.artifact.path}
					</dd>
				</div>
				<div class="flex gap-4">
					<dt class="w-24 shrink-0 text-fg-muted dark:text-dark-fg-muted">Media type</dt>
					<dd class="font-mono-code text-mono-code text-fg-ink dark:text-dark-fg-ink">
						{data.artifact.mediaType}
					</dd>
				</div>
				{#if data.artifact.description}
					<div class="flex gap-4">
						<dt class="w-24 shrink-0 text-fg-muted dark:text-dark-fg-muted">Description</dt>
						<dd class="text-fg-ink dark:text-dark-fg-ink">{data.artifact.description}</dd>
					</div>
				{/if}
				{#if data.artifact.updatedAt}
					<div class="flex gap-4">
						<dt class="w-24 shrink-0 text-fg-muted dark:text-dark-fg-muted">Updated</dt>
						<dd class="text-fg-ink dark:text-dark-fg-ink">
							{new Date(data.artifact.updatedAt).toLocaleString()}
						</dd>
					</div>
				{/if}
			</dl>
		</div>
	{:else}
		<p class="text-body-sm text-fg-muted dark:text-dark-fg-muted">Artifact not found.</p>
	{/if}
</div>
