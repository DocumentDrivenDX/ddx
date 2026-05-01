<script lang="ts">
	import { page } from '$app/stores';
	import { invalidateAll } from '$app/navigation';
	import { ChevronDown, ChevronRight, Copy } from 'lucide-svelte';
	import { createClient } from '$lib/gql/client';
	import { gql } from 'graphql-request';

	export interface GraphIssue {
		issueId?: string;
		kind: string;
		path: string | null;
		id: string | null;
		message: string;
		relatedPath: string | null;
	}

	let {
		issues,
		pathToDocId = {}
	}: { issues: GraphIssue[]; pathToDocId?: Record<string, string> } = $props();

	type RepairStrategy = 'REMOVE_MISSING_DEP' | 'APPLY_SUGGESTED_ID' | 'CLEAN_PATH_MAP';

	const REPAIR_MUTATION = gql`
		mutation GraphRepairIssue($issueId: ID!, $strategy: RepairStrategy!) {
			graphRepairIssue(issueId: $issueId, strategy: $strategy) {
				success
				error
			}
		}
	`;

	// Auto-repairable kinds. duplicate_id is auto-repairable only when no other
	// document already declares a depends_on edge to the duplicate ID — the
	// server enforces that constraint and returns an inline error otherwise.
	const REPAIRABLE: Record<string, RepairStrategy> = {
		missing_dep: 'REMOVE_MISSING_DEP',
		duplicate_id: 'APPLY_SUGGESTED_ID'
	};

	function strategyFor(issue: GraphIssue): RepairStrategy | null {
		if (!issue.issueId) return null;
		return REPAIRABLE[issue.kind] ?? null;
	}

	let previewing = $state<Record<string, boolean>>({});
	let pending = $state<Record<string, boolean>>({});
	let inlineErrors = $state<Record<string, string>>({});
	let previewedSuggestions = $state<Record<string, string>>({});

	const KIND_LABELS: Record<string, string> = {
		duplicate_id: 'Duplicate ID',
		missing_dep: 'Missing dep target',
		id_path_missing: 'Broken id_to_path',
		id_path_mismatch: 'id_to_path mismatch',
		parse_error: 'Parse error',
		required_root_missing: 'Required root missing',
		cascade_unknown: 'Unknown cascade target',
		cycle: 'Cycle'
	};

	function labelFor(kind: string): string {
		return KIND_LABELS[kind] ?? kind;
	}

	const groups = $derived.by(() => {
		const map = new Map<string, GraphIssue[]>();
		for (const issue of issues) {
			const list = map.get(issue.kind) ?? [];
			list.push(issue);
			map.set(issue.kind, list);
		}
		return Array.from(map.entries())
			.map(([kind, list]) => ({ kind, issues: list }))
			.sort((a, b) => labelFor(a.kind).localeCompare(labelFor(b.kind)));
	});

	let expanded = $state<Record<string, boolean>>({});

	function toggle(kind: string) {
		expanded[kind] = !expanded[kind];
	}

	function docLink(path: string | null): string | null {
		if (!path) return null;
		const p = $page.params as Record<string, string>;
		const nodeId = p['nodeId'];
		const projectId = p['projectId'];
		if (!nodeId || !projectId) return null;
		// Use /artifacts/:artifactId when we can resolve the document ID from the path
		const docId = pathToDocId[path];
		if (docId) {
			return `/nodes/${nodeId}/projects/${projectId}/artifacts/${encodeURIComponent('doc:' + docId)}`;
		}
		// Fall back to /documents/:path
		const segments = path
			.split('/')
			.filter((s) => s.length > 0)
			.map(encodeURIComponent)
			.join('/');
		return `/nodes/${nodeId}/projects/${projectId}/documents/${segments}`;
	}

	// Deterministic mirror of cli/internal/docgraph.SuggestUniqueID; we keep a
	// browser-side copy so the "copy unique id" button works without a
	// round-trip. Keep in sync with the Go implementation's SHA-1 truncation.
	async function suggestUniqueID(id: string | null, path: string | null): Promise<string> {
		const safePath = (path ?? '').trim();
		const safeID = (id ?? '').trim();
		const encoder = new TextEncoder();
		const digest = await crypto.subtle.digest('SHA-1', encoder.encode(safePath));
		const bytes = new Uint8Array(digest);
		const suffix = Array.from(bytes.slice(0, 4))
			.map((b) => b.toString(16).padStart(2, '0'))
			.join('');
		if (safeID === '') return `doc-${suffix}`;
		return `${safeID}-${suffix}`;
	}

	async function copySuggestion(issue: GraphIssue, event: Event) {
		event.stopPropagation();
		const suggestion = await suggestUniqueID(issue.id, issue.path);
		try {
			await navigator.clipboard.writeText(suggestion);
		} catch {
			// Clipboard API unavailable (e.g. non-secure context) — surface the
			// suggestion inline so the user can still copy it manually.
			window.prompt('Copy suggested unique id:', suggestion);
		}
	}

	async function copyMessage(issue: GraphIssue, event: Event) {
		event.stopPropagation();
		try {
			await navigator.clipboard.writeText(issue.message);
		} catch {
			window.prompt('Copy issue message:', issue.message);
		}
	}

	function dependencyRemovalSnippet(issue: GraphIssue): string {
		const id = (issue.id ?? '').trim();
		if (!id) return '';
		return `    - ${id}`;
	}

	async function copyDependencyRemovalSnippet(issue: GraphIssue, event: Event) {
		event.stopPropagation();
		const snippet = dependencyRemovalSnippet(issue);
		if (!snippet) return;
		try {
			await navigator.clipboard.writeText(snippet);
		} catch {
			window.prompt('Copy frontmatter line to remove:', snippet);
		}
	}

	async function togglePreview(issue: GraphIssue) {
		const key = issue.issueId ?? '';
		if (!key) return;
		const next = !previewing[key];
		previewing[key] = next;
		inlineErrors[key] = '';
		if (next && issue.kind === 'duplicate_id' && !previewedSuggestions[key]) {
			previewedSuggestions[key] = await suggestUniqueID(issue.id, issue.path);
		}
	}

	async function applyFix(issue: GraphIssue) {
		const key = issue.issueId ?? '';
		const strategy = strategyFor(issue);
		if (!key || !strategy) return;
		pending[key] = true;
		inlineErrors[key] = '';
		try {
			const client = createClient();
			const res = await client.request<{
				graphRepairIssue: { success: boolean; error: string | null };
			}>(REPAIR_MUTATION, { issueId: key, strategy });
			if (!res.graphRepairIssue.success) {
				inlineErrors[key] =
					res.graphRepairIssue.error ?? 'Repair failed; use the copy-snippet fallback.';
				return;
			}
			// Refresh page data so the panel and graph reflect the new state.
			await invalidateAll();
		} catch (err) {
			inlineErrors[key] = err instanceof Error ? err.message : String(err);
		} finally {
			pending[key] = false;
		}
	}

	function diffRemoveDep(issue: GraphIssue): { before: string; after: string } {
		const id = (issue.id ?? '').trim();
		return {
			before: `depends_on:\n  - ${id}`,
			after: `depends_on:\n  # (entry removed)`
		};
	}

	function diffApplySuggestedID(issue: GraphIssue, suggested: string): { before: string; after: string } {
		const id = (issue.id ?? '').trim();
		return {
			before: `id: ${id}`,
			after: `id: ${suggested}`
		};
	}
</script>

<section
	data-testid="integrity-panel"
	class="shrink-0 rounded-none border border-border-line bg-bg-surface text-sm text-fg-ink dark:border-dark-border-line dark:bg-dark-bg-surface dark:text-dark-fg-ink"
>
	<header class="border-b border-border-line px-4 py-2 font-semibold dark:border-dark-border-line">
		Integrity: {issues.length}
		{issues.length === 1 ? 'issue' : 'issues'}
	</header>
	<ul class="divide-y divide-border-line dark:divide-dark-border-line">
		{#each groups as group (group.kind)}
			{@const isOpen = expanded[group.kind] ?? false}
			<li data-kind={group.kind}>
				<button
					type="button"
					class="flex w-full items-center gap-2 px-4 py-2 text-left hover:bg-bg-canvas dark:hover:bg-dark-bg-canvas"
					aria-expanded={isOpen}
					data-testid={`integrity-group-${group.kind}`}
					onclick={() => toggle(group.kind)}
				>
					{#if isOpen}
						<ChevronDown class="h-4 w-4" aria-hidden="true" />
					{:else}
						<ChevronRight class="h-4 w-4" aria-hidden="true" />
					{/if}
					<span class="font-medium">{labelFor(group.kind)}</span>
					<span class="text-fg-muted dark:text-dark-fg-muted">({group.issues.length})</span>
				</button>
				{#if isOpen}
					<ul class="bg-bg-canvas px-4 pb-3 pt-1 dark:bg-dark-bg-canvas">
						{#each group.issues as issue, idx (`${group.kind}-${idx}`)}
							{@const repairStrategy = strategyFor(issue)}
							{@const issueKey = issue.issueId ?? ''}
							<li class="mt-2 flex flex-col gap-1 rounded-none bg-bg-elevated p-2 dark:bg-dark-bg-elevated">
								<div class="flex flex-wrap items-center gap-2 font-mono text-xs">
									{#if issue.path}
										{@const href = docLink(issue.path)}
										{#if href}
											<a
												href={href}
												data-testid="integrity-path-link"
												class="text-accent-lever underline hover:text-accent-lever/80 dark:text-dark-accent-lever dark:hover:text-dark-accent-lever/80"
												>{issue.path}</a
											>
										{:else}
											<span>{issue.path}</span>
										{/if}
									{/if}
									{#if issue.relatedPath}
										{@const relHref = docLink(issue.relatedPath)}
										<span class="text-fg-muted dark:text-dark-fg-muted">↔</span>
										{#if relHref}
											<a
												href={relHref}
												data-testid="integrity-related-link"
												class="text-accent-lever underline hover:text-accent-lever/80 dark:text-dark-accent-lever dark:hover:text-dark-accent-lever/80"
												>{issue.relatedPath}</a
											>
										{:else}
											<span>{issue.relatedPath}</span>
										{/if}
									{/if}
									{#if issue.id}
										<span
											class="rounded-none border px-1.5 py-0.5 text-[10px] uppercase badge-status-blocked"
											>{issue.id}</span
										>
									{/if}
								</div>
								<div class="flex items-start justify-between gap-2">
									<p class="break-words">{issue.message}</p>
									<button
										type="button"
										class="shrink-0 rounded-none border border-border-line bg-bg-elevated px-2 py-1 text-xs hover:bg-bg-surface dark:border-dark-border-line dark:bg-dark-bg-surface dark:hover:bg-dark-bg-canvas"
										title="Copy message"
										aria-label="Copy message"
										onclick={(e) => copyMessage(issue, e)}
									>
										<Copy class="h-3 w-3" aria-hidden="true" />
									</button>
								</div>
								{#if group.kind === 'duplicate_id'}
									<button
										type="button"
										data-testid="integrity-copy-suggestion"
										class="self-start rounded-none border px-2 py-1 text-xs font-medium badge-status-blocked hover:opacity-90"
										onclick={(e) => copySuggestion(issue, e)}
									>
										Copy suggested unique ID
									</button>
								{/if}
								{#if repairStrategy}
									<div class="flex flex-wrap items-center gap-2 text-xs">
										<button
											type="button"
											data-testid="integrity-preview-fix"
											class="rounded-none border border-border-line bg-bg-elevated px-2 py-1 hover:bg-bg-surface dark:border-dark-border-line dark:bg-dark-bg-surface dark:hover:bg-dark-bg-canvas"
											onclick={() => togglePreview(issue)}
										>
											{previewing[issueKey] ? 'Hide preview' : 'Preview fix'}
										</button>
										<button
											type="button"
											data-testid="integrity-apply-fix"
											disabled={pending[issueKey]}
											class="rounded-none border px-2 py-1 font-medium badge-status-blocked hover:opacity-90 disabled:opacity-50"
											onclick={() => applyFix(issue)}
										>
											{pending[issueKey] ? 'Applying…' : 'Apply fix'}
										</button>
									</div>
									{#if previewing[issueKey]}
										{@const diff =
											repairStrategy === 'REMOVE_MISSING_DEP'
												? diffRemoveDep(issue)
												: diffApplySuggestedID(issue, previewedSuggestions[issueKey] ?? '')}
										<div
											data-testid="integrity-preview-diff"
											class="mt-1 grid gap-1 rounded-none border border-border-line bg-bg-canvas p-2 font-mono text-xs dark:border-dark-border-line dark:bg-dark-bg-canvas"
										>
											<div class="text-fg-muted dark:text-dark-fg-muted">Before</div>
											<pre class="overflow-x-auto whitespace-pre-wrap text-status-blocked">{diff.before}</pre>
											<div class="text-fg-muted dark:text-dark-fg-muted">After</div>
											<pre class="overflow-x-auto whitespace-pre-wrap text-status-merged">{diff.after}</pre>
										</div>
									{/if}
									{#if inlineErrors[issueKey]}
										<p
											data-testid="integrity-repair-error"
											class="text-xs text-status-blocked"
										>
											{inlineErrors[issueKey]}
										</p>
									{/if}
								{/if}
								{#if group.kind === 'missing_dep' && issue.id}
									{@const snippet = dependencyRemovalSnippet(issue)}
									{#if snippet}
										<div class="flex flex-wrap items-center gap-2 text-xs">
											<span class="font-medium text-fg-muted dark:text-dark-fg-muted">
												Remove from depends_on
											</span>
											<code
												data-testid="integrity-missing-dep-snippet"
												class="rounded-none border px-2 py-1 font-mono-code badge-status-blocked"
												>{snippet}</code
											>
											<button
												type="button"
												class="rounded-none border border-border-line bg-bg-elevated px-2 py-1 hover:bg-bg-surface dark:border-dark-border-line dark:bg-dark-bg-surface dark:hover:bg-dark-bg-canvas"
												title="Copy removal snippet"
												aria-label="Copy missing dependency removal snippet"
												onclick={(e) => copyDependencyRemovalSnippet(issue, e)}
											>
												<Copy class="h-3 w-3" aria-hidden="true" />
											</button>
										</div>
									{/if}
								{/if}
							</li>
						{/each}
					</ul>
				{/if}
			</li>
		{/each}
	</ul>
</section>
