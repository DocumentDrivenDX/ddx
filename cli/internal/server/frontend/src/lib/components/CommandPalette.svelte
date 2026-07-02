<script lang="ts">
	import { goto, invalidateAll } from '$app/navigation';
	import { page } from '$app/stores';
	import { createClient } from '$lib/gql/client';
	import { nodeStore } from '$lib/stores/node.svelte';
	import { projectStore } from '$lib/stores/project.svelte';
	import { Command } from 'bits-ui';
	import { gql } from 'graphql-request';
	import {
		CheckCircle2,
		FileText,
		FolderKanban,
		Navigation,
		Play,
		RefreshCcw,
		Search,
		Trash2,
		UserCheck,
		UserMinus,
		XCircle
	} from 'lucide-svelte';
	import { onMount, tick } from 'svelte';

	const PALETTE_SEARCH = gql`
		query PaletteSearch($query: String!) {
			paletteSearch(query: $query) {
				documents {
					kind
					path
					title
				}
				beads {
					kind
					id
					title
				}
				actions {
					kind
					id
					label
				}
				navigation {
					kind
					route
					title
				}
			}
		}
	`;

	const BEAD_CLAIM = gql`
		mutation PaletteBeadClaim($id: ID!, $assignee: String!) {
			beadClaim(id: $id, assignee: $assignee) {
				id
				status
			}
		}
	`;

	const BEAD_UNCLAIM = gql`
		mutation PaletteBeadUnclaim($id: ID!) {
			beadUnclaim(id: $id) {
				id
				status
			}
		}
	`;

	const BEAD_CLOSE = gql`
		mutation PaletteBeadClose($id: ID!, $reason: String) {
			beadClose(id: $id, reason: $reason) {
				id
				status
			}
		}
	`;

	const BEAD_REOPEN = gql`
		mutation PaletteBeadReopen($id: ID!) {
			beadReopen(id: $id) {
				id
				status
			}
		}
	`;

	const WORKER_DISPATCH = gql`
		mutation PaletteWorkerDispatch($projectId: String!, $args: String) {
			workerDispatch(kind: "execute-loop", projectId: $projectId, args: $args) {
				id
				state
			}
		}
	`;

	interface PaletteDocumentResult {
		kind: 'document';
		path: string;
		title: string;
	}

	interface PaletteBeadResult {
		kind: 'bead';
		id: string;
		title: string;
	}

	interface PaletteActionResult {
		kind: 'action';
		id: string;
		label: string;
	}

	interface PaletteNavigationResult {
		kind: 'nav';
		route: string;
		title: string;
	}

	interface PaletteSearchResults {
		documents: PaletteDocumentResult[];
		beads: PaletteBeadResult[];
		actions: PaletteActionResult[];
		navigation: PaletteNavigationResult[];
	}

	interface ProjectRouteContext {
		nodeId: string;
		projectId: string;
	}

	interface BeadRouteContext extends ProjectRouteContext {
		beadId: string;
	}

	type PaletteEntryKind = 'bead-action' | 'document' | 'bead' | 'action' | 'navigation';
	type IconComponent = typeof Search;

	interface PaletteEntry {
		id: string;
		kind: PaletteEntryKind;
		label: string;
		detail: string;
		route?: string;
		action?: string;
		Icon: IconComponent;
	}

	const EMPTY_RESULTS: PaletteSearchResults = {
		documents: [],
		beads: [],
		actions: [],
		navigation: []
	};

	const client = createClient();
	const OPEN_EVENT = 'ddx-command-palette-open';
	const CLOSE_EVENT = 'ddx-command-palette-close';

	let open = $state(false);
	let query = $state('');
	let selectedValue = $state('');
	let results = $state<PaletteSearchResults>(EMPTY_RESULTS);
	let loading = $state(false);
	let errorMessage = $state('');
	let dialogElement: HTMLDialogElement;
	let inputElement = $state<HTMLInputElement | null>(null);
	let searchSequence = 0;

	const projectContext = $derived(parseProjectRoute($page.url.pathname));
	const beadContext = $derived(parseBeadRoute($page.url.pathname));
	const nodeId = $derived(projectContext?.nodeId ?? nodeStore.value?.id ?? null);
	const projectId = $derived(projectContext?.projectId ?? projectStore.value?.id ?? null);
	const allEntries = $derived([...beadActionEntries(beadContext), ...searchResultEntries(results)]);

	$effect(() => {
		if (!dialogElement) return;

		if (open && !dialogElement.open) {
			dialogElement.showModal();
			void tick().then(() => inputElement?.focus());
		}

		if (!open && dialogElement.open) {
			dialogElement.close();
		}
	});

	$effect(() => {
		if (!open) return;

		const sequence = ++searchSequence;
		const searchQuery = query.trim();
		loading = true;
		errorMessage = '';

		const timer = window.setTimeout(() => {
			void runSearch(searchQuery, sequence);
		}, 200);

		return () => window.clearTimeout(timer);
	});

	onMount(() => {
		const paletteWindow = window as Window & { __ddxCommandPalettePendingOpen?: boolean };
		const handleBufferedOpen = () => {
			paletteWindow.__ddxCommandPalettePendingOpen = false;
			openPalette();
		};
		const handleBufferedClose = () => closePalette();

		window.addEventListener(OPEN_EVENT, handleBufferedOpen);
		window.addEventListener(CLOSE_EVENT, handleBufferedClose);

		if (paletteWindow.__ddxCommandPalettePendingOpen) {
			handleBufferedOpen();
		}

		return () => {
			window.removeEventListener(OPEN_EVENT, handleBufferedOpen);
			window.removeEventListener(CLOSE_EVENT, handleBufferedClose);
		};
	});

	function handleKeydown(event: KeyboardEvent) {
		if ((event.metaKey || event.ctrlKey) && event.key.toLowerCase() === 'k') {
			event.preventDefault();
			openPalette();
			return;
		}

		if (event.key === 'Escape' && open) {
			event.preventDefault();
			closePalette();
		}
	}

	function openPalette() {
		query = '';
		selectedValue = '';
		results = EMPTY_RESULTS;
		errorMessage = '';
		open = true;
	}

	function closePalette() {
		open = false;
	}

	function handleDialogClose() {
		open = false;
	}

	function handleDialogCancel(event: Event) {
		event.preventDefault();
		closePalette();
	}

	async function runSearch(searchQuery: string, sequence: number) {
		try {
			const data = await client.request<{ paletteSearch: PaletteSearchResults }>(PALETTE_SEARCH, {
				query: searchQuery
			});
			if (sequence === searchSequence) {
				results = data.paletteSearch;
			}
		} catch (error) {
			if (sequence === searchSequence) {
				results = EMPTY_RESULTS;
				errorMessage = error instanceof Error ? error.message : 'Palette search failed.';
			}
		} finally {
			if (sequence === searchSequence) {
				loading = false;
			}
		}
	}

	function parseProjectRoute(pathname: string): ProjectRouteContext | null {
		const match = pathname.match(/^\/nodes\/([^/]+)\/projects\/([^/]+)(?:\/|$)/);
		if (!match) return null;
		return {
			nodeId: decodeURIComponent(match[1]),
			projectId: decodeURIComponent(match[2])
		};
	}

	function parseBeadRoute(pathname: string): BeadRouteContext | null {
		const match = pathname.match(/^\/nodes\/([^/]+)\/projects\/([^/]+)\/beads\/([^/]+)$/);
		if (!match) return null;
		return {
			nodeId: decodeURIComponent(match[1]),
			projectId: decodeURIComponent(match[2]),
			beadId: decodeURIComponent(match[3])
		};
	}

	function beadActionEntries(context: BeadRouteContext | null): PaletteEntry[] {
		if (!context) return [];

		return [
			{
				id: `bead-action:${context.beadId}:claim`,
				kind: 'bead-action',
				label: 'Claim',
				detail: context.beadId,
				action: 'claim',
				Icon: UserCheck
			},
			{
				id: `bead-action:${context.beadId}:unclaim`,
				kind: 'bead-action',
				label: 'Unclaim',
				detail: context.beadId,
				action: 'unclaim',
				Icon: UserMinus
			},
			{
				id: `bead-action:${context.beadId}:close`,
				kind: 'bead-action',
				label: 'Close',
				detail: context.beadId,
				action: 'close',
				Icon: CheckCircle2
			},
			{
				id: `bead-action:${context.beadId}:reopen`,
				kind: 'bead-action',
				label: 'Reopen',
				detail: context.beadId,
				action: 'reopen',
				Icon: RefreshCcw
			},
			{
				id: `bead-action:${context.beadId}:rerun`,
				kind: 'bead-action',
				label: 'Re-run',
				detail: context.beadId,
				action: 'rerun',
				Icon: Play
			},
			{
				id: `bead-action:${context.beadId}:delete`,
				kind: 'bead-action',
				label: 'Delete',
				detail: context.beadId,
				action: 'delete',
				Icon: Trash2
			}
		];
	}

	function searchResultEntries(searchResults: PaletteSearchResults): PaletteEntry[] {
		const entries: PaletteEntry[] = [];

		for (const documentResult of searchResults.documents) {
			entries.push({
				id: `document:${documentResult.path}`,
				kind: 'document',
				label: documentResult.title,
				detail: documentResult.path,
				route: projectRoute(`/documents/${documentPath(documentResult.path)}`),
				Icon: FileText
			});
		}

		for (const beadResult of searchResults.beads) {
			entries.push({
				id: `bead:${beadResult.id}`,
				kind: 'bead',
				label: beadResult.title,
				detail: beadResult.id,
				route: projectRoute(`/beads/${encodeURIComponent(beadResult.id)}`),
				Icon: FolderKanban
			});
		}

		for (const actionResult of searchResults.actions) {
			entries.push({
				id: `action:${actionResult.id}`,
				kind: 'action',
				label: actionResult.label,
				detail: actionResult.id,
				action: actionResult.id,
				Icon: Play
			});
		}

		for (const navigationResult of searchResults.navigation) {
			entries.push({
				id: `navigation:${navigationResult.route}:${navigationResult.title}`,
				kind: 'navigation',
				label: navigationResult.title,
				detail: normalizeNavigationRoute(navigationResult.route),
				route: normalizeNavigationRoute(navigationResult.route),
				Icon: Navigation
			});
		}

		return entries;
	}

	function documentPath(path: string) {
		return path
			.split('/')
			.filter(Boolean)
			.map((segment) => encodeURIComponent(segment))
			.join('/');
	}

	function projectRoute(suffix: string): string | undefined {
		if (!nodeId || !projectId) return undefined;
		return `/nodes/${encodeURIComponent(nodeId)}/projects/${encodeURIComponent(projectId)}${suffix}`;
	}

	function normalizeNavigationRoute(route: string): string {
		if (/^https?:\/\//.test(route)) {
			return new URL(route).pathname;
		}

		if (route.startsWith('/nodes/')) {
			return route;
		}

		if (route.startsWith('/') && nodeId && projectId) {
			return `/nodes/${encodeURIComponent(nodeId)}/projects/${encodeURIComponent(projectId)}${route}`;
		}

		return route;
	}

	async function activateEntry(entry: PaletteEntry) {
		if (!open) return;

		if (entry.route) {
			closePalette();
			// Navigation entries can come from the GraphQL API, so they are not always route IDs.
			// eslint-disable-next-line svelte/no-navigation-without-resolve
			await goto(entry.route);
			return;
		}

		if (entry.kind === 'bead-action' && beadContext) {
			await runBeadAction(entry.action ?? '', beadContext);
			return;
		}

		closePalette();
	}

	async function runBeadAction(action: string, context: BeadRouteContext) {
		errorMessage = '';

		try {
			if (action === 'claim') {
				await client.request(BEAD_CLAIM, { id: context.beadId, assignee: 'web-ui' });
			} else if (action === 'unclaim') {
				await client.request(BEAD_UNCLAIM, { id: context.beadId });
			} else if (action === 'close') {
				await client.request(BEAD_CLOSE, {
					id: context.beadId,
					reason: 'closed from command palette'
				});
			} else if (action === 'reopen') {
				await client.request(BEAD_REOPEN, { id: context.beadId });
			} else if (action === 'rerun') {
				await client.request(WORKER_DISPATCH, {
					projectId: context.projectId,
					args: JSON.stringify({ beadId: context.beadId })
				});
			} else if (action === 'delete') {
				throw new Error('Delete is not available from the GraphQL API yet.');
			}

			closePalette();
			await invalidateAll();
		} catch (error) {
			errorMessage = error instanceof Error ? error.message : 'Command failed.';
		}
	}
</script>

<svelte:window onkeydown={handleKeydown} />

<dialog
	bind:this={dialogElement}
	aria-label="Command palette"
	class="m-auto w-[min(680px,calc(100vw-2rem))] overflow-hidden rounded-none border border-border-line bg-bg-elevated p-0 shadow-2xl shadow-fg-ink/20 backdrop:bg-fg-ink/45 dark:border-dark-border-line dark:bg-dark-bg-elevated dark:shadow-black/50"
	onclose={handleDialogClose}
	oncancel={handleDialogCancel}
>
	<Command.Root
		bind:value={selectedValue}
		shouldFilter={false}
		label="Command palette"
		class="flex max-h-[min(720px,calc(100vh-4rem))] flex-col"
	>
		<div class="flex items-center gap-3 border-b border-border-line px-4 py-3 dark:border-dark-border-line">
			<Search class="h-4 w-4 shrink-0 text-fg-muted dark:text-dark-fg-muted" />
			<input
				bind:value={query}
				bind:this={inputElement}
				role="searchbox"
				aria-label="Command palette"
				class="min-w-0 flex-1 rounded-none border-0 bg-transparent px-2 py-1 text-sm text-fg-ink outline-none placeholder:text-fg-muted focus:ring-1 focus:ring-accent-lever dark:text-dark-fg-ink dark:placeholder:text-dark-fg-muted dark:focus:ring-dark-accent-lever"
				placeholder="Search beads, docs, actions..."
			/>
			<kbd
				class="rounded-none border border-border-line bg-bg-surface px-1.5 py-0.5 text-[11px] font-medium text-fg-muted dark:border-dark-border-line dark:bg-dark-bg-surface dark:text-dark-fg-muted"
				>Esc</kbd
			>
		</div>

		<Command.List
			role="listbox"
			aria-label="Command palette results"
			class="max-h-[560px] overflow-y-auto px-2 pb-2"
		>
			<Command.Viewport>
				{#if errorMessage}
					<div class="flex items-center gap-2 px-3 py-4 text-sm text-error dark:text-dark-error">
						<XCircle class="h-4 w-4 shrink-0" />
						{errorMessage}
					</div>
				{:else if loading && allEntries.length === 0}
					<div class="px-3 py-4 text-sm text-fg-muted dark:text-dark-fg-muted">Searching...</div>
				{:else if allEntries.length === 0}
					<div class="px-3 py-4 text-sm text-fg-muted dark:text-dark-fg-muted">No commands found.</div>
				{:else}
					<Command.Group>
						<Command.GroupItems class="py-2">
							<!-- Hover/selected depth: container is bg-bg-elevated; both hover and data-[selected] use bg-bg-canvas, matching the sidebar nav pattern in NavShell.svelte (canvas as the single highlight tone over the surface/elevated container). -->
							{#each allEntries as entry (entry.id)}
								<Command.Item
									role="option"
									aria-label={entry.action === 'unclaim' ? `Release ${entry.detail}` : undefined}
									value={entry.id}
									onSelect={() => void activateEntry(entry)}
									onclick={() => void activateEntry(entry)}
									class="flex min-h-11 cursor-pointer items-center gap-3 rounded-none px-3 py-2 text-left text-sm outline-none select-none hover:bg-bg-canvas data-[selected]:bg-bg-canvas data-[selected]:text-fg-ink dark:hover:bg-dark-bg-canvas dark:data-[selected]:bg-dark-bg-canvas dark:data-[selected]:text-dark-fg-ink"
								>
									<entry.Icon class="h-4 w-4 shrink-0 text-fg-muted dark:text-dark-fg-muted" />
									<span class="min-w-0 flex-1">
										<span class="block truncate font-medium text-fg-ink dark:text-dark-fg-ink"
											>{entry.label}</span
										>
										<span class="block truncate text-xs text-fg-muted dark:text-dark-fg-muted"
											>{entry.detail}</span
										>
									</span>
								</Command.Item>
							{/each}
						</Command.GroupItems>
					</Command.Group>
				{/if}
			</Command.Viewport>
		</Command.List>
	</Command.Root>
</dialog>
