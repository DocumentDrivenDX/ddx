<script lang="ts">
	import { goto } from '$app/navigation';
	import { page } from '$app/stores';
	import { prefixOf } from '$lib/artifacts/grouping';
	import {
		ARTIFACT_TYPE_TABS,
		artifactTypeKey,
		artifactTypeLabel,
		hasArtifactTypeCollision,
		selectedArtifactTypeDefinition,
		updateTypeDefUrl,
		type ArtifactTypeDefinition,
		type ArtifactTypeTab
	} from '$lib/artifactTypePanel';

	type Props = {
		artifactPath: string;
		typeDefinitions: ArtifactTypeDefinition[];
	};

	let { artifactPath, typeDefinitions }: Props = $props();

	let activeTab = $state<ArtifactTypeTab>('referencePrompt');
	let selectedTypeDefKey = $state('');

	const artifactPrefix = $derived(prefixOf(artifactPath));
	const collision = $derived(hasArtifactTypeCollision(typeDefinitions));
	const selectedTypeDefinition = $derived.by(() =>
		selectedArtifactTypeDefinition(typeDefinitions, $page.url.searchParams.get('typeDef'))
	);

	$effect(() => {
		selectedTypeDefKey = selectedTypeDefinition ? artifactTypeKey(selectedTypeDefinition) : '';
	});

	function selectTypeDefinition(nextKey: string) {
		void goto(updateTypeDefUrl($page.url, nextKey || null), {
			replaceState: true,
			keepFocus: true,
			noScroll: true
		});
	}
</script>

{#if selectedTypeDefinition}
	<section
		data-testid="artifact-type-panel"
		class="border-border-line dark:border-dark-border-line overflow-hidden border"
	>
		<div
			class="bg-bg-surface dark:bg-dark-bg-surface border-border-line dark:border-dark-border-line flex flex-col gap-3 border-b px-4 py-3 md:flex-row md:items-start md:justify-between"
		>
			<div class="space-y-1">
				<div class="flex flex-wrap items-center gap-2">
					<h2 class="text-headline-sm text-fg-ink dark:text-dark-fg-ink font-semibold">
						Artifact type
					</h2>
					<span
						class="rounded-full bg-bg-elevated px-2 py-0.5 font-mono text-xs text-fg-muted dark:bg-dark-bg-elevated dark:text-dark-fg-muted"
					>
						Prefix {artifactPrefix}
					</span>
				</div>
				<p class="text-sm text-fg-muted dark:text-dark-fg-muted">
					{artifactTypeLabel(selectedTypeDefinition)} · {selectedTypeDefinition.plugin}/
					{selectedTypeDefinition.typeId}
				</p>
				<p class="text-sm text-fg-muted dark:text-dark-fg-muted">
					{selectedTypeDefinition.description}
				</p>
				<div class="flex flex-wrap gap-2 text-xs text-fg-muted dark:text-dark-fg-muted">
					<span class="rounded-full bg-bg-elevated px-2 py-0.5 dark:bg-dark-bg-elevated">
						Pattern: {selectedTypeDefinition.pattern}
					</span>
					<span class="rounded-full bg-bg-elevated px-2 py-0.5 dark:bg-dark-bg-elevated">
						Phase: {selectedTypeDefinition.phase || 'n/a'}
					</span>
					<span class="rounded-full bg-bg-elevated px-2 py-0.5 dark:bg-dark-bg-elevated">
						Source: {selectedTypeDefinition.sourceMetaPath}
					</span>
				</div>
			</div>

			{#if collision}
				<label class="space-y-1 text-sm text-fg-muted dark:text-dark-fg-muted">
					<span class="block font-medium">Type definition</span>
					<select
						data-testid="artifact-type-selector"
						class="border-border-line dark:border-dark-border-line bg-bg-canvas dark:bg-dark-bg-canvas text-fg-ink dark:text-dark-fg-ink rounded-md border px-3 py-2"
						bind:value={selectedTypeDefKey}
						onchange={() => selectTypeDefinition(selectedTypeDefKey)}
					>
						{#each typeDefinitions as def}
							<option value={artifactTypeKey(def)}>
								{artifactTypeLabel(def)} · {def.plugin}/{def.typeId}
							</option>
						{/each}
					</select>
				</label>
			{/if}
		</div>

		<div class="border-border-line dark:border-dark-border-line border-b px-4 py-2">
			<div class="flex flex-wrap gap-2">
				{#each ARTIFACT_TYPE_TABS as tab}
					<button
						type="button"
						data-testid={`artifact-type-tab-${tab.value}`}
						class="rounded-full px-3 py-1 text-sm transition-colors {activeTab === tab.value
							? 'bg-accent-lever text-white dark:bg-dark-accent-lever'
							: 'bg-bg-surface text-fg-muted hover:bg-bg-elevated hover:text-fg-ink dark:bg-dark-bg-surface dark:text-dark-fg-muted dark:hover:bg-dark-bg-elevated dark:hover:text-dark-fg-ink'}"
						aria-pressed={activeTab === tab.value}
						onclick={() => {
							activeTab = tab.value;
						}}
					>
						{tab.label}
					</button>
				{/each}
			</div>
		</div>

		<div class="space-y-4 p-4">
			{#if activeTab === 'referencePrompt'}
				<div class="space-y-2" data-testid="artifact-type-reference-prompt">
					<div class="text-xs font-semibold uppercase tracking-wide text-fg-muted dark:text-dark-fg-muted">
						Reference Prompt
					</div>
					<div class="flex items-center gap-2 text-xs text-fg-muted dark:text-dark-fg-muted">
						<span>{selectedTypeDefinition.prompt.path}</span>
						{#if selectedTypeDefinition.prompt.isTruncated}
							<span class="rounded-full bg-amber-100 px-2 py-0.5 text-amber-800 dark:bg-amber-900/30 dark:text-amber-400">
								Truncated
							</span>
						{/if}
					</div>
					<pre class="bg-bg-canvas dark:bg-dark-bg-canvas overflow-auto rounded-md border border-border-line p-3 text-sm whitespace-pre-wrap dark:border-dark-border-line">{selectedTypeDefinition.prompt.content}</pre>
				</div>
			{:else if activeTab === 'template'}
				<div class="space-y-2" data-testid="artifact-type-template">
					<div class="text-xs font-semibold uppercase tracking-wide text-fg-muted dark:text-dark-fg-muted">
						Template
					</div>
					<div class="flex items-center gap-2 text-xs text-fg-muted dark:text-dark-fg-muted">
						<span>{selectedTypeDefinition.template.path}</span>
						{#if selectedTypeDefinition.template.isTruncated}
							<span class="rounded-full bg-amber-100 px-2 py-0.5 text-amber-800 dark:bg-amber-900/30 dark:text-amber-400">
								Truncated
							</span>
						{/if}
					</div>
					<pre class="bg-bg-canvas dark:bg-dark-bg-canvas overflow-auto rounded-md border border-border-line p-3 text-sm whitespace-pre-wrap dark:border-dark-border-line">{selectedTypeDefinition.template.content}</pre>
				</div>
			{:else}
				<div class="space-y-3" data-testid="artifact-type-examples">
					<div class="text-xs font-semibold uppercase tracking-wide text-fg-muted dark:text-dark-fg-muted">
						Examples
					</div>
					{#if selectedTypeDefinition.examples.length === 0}
						<p class="text-sm text-fg-muted dark:text-dark-fg-muted">No examples provided.</p>
					{:else}
						{#each selectedTypeDefinition.examples as example}
							<div class="space-y-2 rounded-md border border-border-line p-3 dark:border-dark-border-line">
								<div class="flex flex-wrap items-center gap-2 text-xs text-fg-muted dark:text-dark-fg-muted">
									<span>{example.path}</span>
									{#if example.description}
										<span>· {example.description}</span>
									{/if}
									{#if example.isTruncated}
										<span class="rounded-full bg-amber-100 px-2 py-0.5 text-amber-800 dark:bg-amber-900/30 dark:text-amber-400">
											Truncated
										</span>
									{/if}
								</div>
								<pre class="bg-bg-canvas dark:bg-dark-bg-canvas overflow-auto rounded-md border border-border-line p-3 text-sm whitespace-pre-wrap dark:border-dark-border-line">{example.content}</pre>
							</div>
						{/each}
					{/if}
				</div>
			{/if}
		</div>
	</section>
{/if}
