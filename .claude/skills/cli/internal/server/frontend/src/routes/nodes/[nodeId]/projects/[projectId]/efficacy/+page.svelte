<script lang="ts">
	import type { PageData } from './$types';
	import type { ComparisonRecord, EfficacyRow } from './+page';
	import { goto } from '$app/navigation';
	import { page } from '$app/stores';
	import { createClient } from '$lib/gql/client';
	import { gql } from 'graphql-request';
	import { BarChart3, GitCompareArrows, Link2, Plus, X } from 'lucide-svelte';

	const EFFICACY_ATTEMPTS_QUERY = gql`
		query EfficacyAttempts($rowKey: String!, $projectId: String) {
			efficacyAttempts(rowKey: $rowKey, projectId: $projectId) {
				rowKey
				attempts {
					beadId
					outcome
					durationMs
					costUsd
					evidenceBundleUrl
				}
			}
		}
	`;

	const COMPARISON_DISPATCH_MUTATION = gql`
		mutation ComparisonDispatch($arms: [ComparisonArmInput!]!) {
			comparisonDispatch(arms: $arms) {
				id
				state
				armCount
			}
		}
	`;

	interface EfficacyAttempt {
		beadId: string;
		outcome: string;
		durationMs: number;
		costUsd: number | null;
		evidenceBundleUrl: string;
	}

	interface EfficacyAttemptsResult {
		efficacyAttempts: {
			rowKey: string;
			attempts: EfficacyAttempt[];
		};
	}

	interface ComparisonArm {
		model: string;
		prompt: string;
	}

	interface ComparisonDispatchResult {
		comparisonDispatch: ComparisonRecord;
	}

	let { data }: { data: PageData } = $props();

	let tierFilter = $state('');
	let labelFilter = $state('');
	let specIdFilter = $state('');
	let selectedRowKey = $state<string | null>(null);
	let selectedRowLabel = $state('');
	let attempts = $state<EfficacyAttempt[]>([]);
	let attemptsLoading = $state(false);
	let compareOpen = $state(false);
	let comparisonArms = $state<ComparisonArm[]>([]);
	let comparisonResults = $state<ComparisonRecord[]>([]);
	let dispatching = $state(false);

	const modelOptions = $derived(
		Array.from(new Set(data.rows.map((row) => row.model))).sort((a, b) => a.localeCompare(b))
	);

	const filteredRows = $derived(
		data.rows.filter((row) => {
			const tierMatches =
				!tierFilter || !rowTier(row) || rowTier(row).toLowerCase() === tierFilter.toLowerCase();
			const labelMatches =
				!labelFilter ||
				!row.labels ||
				row.labels.some((label) => label.toLowerCase().includes(labelFilter.toLowerCase()));
			const specMatches =
				!specIdFilter ||
				!row.specId ||
				row.specId.toLowerCase().includes(specIdFilter.toLowerCase());
			return tierMatches && labelMatches && specMatches;
		})
	);

	$effect(() => {
		tierFilter = data.activeTier;
		labelFilter = data.activeLabel;
		specIdFilter = data.activeSpecId;
		comparisonResults = data.comparisons;
	});

	function rowKey(row: EfficacyRow): string {
		return row.rowKey ?? `${row.harness}|${row.provider}|${row.model}`;
	}

	function rowTier(row: EfficacyRow): string {
		if (row.tier) return row.tier;
		if (/qwen|omlx|local|cheap/i.test(`${row.provider} ${row.model}`)) return 'cheap';
		if (/gpt|claude|sonnet|opus/i.test(row.model)) return 'frontier';
		return '';
	}

	function updateFilter(key: 'tier' | 'label' | 'spec-id', value: string) {
		const params = new URLSearchParams($page.url.searchParams);
		if (value.trim()) {
			params.set(key, value.trim());
		} else {
			params.delete(key);
		}
		const search = params.toString();
		goto(search ? `${$page.url.pathname}?${search}` : $page.url.pathname, { replaceState: true });
	}

	async function openAttempts(row: EfficacyRow) {
		const key = rowKey(row);
		selectedRowKey = key;
		selectedRowLabel = `${row.harness} / ${row.provider} / ${row.model}`;
		attempts = [];
		attemptsLoading = true;
		try {
			const client = createClient();
			const result = await client.request<EfficacyAttemptsResult>(EFFICACY_ATTEMPTS_QUERY, {
				rowKey: key,
				projectId: data.projectId
			});
			if (selectedRowKey === key) {
				attempts = result.efficacyAttempts.attempts.slice(0, 10);
			}
		} finally {
			if (selectedRowKey === key) {
				attemptsLoading = false;
			}
		}
	}

	function openCompare() {
		comparisonArms = [];
		compareOpen = true;
	}

	function addArm() {
		comparisonArms = [...comparisonArms, { model: modelOptions[0] ?? '', prompt: '' }];
	}

	function removeArm(index: number) {
		comparisonArms = comparisonArms.filter((_, i) => i !== index);
	}

	function setArmModel(index: number, model: string) {
		comparisonArms = comparisonArms.map((arm, i) => (i === index ? { ...arm, model } : arm));
	}

	function setArmPrompt(index: number, prompt: string) {
		comparisonArms = comparisonArms.map((arm, i) => (i === index ? { ...arm, prompt } : arm));
	}

	async function submitComparison() {
		const arms = comparisonArms
			.map((arm) => ({ model: arm.model.trim(), prompt: arm.prompt.trim() }))
			.filter((arm) => arm.model && arm.prompt);
		if (arms.length === 0) return;

		dispatching = true;
		try {
			const client = createClient();
			const result = await client.request<ComparisonDispatchResult>(COMPARISON_DISPATCH_MUTATION, {
				arms
			});
			comparisonResults = [result.comparisonDispatch, ...comparisonResults];
			compareOpen = false;
		} finally {
			dispatching = false;
		}
	}

	function beadHref(beadId: string): string {
		const p = $page.params as Record<string, string>;
		return `/nodes/${p['nodeId']}/projects/${p['projectId']}/beads/${beadId}`;
	}

	function formatPercent(value: number): string {
		return `${(value * 100).toFixed(1)}%`;
	}

	function formatTokens(input: number, output: number): string {
		return `${input.toLocaleString()} / ${output.toLocaleString()}`;
	}

	function formatDuration(ms: number): string {
		return `${(ms / 1000).toFixed(ms < 10000 ? 1 : 0)}s`;
	}

	function formatCost(value: number | null): string {
		return value === null ? '—' : `$${value.toFixed(3)}`;
	}
</script>

<svelte:head>
	<title>Efficacy | DDx</title>
</svelte:head>

<div class="space-y-5">
	<header class="flex flex-wrap items-start justify-between gap-4">
		<div>
			<div
				class="mb-2 inline-flex items-center gap-2 rounded-md border border-emerald-200 bg-emerald-50 px-2 py-1 text-xs font-medium text-emerald-800 dark:border-emerald-800 dark:bg-emerald-950/40 dark:text-emerald-200"
			>
				<BarChart3 class="h-3.5 w-3.5" />
				Model routing evidence
			</div>
			<h1 class="text-2xl font-semibold tracking-tight text-gray-950 dark:text-white">Efficacy</h1>
		</div>
		<button
			type="button"
			onclick={openCompare}
			class="inline-flex items-center gap-2 rounded-md bg-gray-950 px-3 py-2 text-sm font-medium text-white hover:bg-gray-800 focus:ring-2 focus:ring-emerald-500 focus:ring-offset-2 focus:outline-none dark:bg-white dark:text-gray-950 dark:hover:bg-gray-200 dark:focus:ring-offset-gray-950"
		>
			<GitCompareArrows class="h-4 w-4" />
			Compare
		</button>
	</header>

	<form class="grid gap-3 md:grid-cols-[12rem_1fr_1fr]" aria-label="Efficacy filters">
		<label class="space-y-1 text-sm font-medium text-gray-700 dark:text-gray-300">
			<span>Tier</span>
			<select
				name="tier"
				value={tierFilter}
				onchange={(event) => {
					const value = (event.currentTarget as HTMLSelectElement).value;
					tierFilter = value;
					updateFilter('tier', value);
				}}
				class="w-full rounded-md border border-gray-300 bg-white px-3 py-2 text-sm text-gray-950 focus:border-emerald-500 focus:ring-1 focus:ring-emerald-500 focus:outline-none dark:border-gray-700 dark:bg-gray-950 dark:text-gray-100"
			>
				<option value="">All tiers</option>
				<option value="cheap">Cheap</option>
				<option value="balanced">Balanced</option>
				<option value="frontier">Frontier</option>
			</select>
		</label>

		<label class="space-y-1 text-sm font-medium text-gray-700 dark:text-gray-300">
			<span>Label</span>
			<input
				name="label"
				type="text"
				value={labelFilter}
				oninput={(event) => {
					const value = (event.currentTarget as HTMLInputElement).value;
					labelFilter = value;
					updateFilter('label', value);
				}}
				class="w-full rounded-md border border-gray-300 bg-white px-3 py-2 text-sm text-gray-950 focus:border-emerald-500 focus:ring-1 focus:ring-emerald-500 focus:outline-none dark:border-gray-700 dark:bg-gray-950 dark:text-gray-100"
			/>
		</label>

		<label class="space-y-1 text-sm font-medium text-gray-700 dark:text-gray-300">
			<span>Spec ID</span>
			<input
				name="spec-id"
				type="text"
				value={specIdFilter}
				oninput={(event) => {
					const value = (event.currentTarget as HTMLInputElement).value;
					specIdFilter = value;
					updateFilter('spec-id', value);
				}}
				class="w-full rounded-md border border-gray-300 bg-white px-3 py-2 text-sm text-gray-950 focus:border-emerald-500 focus:ring-1 focus:ring-emerald-500 focus:outline-none dark:border-gray-700 dark:bg-gray-950 dark:text-gray-100"
			/>
		</label>
	</form>

	<div class="overflow-hidden rounded-lg border border-gray-200 dark:border-gray-800">
		<table aria-label="Efficacy table" class="w-full text-sm">
			<thead>
				<tr class="border-b border-gray-200 bg-gray-50 dark:border-gray-800 dark:bg-gray-900">
					<th class="px-4 py-3 text-left font-medium text-gray-600 dark:text-gray-300">Harness</th>
					<th class="px-4 py-3 text-left font-medium text-gray-600 dark:text-gray-300">Provider</th>
					<th class="px-4 py-3 text-left font-medium text-gray-600 dark:text-gray-300">Model</th>
					<th class="px-4 py-3 text-right font-medium text-gray-600 dark:text-gray-300">Attempts</th
					>
					<th class="px-4 py-3 text-right font-medium text-gray-600 dark:text-gray-300">
						Success rate
					</th>
					<th class="px-4 py-3 text-right font-medium text-gray-600 dark:text-gray-300">
						Tokens
					</th>
					<th class="px-4 py-3 text-right font-medium text-gray-600 dark:text-gray-300">
						Duration
					</th>
					<th class="px-4 py-3 text-right font-medium text-gray-600 dark:text-gray-300">Cost</th>
				</tr>
			</thead>
			<tbody>
				{#each filteredRows as row (rowKey(row))}
					<tr
						onclick={() => openAttempts(row)}
						class="cursor-pointer border-b border-gray-100 last:border-0 hover:bg-emerald-50/60 dark:border-gray-800 dark:hover:bg-emerald-950/20"
					>
						<td class="px-4 py-3 font-medium text-gray-900 dark:text-gray-100">{row.harness}</td>
						<td class="px-4 py-3 text-gray-700 dark:text-gray-300">{row.provider}</td>
						<td class="px-4 py-3 text-gray-900 dark:text-gray-100">
							<div class="flex items-center gap-2">
								<span class="font-mono text-xs">{row.model}</span>
								{#if row.warning}
									<span class="relative inline-flex">
										<svg
											role="img"
											aria-label="below adaptive floor"
											viewBox="0 0 20 20"
											class="warning-badge h-4 w-4 text-amber-600 focus:outline-none dark:text-amber-400"
										>
											<path
												fill="currentColor"
												d="M9.1 2.6a1 1 0 0 1 1.8 0l7.2 13.1A1 1 0 0 1 17.2 17H2.8a1 1 0 0 1-.9-1.3L9.1 2.6Zm.1 4.2.2 5.2h1.2l.2-5.2H9.2Zm.8 8.1a1 1 0 1 0 0-2.1 1 1 0 0 0 0 2.1Z"
											/>
										</svg>
										<span
											role="tooltip"
											class="warning-tooltip absolute top-6 left-1/2 z-20 hidden w-64 -translate-x-1/2 rounded-md border border-amber-200 bg-white p-3 text-xs leading-5 text-gray-800 shadow-lg dark:border-amber-900 dark:bg-gray-950 dark:text-gray-100"
										>
											Below adaptive floor threshold
											{#if row.warning.threshold !== null}
												({formatPercent(row.warning.threshold)}).
											{/if}
											<a
												class="ml-1 font-medium text-emerald-700 underline dark:text-emerald-300"
												href="/docs/routing-metrics"
											>
												Routing metrics
											</a>
										</span>
									</span>
								{/if}
							</div>
						</td>
						<td class="px-4 py-3 text-right text-gray-700 tabular-nums dark:text-gray-300">
							{row.attempts}
						</td>
						<td class="px-4 py-3 text-right text-gray-700 tabular-nums dark:text-gray-300">
							{row.successes}/{row.attempts} · {formatPercent(row.successRate)}
						</td>
						<td class="px-4 py-3 text-right font-mono text-xs text-gray-700 dark:text-gray-300">
							{formatTokens(row.medianInputTokens, row.medianOutputTokens)}
						</td>
						<td class="px-4 py-3 text-right text-gray-700 tabular-nums dark:text-gray-300">
							{formatDuration(row.medianDurationMs)}
						</td>
						<td class="px-4 py-3 text-right text-gray-700 tabular-nums dark:text-gray-300">
							{formatCost(row.medianCostUsd)}
						</td>
					</tr>
				{/each}
				{#if filteredRows.length === 0}
					<tr>
						<td colspan="8" class="px-4 py-8 text-center text-gray-700 dark:text-gray-300">
							No efficacy rows match the current filters.
						</td>
					</tr>
				{/if}
			</tbody>
		</table>
	</div>

	<div class="grid gap-4 lg:grid-cols-[minmax(0,1fr)_24rem]">
		<section
			aria-label="Comparisons"
			class="rounded-lg border border-gray-200 p-4 dark:border-gray-800"
		>
			<div class="mb-3 flex items-center justify-between">
				<h2 class="text-base font-semibold text-gray-950 dark:text-white">Comparisons</h2>
				<span class="text-xs text-gray-500 dark:text-gray-400"
					>{comparisonResults.length} records</span
				>
			</div>
			{#if comparisonResults.length > 0}
				<ul class="divide-y divide-gray-100 dark:divide-gray-800">
					{#each comparisonResults as comparison (comparison.id)}
						<li class="flex items-center justify-between gap-3 py-2">
							<a
								href={`/comparisons/${comparison.id}`}
								class="font-mono text-sm font-medium text-emerald-700 hover:underline dark:text-emerald-300"
							>
								{comparison.id}
							</a>
							<span class="text-xs text-gray-600 dark:text-gray-300">
								{comparison.state} · {comparison.armCount} arms
							</span>
						</li>
					{/each}
				</ul>
			{:else}
				<p class="text-sm text-gray-700 dark:text-gray-300">No comparisons yet.</p>
			{/if}
		</section>

		{#if selectedRowKey}
			<aside
				aria-label="Attempts detail"
				class="rounded-lg border border-gray-200 p-4 dark:border-gray-800"
			>
				<h2 class="text-base font-semibold text-gray-950 dark:text-white">Last 10 attempts</h2>
				<p class="mt-1 mb-3 text-xs text-gray-600 dark:text-gray-400">{selectedRowLabel}</p>
				{#if attemptsLoading}
					<p class="text-sm text-gray-700 dark:text-gray-300">Loading attempts...</p>
				{:else}
					<table class="w-full text-sm">
						<thead>
							<tr class="border-b border-gray-200 dark:border-gray-800">
								<th class="py-2 pr-3 text-left font-medium text-gray-600 dark:text-gray-300"
									>Bead</th
								>
								<th class="px-3 py-2 text-left font-medium text-gray-600 dark:text-gray-300">
									Outcome
								</th>
								<th class="px-3 py-2 text-right font-medium text-gray-600 dark:text-gray-300">
									Cost
								</th>
							</tr>
						</thead>
						<tbody>
							{#each attempts as attempt (attempt.beadId)}
								<tr class="border-b border-gray-100 last:border-0 dark:border-gray-800">
									<td class="py-2 pr-3 align-top">
										<a
											href={beadHref(attempt.beadId)}
											class="font-mono text-xs font-medium text-emerald-700 hover:underline dark:text-emerald-300"
										>
											{attempt.beadId}
										</a>
										<a
											href={attempt.evidenceBundleUrl}
											class="mt-1 flex items-center gap-1 text-xs text-gray-600 hover:text-gray-900 hover:underline dark:text-gray-400 dark:hover:text-gray-100"
										>
											<Link2 class="h-3 w-3" />
											Evidence bundle
										</a>
									</td>
									<td class="px-3 py-2 align-top text-gray-700 dark:text-gray-300">
										{attempt.outcome}
										<div class="text-xs text-gray-500 dark:text-gray-500">
											{formatDuration(attempt.durationMs)}
										</div>
									</td>
									<td
										class="px-3 py-2 text-right align-top text-gray-700 tabular-nums dark:text-gray-300"
									>
										{formatCost(attempt.costUsd)}
									</td>
								</tr>
							{/each}
						</tbody>
					</table>
				{/if}
			</aside>
		{/if}
	</div>
</div>

{#if compareOpen}
	<div class="fixed inset-0 z-40 bg-black/40" aria-hidden="true"></div>
	<div class="fixed inset-0 z-50 grid place-items-center p-4">
		<dialog
			open
			aria-modal="true"
			aria-labelledby="compare-title"
			class="max-h-[90vh] w-full max-w-2xl overflow-auto rounded-lg bg-white p-5 shadow-xl dark:bg-gray-950"
		>
			<div class="mb-4 flex items-center justify-between gap-3">
				<h2 id="compare-title" class="text-lg font-semibold text-gray-950 dark:text-white">
					Compare
				</h2>
				<button
					type="button"
					onclick={() => (compareOpen = false)}
					aria-label="Close compare dialog"
					class="rounded-md p-1 text-gray-500 hover:bg-gray-100 hover:text-gray-900 dark:hover:bg-gray-900 dark:hover:text-white"
				>
					<X class="h-5 w-5" />
				</button>
			</div>

			<div class="space-y-3">
				{#each comparisonArms as arm, index}
					<div
						data-testid="comparison-arm"
						class="grid gap-3 rounded-lg border border-gray-200 p-3 dark:border-gray-800"
					>
						<div class="flex items-center justify-between gap-3">
							<label class="flex-1 space-y-1 text-sm font-medium text-gray-700 dark:text-gray-300">
								<span>Model</span>
								<select
									name="model"
									value={arm.model}
									onchange={(event) =>
										setArmModel(index, (event.currentTarget as HTMLSelectElement).value)}
									class="w-full rounded-md border border-gray-300 bg-white px-3 py-2 text-sm text-gray-950 focus:border-emerald-500 focus:ring-1 focus:ring-emerald-500 focus:outline-none dark:border-gray-700 dark:bg-gray-950 dark:text-gray-100"
								>
									{#each modelOptions as model}
										<option value={model}>{model}</option>
									{/each}
								</select>
							</label>
							<button
								type="button"
								onclick={() => removeArm(index)}
								aria-label="Remove comparison arm"
								class="mt-6 rounded-md p-2 text-gray-500 hover:bg-gray-100 hover:text-gray-900 dark:hover:bg-gray-900 dark:hover:text-white"
							>
								<X class="h-4 w-4" />
							</button>
						</div>
						<label class="space-y-1 text-sm font-medium text-gray-700 dark:text-gray-300">
							<span>Prompt</span>
							<textarea
								name="prompt"
								rows="3"
								value={arm.prompt}
								oninput={(event) =>
									setArmPrompt(index, (event.currentTarget as HTMLTextAreaElement).value)}
								class="w-full resize-y rounded-md border border-gray-300 bg-white px-3 py-2 text-sm text-gray-950 focus:border-emerald-500 focus:ring-1 focus:ring-emerald-500 focus:outline-none dark:border-gray-700 dark:bg-gray-950 dark:text-gray-100"
							></textarea>
						</label>
					</div>
				{/each}
			</div>

			<div class="mt-4 flex flex-wrap items-center justify-between gap-3">
				<button
					type="button"
					onclick={addArm}
					class="inline-flex items-center gap-2 rounded-md border border-gray-300 px-3 py-2 text-sm font-medium text-gray-800 hover:bg-gray-50 focus:ring-2 focus:ring-emerald-500 focus:ring-offset-2 focus:outline-none dark:border-gray-700 dark:text-gray-200 dark:hover:bg-gray-900 dark:focus:ring-offset-gray-950"
				>
					<Plus class="h-4 w-4" />
					Add arm
				</button>
				<button
					type="button"
					onclick={submitComparison}
					disabled={dispatching || comparisonArms.length === 0}
					class="rounded-md bg-emerald-700 px-3 py-2 text-sm font-medium text-white hover:bg-emerald-800 disabled:cursor-not-allowed disabled:opacity-50 dark:bg-emerald-600 dark:hover:bg-emerald-500"
				>
					{dispatching ? 'Starting...' : 'Start'}
				</button>
			</div>
		</dialog>
	</div>
{/if}

<style>
	.warning-badge:hover + .warning-tooltip,
	.warning-badge:focus + .warning-tooltip {
		display: block;
	}
</style>
