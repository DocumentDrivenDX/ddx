<script lang="ts">
	import type { PageData } from './$types';
	import type { PersonaNode } from './+page';

	let { data }: { data: PageData } = $props();

	let selected = $state<PersonaNode | null>(
		data.personas.edges.length > 0 ? data.personas.edges[0].node : null
	);

	function fmtDate(iso: string | null): string {
		if (!iso) return '—';
		return new Date(iso).toLocaleString();
	}
</script>

<div class="flex h-full gap-0">
	<!-- Left panel: persona list -->
	<div class="flex w-64 shrink-0 flex-col border-r border-gray-200 dark:border-gray-700">
		<div class="flex items-center justify-between border-b border-gray-200 px-4 py-3 dark:border-gray-700">
			<h1 class="text-sm font-semibold dark:text-white">Personas</h1>
			<span class="text-xs text-gray-400 dark:text-gray-500">{data.personas.totalCount}</span>
		</div>
		<div class="min-h-0 flex-1 overflow-y-auto">
			{#each data.personas.edges as edge (edge.cursor)}
				{@const p = edge.node}
				<button
					onclick={() => (selected = p)}
					class="w-full border-b border-gray-100 px-4 py-3 text-left hover:bg-gray-50 dark:border-gray-700 dark:hover:bg-gray-800 {selected?.id ===
					p.id
						? 'bg-blue-50 dark:bg-blue-900/20'
						: ''}"
				>
					<div class="text-sm font-medium text-gray-900 dark:text-gray-100">{p.name}</div>
					{#if p.roles.length > 0}
						<div class="mt-0.5 text-xs text-gray-500 dark:text-gray-400">
							{p.roles.join(', ')}
						</div>
					{/if}
				</button>
			{/each}
			{#if data.personas.edges.length === 0}
				<div class="px-4 py-8 text-center text-xs text-gray-400 dark:text-gray-600">
					No personas found.
				</div>
			{/if}
		</div>
	</div>

	<!-- Right panel: persona detail -->
	<div class="min-w-0 flex-1 overflow-y-auto p-6">
		{#if selected}
			<div class="space-y-4">
				<div>
					<h2 class="text-lg font-semibold dark:text-white">{selected.name}</h2>
					<p class="mt-1 text-sm text-gray-600 dark:text-gray-300">{selected.description}</p>
				</div>

				<!-- Metadata grid -->
				<div class="grid grid-cols-2 gap-4 text-sm sm:grid-cols-3">
					{#if selected.roles.length > 0}
						<div>
							<div class="text-xs font-medium text-gray-500 dark:text-gray-400">Roles</div>
							<div class="mt-1 flex flex-wrap gap-1">
								{#each selected.roles as role}
									<span
										class="rounded bg-blue-100 px-1.5 py-0.5 text-xs text-blue-700 dark:bg-blue-900/40 dark:text-blue-300"
									>
										{role}
									</span>
								{/each}
							</div>
						</div>
					{/if}
					{#if selected.tags.length > 0}
						<div>
							<div class="text-xs font-medium text-gray-500 dark:text-gray-400">Tags</div>
							<div class="mt-1 flex flex-wrap gap-1">
								{#each selected.tags as tag}
									<span
										class="rounded bg-gray-100 px-1.5 py-0.5 text-xs text-gray-600 dark:bg-gray-700 dark:text-gray-300"
									>
										{tag}
									</span>
								{/each}
							</div>
						</div>
					{/if}
					<div>
						<div class="text-xs font-medium text-gray-500 dark:text-gray-400">Modified</div>
						<div class="mt-1 text-xs dark:text-gray-200">{fmtDate(selected.modTime)}</div>
					</div>
					{#if selected.filePath}
						<div class="col-span-2 sm:col-span-3">
							<div class="text-xs font-medium text-gray-500 dark:text-gray-400">File</div>
							<div class="mt-1 break-all font-mono text-xs text-gray-500 dark:text-gray-400">
								{selected.filePath}
							</div>
						</div>
					{/if}
				</div>

				<!-- Content -->
				<div>
					<div class="mb-2 text-xs font-medium text-gray-500 dark:text-gray-400">
						Prompt / Instructions
					</div>
					<pre
						class="overflow-x-auto whitespace-pre-wrap rounded-lg border border-gray-200 bg-gray-50 p-4 font-mono text-xs leading-relaxed text-gray-800 dark:border-gray-700 dark:bg-gray-900 dark:text-gray-200"
					>{selected.content}</pre>
				</div>
			</div>
		{:else}
			<div class="flex h-full items-center justify-center text-gray-400 dark:text-gray-600">
				Select a persona to view details.
			</div>
		{/if}
	</div>
</div>
