<script lang="ts">
	import type { PageData } from './$types';
	import { goto } from '$app/navigation';
	import { page } from '$app/stores';
	import { marked } from 'marked';
	import { ArrowLeft } from 'lucide-svelte';

	let { data }: { data: PageData } = $props();

	let rendered = $derived(data.content ? (marked.parse(data.content) as string) : '');

	function handleBack() {
		const p = $page.params as Record<string, string>;
		goto(`/nodes/${p['nodeId']}/projects/${p['projectId']}/documents`);
	}
</script>

<div class="space-y-4">
	<div class="flex items-center gap-3">
		<button
			onclick={handleBack}
			class="flex items-center gap-1.5 rounded-md px-2 py-1.5 text-sm text-gray-600 hover:bg-gray-100 dark:text-gray-400 dark:hover:bg-gray-800"
		>
			<ArrowLeft class="h-4 w-4" />
			Documents
		</button>
		<span class="font-mono text-xs text-gray-400 dark:text-gray-600">{data.path}</span>
	</div>

	{#if data.content}
		<div class="doc-content rounded-lg border border-gray-200 bg-white p-6 dark:border-gray-700 dark:bg-gray-900">
			{@html rendered}
		</div>
	{:else}
		<div class="rounded-lg border border-gray-200 bg-white p-6 text-center text-gray-400 dark:border-gray-700 dark:bg-gray-900 dark:text-gray-600">
			Document not found.
		</div>
	{/if}
</div>

<style>
	.doc-content :global(h1) {
		font-size: 1.875rem;
		font-weight: 700;
		margin-bottom: 1rem;
		margin-top: 1.5rem;
		color: inherit;
	}
	.doc-content :global(h2) {
		font-size: 1.5rem;
		font-weight: 600;
		margin-bottom: 0.75rem;
		margin-top: 1.5rem;
		color: inherit;
	}
	.doc-content :global(h3) {
		font-size: 1.25rem;
		font-weight: 600;
		margin-bottom: 0.5rem;
		margin-top: 1.25rem;
		color: inherit;
	}
	.doc-content :global(h4),
	.doc-content :global(h5),
	.doc-content :global(h6) {
		font-size: 1rem;
		font-weight: 600;
		margin-bottom: 0.5rem;
		margin-top: 1rem;
		color: inherit;
	}
	.doc-content :global(p) {
		margin-bottom: 1rem;
		line-height: 1.75;
		color: inherit;
	}
	.doc-content :global(ul),
	.doc-content :global(ol) {
		margin-bottom: 1rem;
		padding-left: 1.5rem;
	}
	.doc-content :global(ul) {
		list-style-type: disc;
	}
	.doc-content :global(ol) {
		list-style-type: decimal;
	}
	.doc-content :global(li) {
		margin-bottom: 0.25rem;
		line-height: 1.75;
	}
	.doc-content :global(a) {
		color: #2563eb;
		text-decoration: underline;
	}
	.doc-content :global(a:hover) {
		color: #1d4ed8;
	}
	.doc-content :global(code) {
		font-family: ui-monospace, monospace;
		font-size: 0.875em;
		background-color: #f3f4f6;
		padding: 0.125rem 0.375rem;
		border-radius: 0.25rem;
	}
	.doc-content :global(pre) {
		background-color: #1f2937;
		color: #f9fafb;
		padding: 1rem;
		border-radius: 0.5rem;
		overflow-x: auto;
		margin-bottom: 1rem;
	}
	.doc-content :global(pre code) {
		background-color: transparent;
		padding: 0;
		font-size: 0.875rem;
	}
	.doc-content :global(blockquote) {
		border-left: 4px solid #d1d5db;
		padding-left: 1rem;
		margin-left: 0;
		margin-bottom: 1rem;
		color: #6b7280;
		font-style: italic;
	}
	.doc-content :global(table) {
		width: 100%;
		border-collapse: collapse;
		margin-bottom: 1rem;
		font-size: 0.875rem;
	}
	.doc-content :global(th) {
		background-color: #f9fafb;
		border: 1px solid #e5e7eb;
		padding: 0.5rem 0.75rem;
		text-align: left;
		font-weight: 600;
	}
	.doc-content :global(td) {
		border: 1px solid #e5e7eb;
		padding: 0.5rem 0.75rem;
	}
	.doc-content :global(tr:hover) {
		background-color: #f9fafb;
	}
	.doc-content :global(hr) {
		border: none;
		border-top: 1px solid #e5e7eb;
		margin: 1.5rem 0;
	}
	.doc-content :global(img) {
		max-width: 100%;
		height: auto;
		border-radius: 0.375rem;
	}

	/* Dark mode overrides */
	:global(.dark) .doc-content :global(a) {
		color: #60a5fa;
	}
	:global(.dark) .doc-content :global(a:hover) {
		color: #93c5fd;
	}
	:global(.dark) .doc-content :global(code) {
		background-color: #374151;
		color: #f9fafb;
	}
	:global(.dark) .doc-content :global(blockquote) {
		border-left-color: #4b5563;
		color: #9ca3af;
	}
	:global(.dark) .doc-content :global(th) {
		background-color: #1f2937;
		border-color: #374151;
	}
	:global(.dark) .doc-content :global(td) {
		border-color: #374151;
	}
	:global(.dark) .doc-content :global(tr:hover) {
		background-color: #1f2937;
	}
	:global(.dark) .doc-content :global(hr) {
		border-top-color: #374151;
	}
</style>
