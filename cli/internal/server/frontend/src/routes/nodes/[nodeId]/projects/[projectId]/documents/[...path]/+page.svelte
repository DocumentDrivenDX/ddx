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
			.replace(/ /g, ' ')
			.replace(/\n{3,}/g, '\n\n')
			.trim();
	}

	function htmlFragmentToMarkdown(root: HTMLElement) {
		const blocks: string[] = [];

		function textOf(node: Node) {
			return (node.textContent ?? '').replace(/ /g, ' ').trim();
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
			class="flex items-center gap-1.5 px-2 py-1.5 text-body-sm text-fg-muted hover:bg-bg-surface dark:text-dark-fg-muted dark:hover:bg-dark-bg-surface"
		>
			<ArrowLeft class="h-4 w-4" />
			Documents
		</button>
		<span class="font-mono-code text-mono-code text-fg-muted dark:text-dark-fg-muted">{data.path}</span>
		{#if !editing && content}
			<button
				onclick={startEdit}
				class="ml-auto flex items-center gap-1.5 px-2 py-1.5 text-body-sm text-fg-muted hover:bg-bg-surface dark:text-dark-fg-muted dark:hover:bg-dark-bg-surface"
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
					class="border border-border-line bg-bg-surface px-3 py-2 text-body-sm text-error dark:border-dark-border-line dark:bg-dark-bg-surface dark:text-dark-error"
				>
					{saveError}
				</div>
			{/if}
			<fieldset
				class="flex w-fit gap-1 border border-border-line bg-bg-elevated p-1 text-body-sm dark:border-dark-border-line dark:bg-dark-bg-elevated"
			>
				<legend class="sr-only">Editor mode</legend>
				<label
					class="flex cursor-pointer items-center gap-2 px-3 py-1.5 has-[:checked]:bg-fg-ink has-[:checked]:text-bg-elevated dark:has-[:checked]:bg-dark-fg-ink dark:has-[:checked]:text-dark-bg-elevated"
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
					class="flex cursor-pointer items-center gap-2 px-3 py-1.5 has-[:checked]:bg-fg-ink has-[:checked]:text-bg-elevated dark:has-[:checked]:bg-dark-fg-ink dark:has-[:checked]:text-dark-bg-elevated"
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
							class="border border-border-line bg-bg-surface px-4 py-3 dark:border-dark-border-line dark:bg-dark-bg-surface"
						>
							<summary class="cursor-pointer text-body-sm font-medium text-fg-ink dark:text-dark-fg-ink">
								Frontmatter
							</summary>
							<pre
								class="mt-3 overflow-x-auto font-mono-code text-mono-code whitespace-pre-wrap text-fg-muted dark:text-dark-fg-muted">{editParts.frontmatterText}</pre>
						</details>
					{/if}
					<div
						contenteditable="true"
						role="textbox"
						aria-label="WYSIWYG markdown editor"
						tabindex="0"
						data-testid="wysiwyg-editor"
						oninput={handleWysiwygInput}
						class="doc-content min-h-[32rem] w-full border border-border-line bg-bg-elevated p-6 text-fg-ink focus:border-accent-lever focus:ring-1 focus:ring-accent-lever focus:outline-none dark:border-dark-border-line dark:bg-dark-bg-elevated dark:text-dark-fg-ink dark:focus:border-dark-accent-lever"
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
					class="w-full border border-border-line bg-bg-elevated px-4 py-3 font-mono-code text-mono-code text-fg-ink focus:border-accent-lever focus:ring-1 focus:ring-accent-lever focus:outline-none dark:border-dark-border-line dark:bg-dark-bg-elevated dark:text-dark-fg-ink dark:focus:border-dark-accent-lever"
				></textarea>
			{/if}
			<div class="flex justify-end gap-2">
				<button
					onclick={cancelEdit}
					class="border border-border-line px-4 py-2 text-body-sm text-fg-muted hover:bg-bg-surface dark:border-dark-border-line dark:text-dark-fg-muted dark:hover:bg-dark-bg-surface"
				>
					Cancel
				</button>
				<button
					onclick={handleSave}
					disabled={saving}
					class="bg-accent-lever px-4 py-2 text-body-sm font-medium text-bg-elevated hover:opacity-90 disabled:cursor-not-allowed disabled:opacity-50 dark:bg-dark-accent-lever dark:text-dark-bg-canvas"
				>
					{saving ? 'Saving…' : 'Save'}
				</button>
			</div>
		</div>
	{:else if content}
		<div
			bind:this={renderedElement}
			class="doc-content border border-border-line bg-bg-elevated p-6 dark:border-dark-border-line dark:bg-dark-bg-elevated"
		>
			<!-- eslint-disable-next-line svelte/no-at-html-tags -->
			{@html rendered}
		</div>
	{:else}
		<div
			class="border border-border-line bg-bg-elevated p-6 text-center text-body-sm text-fg-muted dark:border-dark-border-line dark:bg-dark-bg-elevated dark:text-dark-fg-muted"
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
		color: var(--doc-link-text);
		text-decoration: underline;
	}
	.doc-content :global(a:hover) {
		color: var(--doc-link-text-hover);
	}
	.doc-content :global(code) {
		font-family: ui-monospace, monospace;
		font-size: 0.875em;
		background-color: var(--doc-code-surface);
		color: var(--doc-code-text);
		padding: 0.125rem 0.375rem;
		border-radius: 0.25rem;
	}
	.doc-content :global(pre) {
		background-color: var(--doc-pre-surface);
		color: var(--doc-pre-text);
		padding: 1rem;
		overflow-x: auto;
		margin-bottom: 1rem;
	}
	.doc-content :global(pre code) {
		background-color: transparent;
		padding: 0;
		font-size: 0.875rem;
	}
	.doc-content :global(blockquote) {
		border-left: 4px solid var(--doc-quote-border);
		padding-left: 1rem;
		margin-left: 0;
		margin-bottom: 1rem;
		color: var(--doc-muted-text);
		font-style: italic;
	}
	.doc-content :global(table) {
		width: 100%;
		border-collapse: collapse;
		margin-bottom: 1rem;
		font-size: 0.875rem;
	}
	.doc-content :global(th) {
		background-color: var(--doc-table-heading-surface);
		border: 1px solid var(--doc-table-border);
		padding: 0.5rem 0.75rem;
		text-align: left;
		font-weight: 600;
	}
	.doc-content :global(td) {
		border: 1px solid var(--doc-table-border);
		padding: 0.5rem 0.75rem;
	}
	.doc-content :global(tr:hover) {
		background-color: var(--doc-row-hover-surface);
	}
	.doc-content :global(hr) {
		border: none;
		border-top: 1px solid var(--doc-rule-border);
		margin: 1.5rem 0;
	}
	.doc-content :global(img) {
		max-width: 100%;
		height: auto;
	}
</style>
