<script lang="ts">
	import { page } from '$app/stores';
	import { ChevronDown, ChevronRight, Copy } from 'lucide-svelte';

	export interface GraphIssue {
		kind: string;
		path: string | null;
		id: string | null;
		message: string;
		relatedPath: string | null;
	}

	let { issues }: { issues: GraphIssue[] } = $props();

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
</script>

<section
	data-testid="integrity-panel"
	class="shrink-0 rounded border border-amber-200 bg-amber-50 text-sm text-amber-900 dark:border-amber-800 dark:bg-amber-950 dark:text-amber-100"
>
	<header class="border-b border-amber-200 px-4 py-2 font-semibold dark:border-amber-800">
		Integrity: {issues.length}
		{issues.length === 1 ? 'issue' : 'issues'}
	</header>
	<ul class="divide-y divide-amber-200 dark:divide-amber-800">
		{#each groups as group (group.kind)}
			{@const isOpen = expanded[group.kind] ?? false}
			<li data-kind={group.kind}>
				<button
					type="button"
					class="flex w-full items-center gap-2 px-4 py-2 text-left hover:bg-amber-100 dark:hover:bg-amber-900"
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
					<span class="text-amber-800 dark:text-amber-200">({group.issues.length})</span>
				</button>
				{#if isOpen}
					<ul class="bg-amber-100/50 px-4 pb-3 pt-1 dark:bg-amber-900/40">
						{#each group.issues as issue, idx (`${group.kind}-${idx}`)}
							<li class="mt-2 flex flex-col gap-1 rounded bg-white/60 p-2 dark:bg-amber-950/40">
								<div class="flex flex-wrap items-center gap-2 font-mono text-xs">
									{#if issue.path}
										{@const href = docLink(issue.path)}
										{#if href}
											<a
												href={href}
												data-testid="integrity-path-link"
												class="text-amber-900 underline hover:text-amber-700 dark:text-amber-100 dark:hover:text-amber-300"
												>{issue.path}</a
											>
										{:else}
											<span>{issue.path}</span>
										{/if}
									{/if}
									{#if issue.relatedPath}
										{@const relHref = docLink(issue.relatedPath)}
										<span class="text-amber-700 dark:text-amber-300">↔</span>
										{#if relHref}
											<a
												href={relHref}
												data-testid="integrity-related-link"
												class="text-amber-900 underline hover:text-amber-700 dark:text-amber-100 dark:hover:text-amber-300"
												>{issue.relatedPath}</a
											>
										{:else}
											<span>{issue.relatedPath}</span>
										{/if}
									{/if}
									{#if issue.id}
										<span
											class="rounded bg-amber-200 px-1.5 py-0.5 text-[10px] uppercase dark:bg-amber-800"
											>{issue.id}</span
										>
									{/if}
								</div>
								<div class="flex items-start justify-between gap-2">
									<p class="break-words">{issue.message}</p>
									<button
										type="button"
										class="shrink-0 rounded border border-amber-300 bg-white px-2 py-1 text-xs hover:bg-amber-50 dark:border-amber-700 dark:bg-amber-900 dark:hover:bg-amber-800"
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
										class="self-start rounded bg-amber-600 px-2 py-1 text-xs font-medium text-white hover:bg-amber-700"
										onclick={(e) => copySuggestion(issue, e)}
									>
										Copy suggested unique ID
									</button>
								{/if}
							</li>
						{/each}
					</ul>
				{/if}
			</li>
		{/each}
	</ul>
</section>
