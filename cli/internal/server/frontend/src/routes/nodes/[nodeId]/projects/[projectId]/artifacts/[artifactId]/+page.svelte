<script lang="ts">
	import type { PageData } from './$types';
	import { goto } from '$app/navigation';
	import { ArrowLeft, Download, ExternalLink, Network } from 'lucide-svelte';
	import { marked } from 'marked';
	import DOMPurify from 'isomorphic-dompurify';
	import { onMount } from 'svelte';
	import { createClient } from '$lib/gql/client';
	import { gql } from 'graphql-request';

	let { data }: { data: PageData } = $props();

	const ARTIFACT_BY_PATH_QUERY = gql`
		query ArtifactsByPath($projectID: ID!, $search: String) {
			artifacts(projectID: $projectID, search: $search) {
				edges {
					node {
						id
						path
					}
				}
			}
		}
	`;

	const listHref = data.back ?? `/nodes/${data.nodeId}/projects/${data.projectId}/artifacts`;

	function stalenessBadge(staleness: string): { label: string; cls: string } {
		switch (staleness) {
			case 'fresh':
				return {
					label: 'fresh',
					cls: 'bg-green-100 text-green-800 dark:bg-green-900/30 dark:text-green-400'
				};
			case 'stale':
				return {
					label: 'stale',
					cls: 'bg-amber-100 text-amber-800 dark:bg-amber-900/30 dark:text-amber-400'
				};
			case 'missing':
				return {
					label: 'missing',
					cls: 'bg-red-100 text-red-800 dark:bg-red-900/30 dark:text-red-400'
				};
			default:
				return {
					label: staleness,
					cls: 'bg-bg-surface text-fg-muted dark:bg-dark-bg-surface dark:text-dark-fg-muted'
				};
		}
	}

	// Determine renderer type from mediaType
	function rendererType(
		mt: string
	): 'markdown' | 'svg' | 'excalidraw' | 'image' | 'pdf' | 'binary' {
		if (mt === 'text/markdown') return 'markdown';
		if (mt === 'image/svg+xml') return 'svg';
		if (mt === 'application/vnd.excalidraw+json') return 'excalidraw';
		if (mt.startsWith('image/')) return 'image';
		if (mt === 'application/pdf') return 'pdf';
		return 'binary';
	}

	// Rendered markdown HTML
	const renderedMarkdown = $derived.by(() => {
		if (!data.artifact?.content || rendererType(data.artifact.mediaType) !== 'markdown') return '';
		const raw = marked.parse(data.artifact.content, { async: false }) as string;
		return DOMPurify.sanitize(raw);
	});

	// Sanitized SVG content
	const sanitizedSvg = $derived.by(() => {
		if (!data.artifact?.content || rendererType(data.artifact.mediaType) !== 'svg') return '';
		return DOMPurify.sanitize(data.artifact.content, {
			USE_PROFILES: { svg: true, svgFilters: true }
		});
	});

	// DDx frontmatter parsed as key/value pairs
	const frontmatterEntries = $derived.by((): [string, string][] => {
		if (!data.artifact?.ddxFrontmatter) return [];
		try {
			const fm = JSON.parse(data.artifact.ddxFrontmatter);
			if (typeof fm !== 'object' || fm === null) return [];
			return Object.entries(fm).map(([k, v]) => [
				k,
				typeof v === 'object' ? JSON.stringify(v) : String(v)
			]);
		} catch {
			return [];
		}
	});

	// Offline detection for Excalidraw
	let isOnline = $state(true);
	onMount(() => {
		isOnline = navigator.onLine;
		const handleOnline = () => {
			isOnline = true;
		};
		const handleOffline = () => {
			isOnline = false;
		};
		window.addEventListener('online', handleOnline);
		window.addEventListener('offline', handleOffline);
		return () => {
			window.removeEventListener('online', handleOnline);
			window.removeEventListener('offline', handleOffline);
		};
	});

	// Resolve a relative href against the base directory of the current artifact.
	function resolvePath(relativePath: string, baseDir: string): string {
		if (relativePath.startsWith('/')) return relativePath.slice(1);
		const parts = baseDir ? baseDir.split('/') : [];
		for (const segment of relativePath.split('/')) {
			if (segment === '..') parts.pop();
			else if (segment !== '.') parts.push(segment);
		}
		return parts.join('/');
	}

	// Process markdown links: external links get target="_blank"; relative intra-repo
	// links are rewritten to href="#" with data-ddx-intra-path for client-side navigation.
	function processMarkdownHtml(html: string): string {
		const artifactPath = data.artifact?.path ?? '';
		const baseDir = artifactPath.split('/').slice(0, -1).join('/');
		return html.replace(/<a\s+href="([^"]+)"/g, (_match, href) => {
			const isExternal = /^https?:\/\//.test(href) || href.startsWith('//');
			if (isExternal) {
				return `<a href="${href}" target="_blank" rel="noopener noreferrer"`;
			}
			const resolvedPath = resolvePath(href, baseDir);
			return `<a href="#" data-ddx-intra-path="${resolvedPath}"`;
		});
	}

	const finalMarkdownHtml = $derived(processMarkdownHtml(renderedMarkdown));

	async function handleMarkdownClick(e: MouseEvent) {
		const anchor = (e.target as Element).closest('a[data-ddx-intra-path]');
		if (!anchor) return;
		e.preventDefault();
		const intraPath = anchor.getAttribute('data-ddx-intra-path');
		if (!intraPath) return;
		const client = createClient();
		const result = await client.request<{
			artifacts: { edges: { node: { id: string; path: string } }[] };
		}>(ARTIFACT_BY_PATH_QUERY, { projectID: data.projectId, search: intraPath });
		const match = result.artifacts.edges.find((edge) => edge.node.path === intraPath);
		if (match) {
			goto(
				`/nodes/${data.nodeId}/projects/${data.projectId}/artifacts/${encodeURIComponent(match.node.id)}`
			);
		}
	}
</script>

<div class="space-y-4">
	<div class="flex items-center justify-between">
		<a
			href={listHref}
			class="text-body-sm text-fg-muted hover:text-fg-ink dark:text-dark-fg-muted dark:hover:text-dark-fg-ink inline-flex items-center gap-1"
		>
			<ArrowLeft class="h-4 w-4" />
			Back to Artifacts
		</a>
		{#if data.artifact?.mediaType === 'text/markdown'}
			<a
				href={`/nodes/${data.nodeId}/projects/${data.projectId}/graph`}
				class="text-body-sm text-fg-muted hover:text-fg-ink dark:text-dark-fg-muted dark:hover:text-dark-fg-ink inline-flex items-center gap-1"
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
					class="font-label-caps text-label-caps mt-1 inline-block rounded-full px-2 py-0.5 uppercase {badge.cls}"
				>
					{badge.label}
				</span>
			</div>

			<!-- Core metadata -->
			<dl class="text-body-sm space-y-1">
				<div class="flex gap-4">
					<dt class="text-fg-muted dark:text-dark-fg-muted w-24 shrink-0">Path</dt>
					<dd class="font-mono-code text-mono-code text-fg-ink dark:text-dark-fg-ink break-all">
						{data.artifact.path}
					</dd>
				</div>
				<div class="flex gap-4">
					<dt class="text-fg-muted dark:text-dark-fg-muted w-24 shrink-0">Media type</dt>
					<dd class="font-mono-code text-mono-code text-fg-ink dark:text-dark-fg-ink">
						<span class="bg-bg-surface dark:bg-dark-bg-surface rounded px-1.5 py-0.5 text-xs">
							{data.artifact.mediaType}
						</span>
					</dd>
				</div>
				{#if data.artifact.description}
					<div class="flex gap-4">
						<dt class="text-fg-muted dark:text-dark-fg-muted w-24 shrink-0">Description</dt>
						<dd class="text-fg-ink dark:text-dark-fg-ink">{data.artifact.description}</dd>
					</div>
				{/if}
				{#if data.artifact.updatedAt}
					<div class="flex gap-4">
						<dt class="text-fg-muted dark:text-dark-fg-muted w-24 shrink-0">Updated</dt>
						<dd class="text-fg-ink dark:text-dark-fg-ink">
							{new Date(data.artifact.updatedAt).toLocaleString()}
						</dd>
					</div>
				{/if}
			</dl>
		</div>

		<!-- Renderer -->
		<div class="border-border-line dark:border-dark-border-line border">
			{#if rtype === 'markdown'}
				<!-- Markdown renderer: rendered with syntax-highlighted code blocks -->
				<!-- Relative intra-repo links are intercepted by handleMarkdownClick for in-UI navigation -->
				<!-- eslint-disable-next-line svelte/no-at-html-tags -->
				<div
					class="prose prose-sm dark:prose-invert max-w-none p-4"
					onclick={handleMarkdownClick}
					role="presentation"
				>
					{@html finalMarkdownHtml}
				</div>
			{:else if rtype === 'svg'}
				<!-- SVG renderer: DOMPurify-sanitized inline SVG -->
				<div class="flex items-center justify-center p-4">
					<!-- eslint-disable-next-line svelte/no-at-html-tags -->
					{@html sanitizedSvg}
				</div>
				<div class="border-border-line dark:border-dark-border-line border-t p-2">
					{#if data.contentUrl}
						<a
							href={data.contentUrl}
							download={data.artifact.path.split('/').pop()}
							class="text-body-sm text-fg-muted hover:text-fg-ink dark:text-dark-fg-muted dark:hover:text-dark-fg-ink inline-flex items-center gap-1"
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
								class="text-body-sm text-accent-lever dark:text-dark-accent-lever inline-flex items-center gap-1 hover:underline"
							>
								<Download class="h-4 w-4" />
								Download Excalidraw JSON
							</a>
						{/if}
					</div>
				{/if}
				{#if data.contentUrl}
					<div class="border-border-line dark:border-dark-border-line border-t p-2">
						<a
							href={data.contentUrl}
							download={data.artifact.path.split('/').pop()}
							class="text-body-sm text-fg-muted hover:text-fg-ink dark:text-dark-fg-muted dark:hover:text-dark-fg-ink inline-flex items-center gap-1"
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
					<div class="border-border-line dark:border-dark-border-line border-t p-2">
						<div class="flex gap-3">
							<a
								href={data.contentUrl}
								target="_blank"
								rel="noopener noreferrer"
								class="text-body-sm text-fg-muted hover:text-fg-ink dark:text-dark-fg-muted dark:hover:text-dark-fg-ink inline-flex items-center gap-1"
							>
								<ExternalLink class="h-4 w-4" />
								Open
							</a>
							<a
								href={data.contentUrl}
								download={data.artifact.path.split('/').pop()}
								class="text-body-sm text-fg-muted hover:text-fg-ink dark:text-dark-fg-muted dark:hover:text-dark-fg-ink inline-flex items-center gap-1"
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
						<div
							class="border-border-line dark:border-dark-border-line flex items-center gap-3 border-t pt-2"
						>
							<a
								href={data.contentUrl}
								target="_blank"
								rel="noopener noreferrer"
								class="text-body-sm text-fg-muted hover:text-fg-ink dark:text-dark-fg-muted dark:hover:text-dark-fg-ink inline-flex items-center gap-1"
							>
								<ExternalLink class="h-4 w-4" />
								Open PDF
							</a>
							<a
								href={data.contentUrl}
								download={data.artifact.path.split('/').pop()}
								class="text-body-sm text-fg-muted hover:text-fg-ink dark:text-dark-fg-muted dark:hover:text-dark-fg-ink inline-flex items-center gap-1"
							>
								<Download class="h-4 w-4" />
								Download
							</a>
						</div>
					</div>
				{/if}
			{:else}
				<!-- Binary/unknown: metadata only with open/download affordance -->
				<div class="text-body-sm text-fg-muted dark:text-dark-fg-muted p-4">
					<p>Binary content — no preview available.</p>
					{#if data.contentUrl}
						<div class="mt-3 flex gap-3">
							<a
								href={data.contentUrl}
								target="_blank"
								rel="noopener noreferrer"
								class="text-fg-muted hover:text-fg-ink dark:text-dark-fg-muted dark:hover:text-dark-fg-ink inline-flex items-center gap-1"
							>
								<ExternalLink class="h-4 w-4" />
								Open
							</a>
							<a
								href={data.contentUrl}
								download={data.artifact.path.split('/').pop()}
								class="text-fg-muted hover:text-fg-ink dark:text-dark-fg-muted dark:hover:text-dark-fg-ink inline-flex items-center gap-1"
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
			<div class="border-border-line dark:border-dark-border-line border">
				<div
					class="border-border-line bg-bg-surface dark:border-dark-border-line dark:bg-dark-bg-surface border-b px-4 py-2"
				>
					<h2
						class="font-label-caps text-label-caps text-fg-muted dark:text-dark-fg-muted tracking-wide uppercase"
					>
						DDx Metadata
					</h2>
				</div>
				<dl class="divide-border-line dark:divide-dark-border-line divide-y">
					{#each frontmatterEntries as [key, value]}
						<div class="flex gap-4 px-4 py-2">
							<dt
								class="font-mono-code text-mono-code text-fg-muted dark:text-dark-fg-muted w-32 shrink-0"
							>
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
