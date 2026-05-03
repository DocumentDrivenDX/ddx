<script lang="ts">
	import { createClient } from '$lib/gql/client'
	import { RUN_BUNDLE_FILE_QUERY } from './queries'
	import type { BundleFile, BundleFileContent } from './types'

	interface Props {
		runId: string
		files: BundleFile[]
	}
	let { runId, files }: Props = $props()

	let viewing = $state<string | null>(null)
	let viewedContent = $state<BundleFileContent | null>(null)
	let viewLoading = $state(false)
	let viewError = $state<string | null>(null)

	function downloadHref(path: string): string {
		return `/api/runs/${encodeURIComponent(runId)}/bundle?path=${encodeURIComponent(path)}`
	}

	function formatBytes(n: number): string {
		if (n < 1024) return `${n} B`
		if (n < 1024 * 1024) return `${(n / 1024).toFixed(1)} KB`
		return `${(n / (1024 * 1024)).toFixed(1)} MB`
	}

	async function viewFile(path: string) {
		if (viewing === path) {
			viewing = null
			viewedContent = null
			viewError = null
			return
		}
		viewing = path
		viewedContent = null
		viewError = null
		viewLoading = true
		try {
			const client = createClient()
			const data = await client.request<{ runBundleFile: BundleFileContent | null }>(
				RUN_BUNDLE_FILE_QUERY,
				{ id: runId, path }
			)
			if (!data.runBundleFile) {
				viewError = 'File not found'
			} else {
				viewedContent = data.runBundleFile
			}
		} catch (err) {
			viewError = err instanceof Error ? err.message : String(err)
		} finally {
			viewLoading = false
		}
	}
</script>

<div class="space-y-2" data-testid="rundetail-evidence">
	{#if files.length === 0}
		<div
			class="border-border-line bg-bg-surface text-body-sm text-fg-muted dark:border-dark-border-line dark:bg-dark-bg-surface dark:text-dark-fg-muted border p-3"
		>
			No bundle files.
		</div>
	{:else}
		<table class="w-full text-left text-sm" data-testid="evidence-files">
			<thead>
				<tr
					class="text-label-caps font-label-caps text-fg-muted dark:text-dark-fg-muted border-border-line dark:border-dark-border-line border-b uppercase"
				>
					<th class="px-2 py-1">Path</th>
					<th class="px-2 py-1">Size</th>
					<th class="px-2 py-1">Type</th>
					<th class="px-2 py-1">Actions</th>
				</tr>
			</thead>
			<tbody>
				{#each files as f (f.path)}
					<tr
						class="border-border-line dark:border-dark-border-line border-b align-top"
						data-evidence-path={f.path}
					>
						<td class="font-mono-code text-mono-code px-2 py-1">{f.path}</td>
						<td class="px-2 py-1">{formatBytes(f.size)}</td>
						<td class="font-mono-code text-mono-code px-2 py-1">{f.mimeType}</td>
						<td class="px-2 py-1">
							<div class="flex gap-2">
								<button
									type="button"
									data-action="view"
									data-evidence-view={f.path}
									class="border-border-line text-body-sm text-fg-ink hover:bg-bg-surface dark:border-dark-border-line dark:text-dark-fg-ink dark:hover:bg-dark-bg-surface border px-2 py-0.5"
									onclick={(e) => {
										e.stopPropagation()
										void viewFile(f.path)
									}}
								>
									{viewing === f.path ? 'Hide' : 'View'}
								</button>
								<a
									href={downloadHref(f.path)}
									download={f.path.split('/').pop()}
									data-action="download"
									data-evidence-download={f.path}
									class="border-border-line text-body-sm text-accent-lever hover:bg-bg-surface dark:border-dark-border-line dark:text-dark-accent-lever dark:hover:bg-dark-bg-surface border px-2 py-0.5"
								>
									Download
								</a>
							</div>
							{#if viewing === f.path}
								<div class="mt-2" data-testid="evidence-inline">
									{#if viewLoading}
										<div class="text-body-sm text-fg-muted dark:text-dark-fg-muted">Loading…</div>
									{:else if viewError}
										<div class="text-body-sm text-fg-muted dark:text-dark-fg-muted">
											{viewError}
										</div>
									{:else if viewedContent}
										{#if viewedContent.truncated || viewedContent.content == null}
											<div
												data-testid="evidence-inline-truncated"
												class="text-body-sm text-fg-muted dark:text-dark-fg-muted"
											>
												Inline view unavailable (file too large or not whitelisted). Use Download.
											</div>
										{:else}
											<pre
												data-testid="evidence-inline-content"
												class="bg-terminal-bg font-mono-code text-mono-code text-terminal-fg max-h-96 overflow-auto whitespace-pre-wrap px-3 py-2">{viewedContent.content}</pre>
										{/if}
									{/if}
								</div>
							{/if}
						</td>
					</tr>
				{/each}
			</tbody>
		</table>
	{/if}
</div>
