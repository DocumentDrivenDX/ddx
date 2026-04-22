<script lang="ts">
	import type { PageData } from './$types';
	import { goto, invalidateAll } from '$app/navigation';
	import { resolve } from '$app/paths';
	import { page } from '$app/stores';
	import { tick } from 'svelte';
	import { marked } from 'marked';
	import DOMPurify from 'isomorphic-dompurify';
	import { ArrowLeft, Pencil } from 'lucide-svelte';
	import { gql } from 'graphql-request';
	import { createClient } from '$lib/gql/client';

	let { data }: { data: PageData } = $props();

	let content = $derived(data.content ?? '');
	let editing = $state(false);
	let editContent = $state('');
	let editorMode = $state<'wysiwyg' | 'plain'>('wysiwyg');
	let wysiwygDraftBody = $state('');
	let saving = $state(false);
	let saveError = $state<string | null>(null);
	let renderedElement: HTMLDivElement | undefined = $state();

	// Sanitize: marked dropped its built-in sanitize option, so raw <script>,
	// onerror handlers, and javascript: URIs in a document would execute in
	// the admin UI context. A compromised document on disk must not be able
	// to hijack a session that has documentWrite access.
	let rendered = $derived(renderMarkdown(splitMarkdown(content).body));
	let editParts = $derived(splitMarkdown(editContent));
	let renderedEditBody = $derived(renderMarkdown(editParts.body));

	const DOCUMENT_WRITE = gql`
		mutation DocumentWrite($path: String!, $content: String!) {
			documentWrite(path: $path, content: $content) {
				path
				content
			}
		}
	`;

	interface DocumentWriteResult {
		documentWrite: { path: string; content?: string | null };
	}

	function splitMarkdown(markdown: string) {
		const match = markdown.match(/^---\r?\n[\s\S]*?\r?\n---(?:\r?\n|$)/);
		if (!match) {
			return { frontmatter: '', frontmatterText: '', body: markdown };
		}
		const frontmatter = match[0];
		const frontmatterText = frontmatter.replace(/^---\r?\n/, '').replace(/\r?\n---(?:\r?\n|$)/, '');
		return {
			frontmatter,
			frontmatterText,
			body: markdown.slice(frontmatter.length)
		};
	}

	function renderMarkdown(markdown: string) {
		return markdown ? DOMPurify.sanitize(marked.parse(markdown) as string) : '';
	}

	function normalizeMarkdownBody(body: string) {
		return body
			.replace(/\u00a0/g, ' ')
			.replace(/\n{3,}/g, '\n\n')
			.trim();
	}

	function htmlFragmentToMarkdown(root: HTMLElement) {
		const blocks: string[] = [];

		function textOf(node: Node) {
			return (node.textContent ?? '').replace(/\u00a0/g, ' ').trim();
		}

		function addBlock(value: string) {
			const normalized = normalizeMarkdownBody(value);
			if (normalized) blocks.push(normalized);
		}

		for (const child of Array.from(root.childNodes)) {
			if (child.nodeType === Node.TEXT_NODE) {
				addBlock(child.textContent ?? '');
				continue;
			}
			if (!(child instanceof HTMLElement)) continue;
			const tag = child.tagName.toLowerCase();
			if (/^h[1-6]$/.test(tag)) {
				addBlock(`${'#'.repeat(Number(tag.slice(1)))} ${textOf(child)}`);
			} else if (tag === 'ul' || tag === 'ol') {
				const ordered = tag === 'ol';
				const items = Array.from(child.querySelectorAll(':scope > li')).map((li, index) => {
					const marker = ordered ? `${index + 1}.` : '-';
					return `${marker} ${textOf(li)}`;
				});
				addBlock(items.join('\n'));
			} else if (tag === 'pre') {
				blocks.push(`\`\`\`\n${child.textContent ?? ''}\n\`\`\``);
			} else if (tag === 'blockquote') {
				addBlock(
					textOf(child)
						.split('\n')
						.map((line) => `> ${line}`)
						.join('\n')
				);
			} else {
				addBlock(textOf(child));
			}
		}

		return blocks.join('\n\n');
	}

	function commitWysiwygDraft() {
		if (editorMode !== 'wysiwyg') return;
		const body = normalizeMarkdownBody(wysiwygDraftBody);
		editContent = `${editParts.frontmatter}${body}${body ? '\n' : ''}`;
	}

	function setEditorMode(mode: 'wysiwyg' | 'plain') {
		if (mode === editorMode) return;
		commitWysiwygDraft();
		editorMode = mode;
		if (mode === 'wysiwyg') {
			wysiwygDraftBody = splitMarkdown(editContent).body;
		}
	}

	function handleWysiwygInput(event: Event) {
		wysiwygDraftBody = htmlFragmentToMarkdown(event.currentTarget as HTMLElement);
	}

	function isExternalHttpLink(href: string) {
		return /^https?:\/\//i.test(href);
	}

	function encodeDocPath(path: string) {
		return path.split('/').map(encodeURIComponent).join('/');
	}

	function resolveDocumentHref(href: string) {
		const baseDir = data.path.includes('/')
			? data.path.slice(0, data.path.lastIndexOf('/') + 1)
			: '';
		const resolved = new URL(href, `https://ddx.local/${baseDir}`);
		return {
			path: decodeURIComponent(resolved.pathname.replace(/^\/+/, '')),
			hash: resolved.hash ? decodeURIComponent(resolved.hash.slice(1)) : ''
		};
	}

	function slugify(value: string) {
		return value
			.toLowerCase()
			.trim()
			.replace(/[^\w\s-]/g, '')
			.replace(/\s+/g, '-');
	}

	function scrollToAnchor(anchor: string) {
		if (!anchor) return;
		const decoded = decodeURIComponent(anchor);
		let target = document.getElementById(decoded);
		if (!target && renderedElement) {
			target = Array.from(renderedElement.querySelectorAll('h1, h2, h3, h4, h5, h6')).find(
				(heading) => slugify(heading.textContent ?? '') === decoded
			) as HTMLElement | null;
		}
		target?.scrollIntoView({ behavior: 'smooth', block: 'start' });
	}

	function handleRenderedClick(event: MouseEvent) {
		const target = event.target as Element | null;
		const anchor = target?.closest?.('a[href]') as HTMLAnchorElement | null;
		if (!anchor || !renderedElement?.contains(anchor)) return;

		const href = anchor.getAttribute('href') ?? '';
		if (!href || isExternalHttpLink(href)) return;

		if (href.startsWith('#')) {
			event.preventDefault();
			scrollToAnchor(href.slice(1));
			return;
		}

		const resolved = resolveDocumentHref(href);
		if (!resolved.path.endsWith('.md')) return;

		event.preventDefault();
		if (resolved.path === data.path && resolved.hash) {
			scrollToAnchor(resolved.hash);
			return;
		}

		const p = $page.params as Record<string, string>;
		const hash = resolved.hash ? `#${encodeURIComponent(resolved.hash)}` : '';
		goto(
			resolve(
				`/nodes/${p['nodeId']}/projects/${p['projectId']}/documents/${encodeDocPath(resolved.path)}${hash}`
			),
			{ keepFocus: true, noScroll: Boolean(resolved.hash) }
		).then(() => {
			if (resolved.hash) scrollToAnchor(resolved.hash);
		});
	}

	function enhanceRenderedLinks() {
		if (!renderedElement) return;
		for (const link of renderedElement.querySelectorAll<HTMLAnchorElement>('a[href]')) {
			const href = link.getAttribute('href') ?? '';
			if (isExternalHttpLink(href)) {
				link.target = '_blank';
				link.rel = 'noopener';
			}
		}
	}

	function scheduleRenderedLinkEnhancement(html: string) {
		if (html || html === '') {
			tick().then(enhanceRenderedLinks);
		}
	}

	$effect(() => {
		scheduleRenderedLinkEnhancement(rendered);
	});

	$effect(() => {
		const element = renderedElement;
		if (!element) return;
		element.addEventListener('click', handleRenderedClick);
		return () => element.removeEventListener('click', handleRenderedClick);
	});

	function handleBack() {
		const p = $page.params as Record<string, string>;
		goto(resolve(`/nodes/${p['nodeId']}/projects/${p['projectId']}/documents`));
	}

	function startEdit() {
		editContent = content;
		editorMode = 'wysiwyg';
		wysiwygDraftBody = splitMarkdown(content).body;
		saveError = null;
		editing = true;
	}

	function cancelEdit() {
		editing = false;
		saveError = null;
	}

	async function handleSave() {
		commitWysiwygDraft();
		saving = true;
		saveError = null;
		try {
			const client = createClient();
			const result = await client.request<DocumentWriteResult>(DOCUMENT_WRITE, {
				path: data.path,
				content: editContent
			});
			if (result.documentWrite.content != null) {
				editContent = result.documentWrite.content;
			}
			editing = false;
			await invalidateAll();
		} catch (e) {
			saveError = e instanceof Error ? e.message : 'Save failed';
		} finally {
			saving = false;
		}
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
		{#if !editing && content}
			<button
				onclick={startEdit}
				class="ml-auto flex items-center gap-1.5 rounded-md px-2 py-1.5 text-sm text-gray-600 hover:bg-gray-100 dark:text-gray-400 dark:hover:bg-gray-800"
			>
				<Pencil class="h-4 w-4" />
				Edit
			</button>
		{/if}
	</div>

	{#if editing}
		<div class="space-y-2">
			{#if saveError}
				<div
					class="rounded-md border border-red-200 bg-red-50 px-3 py-2 text-sm text-red-700 dark:border-red-800 dark:bg-red-900/30 dark:text-red-400"
				>
					{saveError}
				</div>
			{/if}
			<fieldset
				class="flex w-fit gap-1 rounded-md border border-gray-200 bg-white p-1 text-sm dark:border-gray-700 dark:bg-gray-900"
			>
				<legend class="sr-only">Editor mode</legend>
				<label
					class="flex cursor-pointer items-center gap-2 rounded px-3 py-1.5 has-[:checked]:bg-gray-900 has-[:checked]:text-white dark:has-[:checked]:bg-gray-100 dark:has-[:checked]:text-gray-950"
				>
					<input
						type="radio"
						name="editor-mode"
						value="wysiwyg"
						aria-label="WYSIWYG"
						checked={editorMode === 'wysiwyg'}
						onchange={() => setEditorMode('wysiwyg')}
					/>
					WYSIWYG
				</label>
				<label
					class="flex cursor-pointer items-center gap-2 rounded px-3 py-1.5 has-[:checked]:bg-gray-900 has-[:checked]:text-white dark:has-[:checked]:bg-gray-100 dark:has-[:checked]:text-gray-950"
				>
					<input
						type="radio"
						name="editor-mode"
						value="plain"
						aria-label="Plain"
						checked={editorMode === 'plain'}
						onchange={() => setEditorMode('plain')}
					/>
					Plain
				</label>
			</fieldset>

			{#if editorMode === 'wysiwyg'}
				<div class="space-y-3">
					{#if editParts.frontmatter}
						<details
							class="rounded-md border border-gray-200 bg-gray-50 px-4 py-3 dark:border-gray-700 dark:bg-gray-900/60"
						>
							<summary class="cursor-pointer text-sm font-medium text-gray-800 dark:text-gray-100">
								Frontmatter
							</summary>
							<pre
								class="mt-3 overflow-x-auto font-mono text-xs whitespace-pre-wrap text-gray-700 dark:text-gray-300">{editParts.frontmatterText}</pre>
						</details>
					{/if}
					<div
						contenteditable="true"
						role="textbox"
						aria-label="WYSIWYG markdown editor"
						tabindex="0"
						data-testid="wysiwyg-editor"
						oninput={handleWysiwygInput}
						class="doc-content min-h-[32rem] w-full rounded-lg border border-gray-300 bg-white p-6 text-gray-900 focus:border-blue-500 focus:ring-1 focus:ring-blue-500 focus:outline-none dark:border-gray-600 dark:bg-gray-900 dark:text-gray-100 dark:focus:border-blue-400"
					>
						<!-- eslint-disable-next-line svelte/no-at-html-tags -->
						{@html renderedEditBody}
					</div>
				</div>
			{:else}
				<label class="sr-only" for="plain-markdown-editor">Plain markdown editor</label>
				<textarea
					id="plain-markdown-editor"
					aria-label="Plain markdown editor"
					bind:value={editContent}
					rows={24}
					class="w-full rounded-lg border border-gray-300 bg-white px-4 py-3 font-mono text-sm text-gray-900 focus:border-blue-500 focus:ring-1 focus:ring-blue-500 focus:outline-none dark:border-gray-600 dark:bg-gray-900 dark:text-gray-100 dark:focus:border-blue-400"
				></textarea>
			{/if}
			<div class="flex justify-end gap-2">
				<button
					onclick={cancelEdit}
					class="rounded-md border border-gray-300 px-4 py-2 text-sm text-gray-700 hover:bg-gray-50 dark:border-gray-600 dark:text-gray-300 dark:hover:bg-gray-800"
				>
					Cancel
				</button>
				<button
					onclick={handleSave}
					disabled={saving}
					class="rounded-md bg-blue-600 px-4 py-2 text-sm font-medium text-white hover:bg-blue-700 disabled:cursor-not-allowed disabled:opacity-50"
				>
					{saving ? 'Saving…' : 'Save'}
				</button>
			</div>
		</div>
	{:else if content}
		<div
			bind:this={renderedElement}
			class="doc-content rounded-lg border border-gray-200 bg-white p-6 dark:border-gray-700 dark:bg-gray-900"
		>
			<!-- eslint-disable-next-line svelte/no-at-html-tags -->
			{@html rendered}
		</div>
	{:else}
		<div
			class="rounded-lg border border-gray-200 bg-white p-6 text-center text-gray-400 dark:border-gray-700 dark:bg-gray-900 dark:text-gray-600"
		>
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
