<script lang="ts">
	import type { PageData } from './$types'
	import { ArrowLeft, Download, ExternalLink, Network } from 'lucide-svelte'
	import { marked } from 'marked'
	import DOMPurify from 'isomorphic-dompurify'
	import { onMount } from 'svelte'

	let { data }: { data: PageData } = $props()

	const listHref =
		data.back ?? `/nodes/${data.nodeId}/projects/${data.projectId}/artifacts`

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

	// Determine renderer type from mediaType
	function rendererType(mt: string): 'markdown' | 'svg' | 'excalidraw' | 'image' | 'pdf' | 'binary' {
		if (mt === 'text/markdown') return 'markdown'
		if (mt === 'image/svg+xml') return 'svg'
		if (mt === 'application/vnd.excalidraw+json') return 'excalidraw'
		if (mt.startsWith('image/')) return 'image'
		if (mt === 'application/pdf') return 'pdf'
		return 'binary'
	}

	// Rendered markdown HTML
	const renderedMarkdown = $derived.by(() => {
		if (!data.artifact?.content || rendererType(data.artifact.mediaType) !== 'markdown') return ''
		const raw = marked.parse(data.artifact.content, { async: false }) as string
		return DOMPurify.sanitize(raw)
	})

	// Sanitized SVG content
	const sanitizedSvg = $derived.by(() => {
		if (!data.artifact?.content || rendererType(data.artifact.mediaType) !== 'svg') return ''
		return DOMPurify.sanitize(data.artifact.content, {
			USE_PROFILES: { svg: true, svgFilters: true }
		})
	})

	// DDx frontmatter parsed as key/value pairs
	const frontmatterEntries = $derived.by((): [string, string][] => {
		if (!data.artifact?.ddxFrontmatter) return []
		try {
			const fm = JSON.parse(data.artifact.ddxFrontmatter)
			if (typeof fm !== 'object' || fm === null) return []
			return Object.entries(fm).map(([k, v]) => [
				k,
				typeof v === 'object' ? JSON.stringify(v) : String(v)
			])
		} catch {
			return []
		}
	})

	// Offline detection for Excalidraw
	let isOnline = $state(true)
	onMount(() => {
		isOnline = navigator.onLine
		const handleOnline = () => { isOnline = true }
		const handleOffline = () => { isOnline = false }
		window.addEventListener('online', handleOnline)
		window.addEventListener('offline', handleOffline)
		return () => {
			window.removeEventListener('online', handleOnline)
			window.removeEventListener('offline', handleOffline)
		}
	})

	// Process markdown links to handle intra-repo navigation vs external links
	function processMarkdownHtml(html: string): string {
		// External links open in new tab; relative (intra-repo) links navigate within UI
		return html.replace(/<a\s+href="([^"]+)"/g, (_match, href) => {
			const isExternal = /^https?:\/\//.test(href) || href.startsWith('//')
			if (isExternal) {
				return `<a href="${href}" target="_blank" rel="noopener noreferrer"`
			}
			return `<a href="${href}"`
		})
	}

	const finalMarkdownHtml = $derived(processMarkdownHtml(renderedMarkdown))
</script>

<div class="space-y-4">
	<div class="flex items-center justify-between">
		<a
			href={listHref}
			class="inline-flex items-center gap-1 text-body-sm text-fg-muted hover:text-fg-ink dark:text-dark-fg-muted dark:hover:text-dark-fg-ink"
		>
			<ArrowLeft class="h-4 w-4" />
			Back to Artifacts
		</a>
		{#if data.artifact?.mediaType === 'text/markdown'}
			<a
				href={`/nodes/${data.nodeId}/projects/${data.projectId}/graph`}
				class="inline-flex items-center gap-1 text-body-sm text-fg-muted hover:text-fg-ink dark:text-dark-fg-muted dark:hover:text-dark-fg-ink"
			>
				<Network class="h-4 w-4" />
				View in Graph
			</a>
		{/if}
	</div>

	{#if data.artifact}
		{@const badge = stalenessBadge(data.artifact.staleness)}
		{@const rtype = rendererType(data.artifact.mediaType)}

		<!-- Header -->
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

			<!-- Core metadata -->
			<dl class="space-y-1 text-body-sm">
				<div class="flex gap-4">
					<dt class="w-24 shrink-0 text-fg-muted dark:text-dark-fg-muted">Path</dt>
					<dd class="font-mono-code text-mono-code text-fg-ink dark:text-dark-fg-ink break-all">
						{data.artifact.path}
					</dd>
				</div>
				<div class="flex gap-4">
					<dt class="w-24 shrink-0 text-fg-muted dark:text-dark-fg-muted">Media type</dt>
					<dd class="font-mono-code text-mono-code text-fg-ink dark:text-dark-fg-ink">
						<span class="rounded bg-bg-surface px-1.5 py-0.5 text-xs dark:bg-dark-bg-surface">
							{data.artifact.mediaType}
						</span>
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

		<!-- Renderer -->
		<div class="border border-border-line dark:border-dark-border-line">
			{#if rtype === 'markdown'}
				<!-- Markdown renderer: rendered with syntax-highlighted code blocks -->
				<!-- eslint-disable-next-line svelte/no-at-html-tags -->
				<div class="prose prose-sm dark:prose-invert max-w-none p-4">{@html finalMarkdownHtml}</div>

			{:else if rtype === 'svg'}
				<!-- SVG renderer: DOMPurify-sanitized inline SVG -->
				<div class="flex items-center justify-center p-4">
					<!-- eslint-disable-next-line svelte/no-at-html-tags -->
					{@html sanitizedSvg}
				</div>
				<div class="border-t border-border-line p-2 dark:border-dark-border-line">
					{#if data.contentUrl}
						<a
							href={data.contentUrl}
							download={data.artifact.path.split('/').pop()}
							class="inline-flex items-center gap-1 text-body-sm text-fg-muted hover:text-fg-ink dark:text-dark-fg-muted dark:hover:text-dark-fg-ink"
						>
							<Download class="h-4 w-4" />
							Download SVG
						</a>
					{/if}
				</div>

			{:else if rtype === 'excalidraw'}
				<!-- Excalidraw renderer: hosted iframe embed with offline fallback -->
				{#if isOnline}
					<div class="relative h-[600px] w-full">
						<iframe
							src="https://excalidraw.com/embed"
							title="Excalidraw preview"
							class="h-full w-full border-0"
							sandbox="allow-scripts allow-same-origin allow-popups"
						></iframe>
					</div>
				{:else}
					<div class="flex flex-col items-center gap-3 p-8 text-center">
						<p class="text-body-sm text-fg-muted dark:text-dark-fg-muted">
							Excalidraw preview requires network connection.
						</p>
						{#if data.contentUrl}
							<a
								href={data.contentUrl}
								download={data.artifact.path.split('/').pop()}
								class="inline-flex items-center gap-1 text-body-sm text-accent-lever hover:underline dark:text-dark-accent-lever"
							>
								<Download class="h-4 w-4" />
								Download Excalidraw JSON
							</a>
						{/if}
					</div>
				{/if}
				{#if data.contentUrl}
					<div class="border-t border-border-line p-2 dark:border-dark-border-line">
						<a
							href={data.contentUrl}
							download={data.artifact.path.split('/').pop()}
							class="inline-flex items-center gap-1 text-body-sm text-fg-muted hover:text-fg-ink dark:text-dark-fg-muted dark:hover:text-dark-fg-ink"
						>
							<Download class="h-4 w-4" />
							Download JSON
						</a>
					</div>
				{/if}

			{:else if rtype === 'image'}
				<!-- Image renderer: inline preview -->
				{#if data.contentUrl}
					<div class="flex items-center justify-center p-4">
						<img
							src={data.contentUrl}
							alt={data.artifact.title}
							class="max-h-[600px] max-w-full object-contain"
						/>
					</div>
					<div class="border-t border-border-line p-2 dark:border-dark-border-line">
						<div class="flex gap-3">
							<a
								href={data.contentUrl}
								target="_blank"
								rel="noopener noreferrer"
								class="inline-flex items-center gap-1 text-body-sm text-fg-muted hover:text-fg-ink dark:text-dark-fg-muted dark:hover:text-dark-fg-ink"
							>
								<ExternalLink class="h-4 w-4" />
								Open
							</a>
							<a
								href={data.contentUrl}
								download={data.artifact.path.split('/').pop()}
								class="inline-flex items-center gap-1 text-body-sm text-fg-muted hover:text-fg-ink dark:text-dark-fg-muted dark:hover:text-dark-fg-ink"
							>
								<Download class="h-4 w-4" />
								Download
							</a>
						</div>
					</div>
				{/if}

			{:else if rtype === 'pdf'}
				<!-- PDF renderer: embedded viewer with fallback link -->
				{#if data.contentUrl}
					<div class="space-y-2 p-2">
						<embed
							src={data.contentUrl}
							type="application/pdf"
							class="h-[700px] w-full"
							title={data.artifact.title}
						/>
						<div class="flex items-center gap-3 border-t border-border-line pt-2 dark:border-dark-border-line">
							<a
								href={data.contentUrl}
								target="_blank"
								rel="noopener noreferrer"
								class="inline-flex items-center gap-1 text-body-sm text-fg-muted hover:text-fg-ink dark:text-dark-fg-muted dark:hover:text-dark-fg-ink"
							>
								<ExternalLink class="h-4 w-4" />
								Open PDF
							</a>
							<a
								href={data.contentUrl}
								download={data.artifact.path.split('/').pop()}
								class="inline-flex items-center gap-1 text-body-sm text-fg-muted hover:text-fg-ink dark:text-dark-fg-muted dark:hover:text-dark-fg-ink"
							>
								<Download class="h-4 w-4" />
								Download
							</a>
						</div>
					</div>
				{/if}

			{:else}
				<!-- Binary/unknown: metadata only with open/download affordance -->
				<div class="p-4 text-body-sm text-fg-muted dark:text-dark-fg-muted">
					<p>Binary content — no preview available.</p>
					{#if data.contentUrl}
						<div class="mt-3 flex gap-3">
							<a
								href={data.contentUrl}
								target="_blank"
								rel="noopener noreferrer"
								class="inline-flex items-center gap-1 text-fg-muted hover:text-fg-ink dark:text-dark-fg-muted dark:hover:text-dark-fg-ink"
							>
								<ExternalLink class="h-4 w-4" />
								Open
							</a>
							<a
								href={data.contentUrl}
								download={data.artifact.path.split('/').pop()}
								class="inline-flex items-center gap-1 text-fg-muted hover:text-fg-ink dark:text-dark-fg-muted dark:hover:text-dark-fg-ink"
							>
								<Download class="h-4 w-4" />
								Download
							</a>
						</div>
					{/if}
				</div>
			{/if}
		</div>

		<!-- DDx sidecar metadata panel -->
		{#if frontmatterEntries.length > 0}
			<div class="border border-border-line dark:border-dark-border-line">
				<div class="border-b border-border-line bg-bg-surface px-4 py-2 dark:border-dark-border-line dark:bg-dark-bg-surface">
					<h2 class="font-label-caps text-label-caps uppercase tracking-wide text-fg-muted dark:text-dark-fg-muted">
						DDx Metadata
					</h2>
				</div>
				<dl class="divide-y divide-border-line dark:divide-dark-border-line">
					{#each frontmatterEntries as [key, value]}
						<div class="flex gap-4 px-4 py-2">
							<dt class="w-32 shrink-0 font-mono-code text-mono-code text-fg-muted dark:text-dark-fg-muted">
								{key}
							</dt>
							<dd class="font-mono-code text-mono-code text-fg-ink dark:text-dark-fg-ink break-all">
								{value}
							</dd>
						</div>
					{/each}
				</dl>
			</div>
		{/if}
	{:else}
		<p class="text-body-sm text-fg-muted dark:text-dark-fg-muted">Artifact not found.</p>
	{/if}
</div>
