<script lang="ts">
	import { resolve } from '$app/paths';
	import { invalidateAll } from '$app/navigation';
	import { createClient } from '$lib/gql/client';
	import { gql } from 'graphql-request';
	import DOMPurify from 'isomorphic-dompurify';
	import { marked } from 'marked';
	import type { PageData } from './$types';
	import type { PersonaNode, ProjectOption } from './data';
	import {
		PERSONA_CREATE_MUTATION,
		PERSONA_UPDATE_MUTATION,
		PERSONA_DELETE_MUTATION,
		PERSONA_FORK_MUTATION
	} from './data';

	let { data }: { data: PageData } = $props();

	const PROJECT_BINDINGS_QUERY = gql`
		query ProjectBindings($projectId: String!) {
			projectBindings(projectId: $projectId)
		}
	`;

	const PERSONA_BIND_MUTATION = gql`
		mutation PersonaBind($role: String!, $persona: String!, $projectId: String!) {
			personaBind(role: $role, persona: $persona, projectId: $projectId) {
				ok
				role
				persona
			}
		}
	`;

	type ProjectBindingsResult = {
		projectBindings: unknown;
	};

	// Binding dialog state.
	let bindOpen = $state(false);
	let bindRole = $state('');
	let bindProjectId = $state('');
	let bindingsByRole = $state<Record<string, string>>({});
	let warning = $state('');
	let bindError = $state('');
	let status = $state('');
	let needsConfirm = $state(false);
	let loadingBindings = $state(false);
	let savingBinding = $state(false);

	// Editor dialog state.
	let editorOpen = $state(false);
	let editorMode = $state<'create' | 'edit'>('create');
	let editorName = $state('');
	let editorBody = $state('');
	let editorError = $state('');
	let editorSaving = $state(false);

	// Delete confirmation state.
	let deleteTarget = $state<string | null>(null);
	let deleteError = $state('');

	const selected = $derived.by<PersonaNode | null>(() => {
		if (!data.selectedName) return null;
		return data.personas.find((p) => p.name === data.selectedName) ?? null;
	});

	const hasProjectPersonas = $derived.by(() =>
		data.personas.some((p) => p.source === 'project')
	);

	const projects = $derived.by<ProjectOption[]>(() => {
		if ('projects' in data && Array.isArray(data.projects) && data.projects.length > 0) {
			return data.projects as ProjectOption[];
		}
		if ('project' in data && data.project) {
			return [data.project as ProjectOption];
		}
		return [{ id: data.projectId, name: data.projectId, path: '' }];
	});

	const renderedBody = $derived.by(() => {
		if (!selected) return '';
		return DOMPurify.sanitize(marked.parse(bodyWithoutLeadingHeading(selected.body)) as string);
	});

	function displayName(name: string): string {
		return name
			.split(/[-_\s]+/)
			.filter(Boolean)
			.map((part) => part.charAt(0).toUpperCase() + part.slice(1))
			.join(' ');
	}

	function bodyWithoutLeadingHeading(body: string): string {
		return body.replace(/^#\s+.+(?:\r?\n)+/, '');
	}

	function parseProjectBindings(value: unknown): Record<string, string> {
		if (typeof value === 'string') {
			try {
				const parsed = JSON.parse(value) as unknown;
				return parseProjectBindings(parsed);
			} catch {
				return {};
			}
		}
		if (!value || typeof value !== 'object' || Array.isArray(value)) return {};
		const out: Record<string, string> = {};
		for (const [role, persona] of Object.entries(value)) {
			if (typeof persona === 'string') out[role] = persona;
		}
		return out;
	}

	async function readProjectBindings(projectId: string): Promise<void> {
		loadingBindings = true;
		bindError = '';
		try {
			const client = createClient(fetch);
			const result = await client.request<ProjectBindingsResult>(PROJECT_BINDINGS_QUERY, {
				projectId
			});
			bindingsByRole = parseProjectBindings(result.projectBindings);
		} catch (err) {
			bindingsByRole = {};
			bindError = err instanceof Error ? err.message : 'Unable to read current bindings.';
		} finally {
			loadingBindings = false;
		}
	}

	async function openBindDialog(): Promise<void> {
		if (!selected) return;
		bindRole = selected.roles[0] ?? '';
		bindProjectId = data.projectId;
		warning = '';
		bindError = '';
		needsConfirm = false;
		await readProjectBindings(bindProjectId);
		bindOpen = true;
	}

	async function onProjectChange(event: Event): Promise<void> {
		bindProjectId = (event.currentTarget as HTMLSelectElement).value;
		warning = '';
		needsConfirm = false;
		await readProjectBindings(bindProjectId);
	}

	async function submitBind(confirm = false): Promise<void> {
		if (!selected || !bindRole || !bindProjectId) return;
		const existing = bindingsByRole[bindRole];
		if (existing && !confirm) {
			warning = `${bindRole} is already bound to ${existing}. Confirm to replace it with ${selected.name}.`;
			needsConfirm = true;
			return;
		}

		savingBinding = true;
		warning = '';
		bindError = '';
		try {
			const client = createClient(fetch);
			await client.request(PERSONA_BIND_MUTATION, {
				role: bindRole,
				persona: selected.name,
				projectId: bindProjectId
			});
			bindingsByRole = { ...bindingsByRole, [bindRole]: selected.name };
			status = `${selected.name} bound to ${bindRole}.`;
			bindOpen = false;
			needsConfirm = false;
		} catch (err) {
			bindError = err instanceof Error ? err.message : 'Unable to save binding.';
		} finally {
			savingBinding = false;
		}
	}

	// ── Project-local persona lifecycle ────────────────────────────────────

	function scaffoldBody(name: string): string {
		return `---\nname: ${name}\nroles: [general]\ndescription: Project persona ${name}\ntags: []\n---\n\n# ${name}\n\nTODO: describe what this persona does.\n`;
	}

	function openNewEditor(): void {
		editorMode = 'create';
		editorName = '';
		editorBody = scaffoldBody('new-persona');
		editorError = '';
		editorOpen = true;
	}

	function openEditEditor(persona: PersonaNode): void {
		editorMode = 'edit';
		editorName = persona.name;
		editorBody = persona.body ?? '';
		editorError = '';
		editorOpen = true;
	}

	async function submitEditor(): Promise<void> {
		editorError = '';
		editorSaving = true;
		try {
			const client = createClient(fetch);
			if (editorMode === 'create') {
				await client.request(PERSONA_CREATE_MUTATION, {
					name: editorName,
					body: editorBody,
					projectId: data.projectId
				});
				status = `Created project persona '${editorName}'.`;
			} else {
				await client.request(PERSONA_UPDATE_MUTATION, {
					name: editorName,
					body: editorBody,
					projectId: data.projectId
				});
				status = `Updated project persona '${editorName}'.`;
			}
			editorOpen = false;
			await invalidateAll();
		} catch (err) {
			editorError = err instanceof Error ? err.message : 'Unable to save persona.';
		} finally {
			editorSaving = false;
		}
	}

	function confirmDelete(name: string): void {
		deleteTarget = name;
		deleteError = '';
	}

	async function runDelete(): Promise<void> {
		if (!deleteTarget) return;
		try {
			const client = createClient(fetch);
			await client.request(PERSONA_DELETE_MUTATION, {
				name: deleteTarget,
				projectId: data.projectId
			});
			status = `Deleted project persona '${deleteTarget}'.`;
			deleteTarget = null;
			await invalidateAll();
		} catch (err) {
			deleteError = err instanceof Error ? err.message : 'Unable to delete persona.';
		}
	}

	async function forkLibrary(persona: PersonaNode): Promise<void> {
		const existingNames = new Set(data.personas.filter((p) => p.source === 'project').map((p) => p.name));
		let target = persona.name;
		if (existingNames.has(target)) {
			target = `${persona.name}-local`;
		}
		const entered = typeof window !== 'undefined' ? window.prompt(`Name for the forked persona?`, target) : target;
		if (!entered) return;
		try {
			const client = createClient(fetch);
			await client.request(PERSONA_FORK_MUTATION, {
				libraryName: persona.name,
				newName: entered,
				projectId: data.projectId
			});
			status = `Forked '${persona.name}' to '${entered}'.`;
			await invalidateAll();
			// Navigate to the newly forked persona's editor.
			editorMode = 'edit';
			editorName = entered;
			editorBody = (data.personas.find((p) => p.name === entered)?.body) ?? persona.body ?? '';
			editorError = '';
			editorOpen = true;
		} catch (err) {
			status = err instanceof Error ? err.message : 'Unable to fork persona.';
		}
	}
</script>

<div class="min-h-full bg-stone-50 text-zinc-950 dark:bg-zinc-950 dark:text-zinc-50">
	<div class="mx-auto flex max-w-7xl flex-col gap-6 px-5 py-6 lg:px-8">
		<header
			class="flex flex-col gap-3 border-b border-zinc-200 pb-5 md:flex-row md:items-end md:justify-between dark:border-zinc-800"
		>
			<div>
				<p
					class="text-xs font-semibold tracking-[0.18em] text-teal-700 uppercase dark:text-teal-300"
				>
					Persona Library
				</p>
				<h1 class="mt-1 text-3xl font-semibold tracking-tight">Personas</h1>
				<p
					class="mt-3 max-w-2xl text-sm leading-6 text-zinc-600 dark:text-zinc-300"
					data-testid="personas-explainer"
				>
					Personas are AI personality templates that get injected into agent prompts when
					a persona is bound to a role.
					Library personas are shared across projects and are read-only; project personas
					live with this project under <code>.ddx/personas</code> and are fully editable.
				</p>
			</div>
			<div class="flex flex-col items-end gap-2 text-sm text-zinc-600 dark:text-zinc-300">
				<button
					type="button"
					data-testid="persona-new-button"
					class="inline-flex items-center justify-center rounded-md bg-teal-700 px-4 py-2 text-sm font-semibold text-white shadow-sm transition hover:bg-teal-800 focus:ring-2 focus:ring-teal-500 focus:ring-offset-2 focus:outline-none dark:bg-teal-300 dark:text-zinc-950 dark:hover:bg-teal-200"
					onclick={openNewEditor}
				>
					New persona
				</button>
				<span>
					{data.personas.length}
					{data.personas.length === 1 ? 'persona' : 'personas'}
				</span>
			</div>
		</header>

		{#if status}
			<div
				role="status"
				class="rounded border border-emerald-300 bg-emerald-50 px-4 py-3 text-sm font-medium text-emerald-900 dark:border-emerald-800 dark:bg-emerald-950 dark:text-emerald-100"
			>
				{status}
			</div>
		{/if}

		{#if !hasProjectPersonas}
			<div
				data-testid="no-project-personas-hint"
				class="rounded-md border border-dashed border-zinc-300 bg-white px-4 py-3 text-sm text-zinc-600 dark:border-zinc-700 dark:bg-zinc-900 dark:text-zinc-300"
			>
				No project personas yet. Fork a library persona or create a new one.
			</div>
		{/if}

		<div class="grid gap-6 xl:grid-cols-[minmax(280px,380px)_1fr]">
			<section aria-label="Installed personas" class="grid gap-3 self-start">
				{#each data.personas as persona (persona.name)}
					<article
						aria-label={persona.name}
						data-testid={`persona-row-${persona.name}`}
						class="group relative rounded-md border border-zinc-200 bg-white p-4 shadow-sm transition hover:-translate-y-0.5 hover:border-teal-500 hover:shadow-md dark:border-zinc-800 dark:bg-zinc-900 dark:hover:border-teal-400 {selected?.name ===
						persona.name
							? 'border-teal-600 ring-1 ring-teal-600 dark:border-teal-300 dark:ring-teal-300'
							: ''}"
					>
						<a
							class="absolute inset-0 rounded-md"
							href={resolve(
								`/nodes/${data.nodeInfo.id}/projects/${data.projectId}/personas/${encodeURIComponent(persona.name)}`
							)}
						>
							<span class="sr-only">Open persona details</span>
						</a>
						<div class="flex items-start justify-between gap-3">
							<div>
								<h2
									id={`persona-card-${persona.name}`}
									class="text-base font-semibold text-zinc-950 dark:text-zinc-50"
								>
									{displayName(persona.name)}
								</h2>
								<p class="mt-1 text-sm leading-6 text-zinc-600 dark:text-zinc-300">
									{persona.description}
								</p>
							</div>
							{#if persona.source}
								<span
									data-testid={`persona-source-${persona.name}`}
									class="shrink-0 rounded border px-2 py-1 text-[11px] font-medium {persona.source === 'project'
										? 'border-teal-400 bg-teal-50 text-teal-800 dark:border-teal-500 dark:bg-teal-950 dark:text-teal-200'
										: 'border-zinc-200 text-zinc-600 dark:border-zinc-700 dark:text-zinc-300'}"
								>
									{persona.source}
								</span>
							{/if}
						</div>
						{#if persona.roles.length > 0}
							<div class="mt-4 flex flex-wrap gap-2">
								{#each persona.roles as role (role)}
									<span
										class="rounded bg-teal-50 px-2 py-1 text-xs font-medium text-teal-800 dark:bg-teal-950 dark:text-teal-200"
									>
										{role}
									</span>
								{/each}
							</div>
						{/if}

						<div class="relative z-10 mt-3 flex flex-wrap gap-2">
							{#if persona.source === 'project'}
								<button
									type="button"
									data-testid={`persona-edit-${persona.name}`}
									class="rounded border border-zinc-300 px-2 py-1 text-xs font-medium hover:bg-zinc-100 dark:border-zinc-700 dark:hover:bg-zinc-800"
									onclick={(event) => {
										event.stopPropagation();
										openEditEditor(persona);
									}}
								>
									Edit
								</button>
								<button
									type="button"
									data-testid={`persona-delete-${persona.name}`}
									class="rounded border border-red-300 px-2 py-1 text-xs font-medium text-red-700 hover:bg-red-50 dark:border-red-800 dark:text-red-300 dark:hover:bg-red-950"
									onclick={(event) => {
										event.stopPropagation();
										confirmDelete(persona.name);
									}}
								>
									Delete
								</button>
							{:else}
								<button
									type="button"
									data-testid={`persona-fork-${persona.name}`}
									class="rounded border border-zinc-300 px-2 py-1 text-xs font-medium hover:bg-zinc-100 dark:border-zinc-700 dark:hover:bg-zinc-800"
									onclick={(event) => {
										event.stopPropagation();
										void forkLibrary(persona);
									}}
								>
									Fork to project
								</button>
							{/if}
						</div>
					</article>
				{/each}

				{#if data.personas.length === 0}
					<div
						class="rounded-md border border-dashed border-zinc-300 bg-white px-4 py-10 text-center text-sm text-zinc-600 dark:border-zinc-700 dark:bg-zinc-900 dark:text-zinc-300"
					>
						No personas found.
					</div>
				{/if}
			</section>

			<section
				aria-label="Persona detail"
				class="min-h-[520px] rounded-md border border-zinc-200 bg-white p-5 shadow-sm dark:border-zinc-800 dark:bg-zinc-900"
			>
				{#if selected}
					<div class="flex flex-col gap-5">
						<div
							class="flex flex-col gap-4 border-b border-zinc-200 pb-5 md:flex-row md:items-start md:justify-between dark:border-zinc-800"
						>
							<div>
								<p
									class="text-xs font-semibold tracking-[0.16em] text-teal-700 uppercase dark:text-teal-300"
								>
									{selected.source}
								</p>
								<h1 class="mt-1 text-3xl font-semibold tracking-tight">
									{displayName(selected.name)}
								</h1>
								<p class="mt-2 max-w-3xl text-sm leading-6 text-zinc-600 dark:text-zinc-300">
									{selected.description}
								</p>
							</div>
							<button
								type="button"
								class="inline-flex items-center justify-center rounded-md bg-zinc-950 px-4 py-2 text-sm font-semibold text-white shadow-sm transition hover:bg-teal-700 focus:ring-2 focus:ring-teal-500 focus:ring-offset-2 focus:outline-none dark:bg-zinc-50 dark:text-zinc-950 dark:hover:bg-teal-200"
								onclick={() => void openBindDialog()}
							>
								Bind to role
							</button>
						</div>

						<div class="grid gap-4 lg:grid-cols-[1fr_280px]">
							<div>
								<h2 class="mb-3 text-sm font-semibold text-zinc-700 dark:text-zinc-200">
									Instructions
								</h2>
								<div
									class="persona-body rounded-md border border-zinc-200 bg-stone-50 p-5 text-sm leading-7 text-zinc-800 [&_code]:rounded [&_code]:bg-zinc-950/8 [&_code]:px-1 [&_code]:py-0.5 [&_code]:text-[0.85em] dark:border-zinc-800 dark:bg-zinc-950 dark:text-zinc-100 dark:[&_code]:bg-zinc-100/12"
								>
									<!-- eslint-disable-next-line svelte/no-at-html-tags -->
									{@html renderedBody}
								</div>
							</div>

							<div class="space-y-4">
								<section
									aria-labelledby="bindings-heading"
									class="rounded-md border border-zinc-200 p-4 dark:border-zinc-800"
								>
									<h2 id="bindings-heading" class="text-sm font-semibold">Bindings</h2>
									{#if selected.bindings.length > 0}
										<ul class="mt-3 space-y-2 text-sm text-zinc-700 dark:text-zinc-300">
											{#each selected.bindings as binding (`${binding.projectId}:${binding.role}`)}
												<li class="rounded bg-zinc-100 px-3 py-2 dark:bg-zinc-800">
													<span class="font-medium">{binding.projectId}</span>
													<span class="text-zinc-500 dark:text-zinc-400"> / {binding.role}</span>
												</li>
											{/each}
										</ul>
									{:else}
										<p class="mt-3 text-sm text-zinc-500 dark:text-zinc-400">
											No current bindings.
										</p>
									{/if}
								</section>

								{#if selected.roles.length > 0}
									<section class="rounded-md border border-zinc-200 p-4 dark:border-zinc-800">
										<h2 class="text-sm font-semibold">Roles</h2>
										<div class="mt-3 flex flex-wrap gap-2">
											{#each selected.roles as role (role)}
												<span
													class="rounded bg-teal-50 px-2 py-1 text-xs font-medium text-teal-800 dark:bg-teal-950 dark:text-teal-200"
												>
													{role}
												</span>
											{/each}
										</div>
									</section>
								{/if}
							</div>
						</div>
					</div>
				{:else}
					<div class="flex min-h-[480px] items-center justify-center text-center">
						<div>
							<h2 class="text-xl font-semibold">Select a persona</h2>
							<p class="mt-2 text-sm text-zinc-600 dark:text-zinc-300">
								Open a card to inspect the prompt body and role bindings.
							</p>
						</div>
					</div>
				{/if}
			</section>
		</div>
	</div>
</div>

{#if editorOpen}
	<dialog
		open
		aria-labelledby="editor-dialog-title"
		data-testid="persona-editor"
		class="fixed top-1/2 left-1/2 z-50 w-[min(92vw,40rem)] -translate-x-1/2 -translate-y-1/2 rounded-md border border-zinc-200 bg-white p-0 text-zinc-950 shadow-2xl backdrop:bg-zinc-950/50 dark:border-zinc-700 dark:bg-zinc-900 dark:text-zinc-50"
	>
		<form
			class="space-y-4 p-5"
			onsubmit={(event) => {
				event.preventDefault();
				void submitEditor();
			}}
		>
			<div class="flex items-start justify-between gap-4">
				<div>
					<h2 id="editor-dialog-title" class="text-lg font-semibold">
						{editorMode === 'create' ? 'New persona' : `Edit ${editorName}`}
					</h2>
					<p class="mt-1 text-sm text-zinc-600 dark:text-zinc-300">
						Project personas live with this project under <code>.ddx/personas</code>.
					</p>
				</div>
				<button
					type="button"
					class="rounded px-2 py-1 text-sm text-zinc-500 hover:bg-zinc-100 dark:text-zinc-300 dark:hover:bg-zinc-800"
					onclick={() => (editorOpen = false)}
				>
					Close
				</button>
			</div>

			{#if editorMode === 'create'}
				<label class="block text-sm font-medium" for="editor-name">
					Name
					<input
						id="editor-name"
						type="text"
						data-testid="persona-editor-name"
						class="mt-1 w-full rounded-md border border-zinc-300 bg-white px-3 py-2 text-sm dark:border-zinc-700 dark:bg-zinc-950"
						bind:value={editorName}
						placeholder="our-reviewer"
					/>
				</label>
			{/if}

			<label class="block text-sm font-medium" for="editor-body">
				Body (markdown with YAML frontmatter)
				<textarea
					id="editor-body"
					data-testid="persona-editor-body"
					class="mt-1 h-72 w-full rounded-md border border-zinc-300 bg-white px-3 py-2 font-mono text-xs dark:border-zinc-700 dark:bg-zinc-950"
					bind:value={editorBody}
				></textarea>
			</label>

			{#if editorError}
				<div
					role="alert"
					class="rounded border border-red-300 bg-red-50 px-3 py-2 text-sm text-red-900 dark:border-red-800 dark:bg-red-950 dark:text-red-100"
				>
					{editorError}
				</div>
			{/if}

			<div class="flex justify-end gap-2 pt-2">
				<button
					type="button"
					class="rounded-md border border-zinc-300 px-4 py-2 text-sm font-medium hover:bg-zinc-100 dark:border-zinc-700 dark:hover:bg-zinc-800"
					onclick={() => (editorOpen = false)}
				>
					Cancel
				</button>
				<button
					type="submit"
					data-testid="persona-editor-save"
					class="rounded-md bg-teal-700 px-4 py-2 text-sm font-semibold text-white hover:bg-teal-800 disabled:opacity-60"
					disabled={editorSaving || !editorBody.trim() || (editorMode === 'create' && !editorName.trim())}
				>
					{editorSaving ? 'Saving...' : 'Save'}
				</button>
			</div>
		</form>
	</dialog>
{/if}

{#if deleteTarget}
	<dialog
		open
		aria-labelledby="delete-dialog-title"
		data-testid="persona-delete-dialog"
		class="fixed top-1/2 left-1/2 z-50 w-[min(92vw,28rem)] -translate-x-1/2 -translate-y-1/2 rounded-md border border-zinc-200 bg-white p-0 text-zinc-950 shadow-2xl backdrop:bg-zinc-950/50 dark:border-zinc-700 dark:bg-zinc-900 dark:text-zinc-50"
	>
		<div class="space-y-4 p-5">
			<h2 id="delete-dialog-title" class="text-lg font-semibold">Delete persona?</h2>
			<p class="text-sm text-zinc-600 dark:text-zinc-300">
				This removes <code>.ddx/personas/{deleteTarget}.md</code>. Existing bindings that
				point at it will continue to reference the name; update them as needed.
			</p>
			{#if deleteError}
				<div
					role="alert"
					class="rounded border border-red-300 bg-red-50 px-3 py-2 text-sm text-red-900 dark:border-red-800 dark:bg-red-950 dark:text-red-100"
				>
					{deleteError}
				</div>
			{/if}
			<div class="flex justify-end gap-2 pt-2">
				<button
					type="button"
					class="rounded-md border border-zinc-300 px-4 py-2 text-sm font-medium hover:bg-zinc-100 dark:border-zinc-700 dark:hover:bg-zinc-800"
					onclick={() => (deleteTarget = null)}
				>
					Cancel
				</button>
				<button
					type="button"
					data-testid="persona-delete-confirm"
					class="rounded-md bg-red-600 px-4 py-2 text-sm font-semibold text-white hover:bg-red-700"
					onclick={() => void runDelete()}
				>
					Delete
				</button>
			</div>
		</div>
	</dialog>
{/if}

{#if bindOpen && selected}
	<dialog
		open
		aria-labelledby="bind-dialog-title"
		class="fixed top-1/2 left-1/2 z-50 w-[min(92vw,32rem)] -translate-x-1/2 -translate-y-1/2 rounded-md border border-zinc-200 bg-white p-0 text-zinc-950 shadow-2xl backdrop:bg-zinc-950/50 dark:border-zinc-700 dark:bg-zinc-900 dark:text-zinc-50"
	>
		<form
			class="space-y-4 p-5"
			onsubmit={(event) => {
				event.preventDefault();
				void submitBind();
			}}
		>
			<div class="flex items-start justify-between gap-4">
				<div>
					<h2 id="bind-dialog-title" class="text-lg font-semibold">Bind {selected.name}</h2>
					<p class="mt-1 text-sm text-zinc-600 dark:text-zinc-300">
						Assign this persona to a role in a project.
					</p>
				</div>
				<button
					type="button"
					class="rounded px-2 py-1 text-sm text-zinc-500 hover:bg-zinc-100 dark:text-zinc-300 dark:hover:bg-zinc-800"
					onclick={() => (bindOpen = false)}
				>
					Close
				</button>
			</div>

			<label class="block text-sm font-medium" for="bind-role">
				Role
				<select
					id="bind-role"
					class="mt-1 w-full rounded-md border border-zinc-300 bg-white px-3 py-2 text-sm dark:border-zinc-700 dark:bg-zinc-950"
					bind:value={bindRole}
					onchange={() => {
						warning = '';
						needsConfirm = false;
					}}
				>
					{#each selected.roles as role (role)}
						<option value={role}>{role}</option>
					{/each}
				</select>
			</label>

			<label class="block text-sm font-medium" for="bind-project">
				Project
				<select
					id="bind-project"
					class="mt-1 w-full rounded-md border border-zinc-300 bg-white px-3 py-2 text-sm dark:border-zinc-700 dark:bg-zinc-950"
					value={bindProjectId}
					onchange={(event) => void onProjectChange(event)}
				>
					{#each projects as project (project.id)}
						<option value={project.id}>{project.name}</option>
					{/each}
				</select>
			</label>

			{#if loadingBindings}
				<p class="text-sm text-zinc-500 dark:text-zinc-400">Reading current bindings...</p>
			{/if}

			{#if warning}
				<div
					role="alert"
					class="rounded border border-amber-300 bg-amber-50 px-3 py-2 text-sm text-amber-900 dark:border-amber-800 dark:bg-amber-950 dark:text-amber-100"
				>
					{warning}
				</div>
			{/if}

			{#if bindError}
				<div
					role="alert"
					class="rounded border border-red-300 bg-red-50 px-3 py-2 text-sm text-red-900 dark:border-red-800 dark:bg-red-950 dark:text-red-100"
				>
					{bindError}
				</div>
			{/if}

			<div class="flex justify-end gap-2 pt-2">
				<button
					type="button"
					class="rounded-md border border-zinc-300 px-4 py-2 text-sm font-medium hover:bg-zinc-100 dark:border-zinc-700 dark:hover:bg-zinc-800"
					onclick={() => (bindOpen = false)}
				>
					Cancel
				</button>
				{#if needsConfirm}
					<button
						type="button"
						class="rounded-md bg-amber-600 px-4 py-2 text-sm font-semibold text-white hover:bg-amber-700 disabled:opacity-60"
						disabled={savingBinding}
						onclick={() => void submitBind(true)}
					>
						Confirm overwrite
					</button>
				{:else}
					<button
						type="submit"
						class="rounded-md bg-zinc-950 px-4 py-2 text-sm font-semibold text-white hover:bg-teal-700 disabled:opacity-60 dark:bg-zinc-50 dark:text-zinc-950 dark:hover:bg-teal-200"
						disabled={savingBinding || loadingBindings}
					>
						{savingBinding ? 'Saving...' : 'Bind'}
					</button>
				{/if}
			</div>
		</form>
	</dialog>
{/if}

<style>
	.persona-body :global(h1),
	.persona-body :global(h2),
	.persona-body :global(h3) {
		margin: 0 0 0.75rem;
		font-weight: 700;
		line-height: 1.2;
	}

	.persona-body :global(h1) {
		font-size: 1.35rem;
	}

	.persona-body :global(h2) {
		font-size: 1.1rem;
	}

	.persona-body :global(p),
	.persona-body :global(ul),
	.persona-body :global(ol),
	.persona-body :global(pre) {
		margin: 0 0 1rem;
	}

	.persona-body :global(ul),
	.persona-body :global(ol) {
		padding-left: 1.25rem;
	}
</style>
