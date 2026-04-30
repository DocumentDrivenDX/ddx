<script lang="ts">
	import type { PageData } from './$types';
	import type { ComparisonRecord, EfficacyRow } from './+page';
	import { goto } from '$app/navigation';
	import { page } from '$app/stores';
	import { createClient } from '$lib/gql/client';
	import { gql } from 'graphql-request';
	import { BarChart3, GitCompareArrows, Link2, X } from 'lucide-svelte';

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

	interface ComparisonArm {
		harness: string;
		provider: string;
		model: string;
		prompt: string;
	}

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
	let comparisonPrompt = $state('');
	let comparisonResults = $state<ComparisonRecord[]>([]);
	let dispatching = $state(false);
	let selectedRowKeys = $state<Set<string>>(new Set());

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

	function toggleRowSelection(row: EfficacyRow, checked: boolean) {
		const key = rowKey(row);
		const next = new Set(selectedRowKeys);
		if (checked) {
			next.add(key);
		} else {
			next.delete(key);
		}
		selectedRowKeys = next;
	}

	function openCompareSelected() {
		const selectedRows = filteredRows.filter((row) => selectedRowKeys.has(rowKey(row)));
		if (selectedRows.length < 2) return;
		comparisonArms = selectedRows.map((row) => ({
			harness: row.harness,
			provider: row.provider,
			model: row.model,
			prompt: ''
		}));
		comparisonPrompt = '';
		compareOpen = true;
	}

	function setPromptForAllArms(prompt: string) {
		comparisonPrompt = prompt;
	}

	async function submitComparison() {
		const prompt = comparisonPrompt.trim();
		if (!prompt) return;
		const arms = comparisonArms.map((arm) => ({
			harness: arm.harness,
			provider: arm.provider,
			model: arm.model,
			prompt
		}));
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
				class="mb-2 inline-flex items-center gap-2 border border-[#3B5B7A]/30 bg-[#3B5B7A]/8 px-2 py-1 text-[11px] font-bold tracking-[0.05em] uppercase text-[#3B5B7A] dark:border-[#7BA3CC]/30 dark:bg-[#7BA3CC]/10 dark:text-[#7BA3CC]"
			>
				<BarChart3 class="h-3.5 w-3.5" />
				Model routing evidence
			</div>
			<h1 class="text-xl font-bold tracking-[-0.02em] text-[#1F2125] dark:text-[#EDE6D6]">Efficacy</h1>
		</div>
		{#if filteredRows.length > 0}
			{@const selectionCount = filteredRows.filter((r) => selectedRowKeys.has(rowKey(r))).length}
			{@const canCompare = selectionCount >= 2}
			<button
				type="button"
				onclick={openCompareSelected}
				disabled={!canCompare}
				title={canCompare
					? `Compare ${selectionCount} selected rows`
					: 'Select 2 or more rows to compare'}
				class="inline-flex items-center gap-2 bg-[#3B5B7A] px-3 py-2 text-[11px] font-bold tracking-[0.05em] uppercase text-white hover:opacity-90 focus:ring-2 focus:ring-[#3B5B7A] focus:ring-offset-2 focus:outline-none disabled:cursor-not-allowed disabled:opacity-50 dark:bg-[#7BA3CC] dark:text-[#1A1815]"
			>
				<GitCompareArrows class="h-4 w-4" />
				Compare selected{selectionCount >= 2 ? ` (${selectionCount})` : ''}
			</button>
		{/if}
	</header>

	<form class="grid gap-3 md:grid-cols-[12rem_1fr_1fr]" aria-label="Efficacy filters">
		<label class="space-y-1 text-[11px] font-bold tracking-[0.05em] uppercase text-[#6B6558] dark:text-[#8E8674]">
			<span>Tier</span>
			<select
				name="tier"
				value={tierFilter}
				onchange={(event) => {
					const value = (event.currentTarget as HTMLSelectElement).value;
					tierFilter = value;
					updateFilter('tier', value);
				}}
				class="w-full border border-[#E4DDD0] bg-[#FFFFFF] px-3 py-2 text-sm text-[#1F2125] focus:border-[#3B5B7A] focus:ring-1 focus:ring-[#3B5B7A] focus:outline-none dark:border-[#34302A] dark:bg-[#1A1815] dark:text-[#EDE6D6]"
			>
				<option value="">All tiers</option>
				<option value="cheap">Cheap</option>
				<option value="balanced">Balanced</option>
				<option value="frontier">Frontier</option>
			</select>
		</label>

		<label class="space-y-1 text-[11px] font-bold tracking-[0.05em] uppercase text-[#6B6558] dark:text-[#8E8674]">
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
				class="w-full border border-[#E4DDD0] bg-[#FFFFFF] px-3 py-2 text-sm text-[#1F2125] focus:border-[#3B5B7A] focus:ring-1 focus:ring-[#3B5B7A] focus:outline-none dark:border-[#34302A] dark:bg-[#1A1815] dark:text-[#EDE6D6]"
			/>
		</label>

		<label class="space-y-1 text-[11px] font-bold tracking-[0.05em] uppercase text-[#6B6558] dark:text-[#8E8674]">
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
				class="w-full border border-[#E4DDD0] bg-[#FFFFFF] px-3 py-2 text-sm text-[#1F2125] focus:border-[#3B5B7A] focus:ring-1 focus:ring-[#3B5B7A] focus:outline-none dark:border-[#34302A] dark:bg-[#1A1815] dark:text-[#EDE6D6]"
			/>
		</label>
	</form>

	<div class="overflow-hidden border border-[#E4DDD0] dark:border-[#34302A]">
		<table aria-label="Efficacy table" class="w-full text-sm">
			<thead>
				<tr class="border-b border-[#E4DDD0] bg-[#F4EFE6] dark:border-[#34302A] dark:bg-[#26231F]">
					<th scope="col" class="w-10 px-4 py-3 text-left">
						<span class="sr-only">Select row for comparison</span>
					</th>
					<th class="px-4 py-3 text-left text-[11px] font-bold tracking-[0.05em] uppercase text-[#6B6558] dark:text-[#8E8674]">Harness</th>
					<th class="px-4 py-3 text-left text-[11px] font-bold tracking-[0.05em] uppercase text-[#6B6558] dark:text-[#8E8674]">Provider</th>
					<th class="px-4 py-3 text-left text-[11px] font-bold tracking-[0.05em] uppercase text-[#6B6558] dark:text-[#8E8674]">Model</th>
					<th class="px-4 py-3 text-right text-[11px] font-bold tracking-[0.05em] uppercase text-[#6B6558] dark:text-[#8E8674]">Attempts</th
					>
					<th class="px-4 py-3 text-right text-[11px] font-bold tracking-[0.05em] uppercase text-[#6B6558] dark:text-[#8E8674]">
						Success rate
					</th>
					<th class="px-4 py-3 text-right text-[11px] font-bold tracking-[0.05em] uppercase text-[#6B6558] dark:text-[#8E8674]">
						Tokens
					</th>
					<th class="px-4 py-3 text-right text-[11px] font-bold tracking-[0.05em] uppercase text-[#6B6558] dark:text-[#8E8674]">
						Duration
					</th>
					<th class="px-4 py-3 text-right text-[11px] font-bold tracking-[0.05em] uppercase text-[#6B6558] dark:text-[#8E8674]">Cost</th>
				</tr>
			</thead>
			<tbody>
				{#each filteredRows as row (rowKey(row))}
					<tr
						onclick={() => openAttempts(row)}
						class="cursor-pointer border-b border-[#E4DDD0] last:border-0 hover:bg-[#3B5B7A]/5 dark:border-[#34302A] dark:hover:bg-[#7BA3CC]/5"
					>
						<td class="px-4 py-3" onclick={(e) => e.stopPropagation()}>
							<input
								type="checkbox"
								aria-label={`Select ${row.harness} / ${row.provider} / ${row.model} for comparison`}
								checked={selectedRowKeys.has(rowKey(row))}
								onchange={(e) =>
									toggleRowSelection(row, (e.currentTarget as HTMLInputElement).checked)}
								class="h-4 w-4 cursor-pointer border-[#E4DDD0] text-[#3B5B7A] focus:ring-[#3B5B7A] dark:border-[#34302A]"
							/>
						</td>
						<td class="px-4 py-3 font-medium text-[#1F2125] dark:text-[#EDE6D6]">{row.harness}</td>
						<td class="px-4 py-3 text-[#6B6558] dark:text-[#8E8674]">{row.provider}</td>
						<td class="px-4 py-3 text-[#1F2125] dark:text-[#EDE6D6]">
							<div class="flex items-center gap-2">
								<span class="font-mono text-xs">{row.model}</span>
								{#if row.warning}
									<span class="relative inline-flex">
										<svg
											role="img"
											aria-label="below adaptive floor"
											viewBox="0 0 20 20"
											class="warning-badge h-4 w-4 text-[#A8801F] focus:outline-none dark:text-[#D4A53D]"
										>
											<path
												fill="currentColor"
												d="M9.1 2.6a1 1 0 0 1 1.8 0l7.2 13.1A1 1 0 0 1 17.2 17H2.8a1 1 0 0 1-.9-1.3L9.1 2.6Zm.1 4.2.2 5.2h1.2l.2-5.2H9.2Zm.8 8.1a1 1 0 1 0 0-2.1 1 1 0 0 0 0 2.1Z"
											/>
										</svg>
										<span
											role="tooltip"
											class="warning-tooltip absolute top-6 left-1/2 z-20 hidden w-64 -translate-x-1/2 border border-[#A8801F]/30 bg-[#FFFFFF] p-3 text-xs leading-5 text-[#1F2125] dark:border-[#D4A53D]/30 dark:bg-[#2E2A25] dark:text-[#EDE6D6]"
										>
											Below adaptive floor threshold
											{#if row.warning.threshold !== null}
												({formatPercent(row.warning.threshold)}).
											{/if}
											<a
												class="ml-1 font-medium text-[#3B5B7A] underline dark:text-[#7BA3CC]"
												href="/docs/routing-metrics"
											>
												Routing metrics
											</a>
										</span>
									</span>
								{/if}
							</div>
						</td>
						<td class="px-4 py-3 text-right text-[#6B6558] tabular-nums dark:text-[#8E8674]">
							{row.attempts}
						</td>
						<td class="px-4 py-3 text-right text-[#6B6558] tabular-nums dark:text-[#8E8674]">
							{row.successes}/{row.attempts} · {formatPercent(row.successRate)}
						</td>
						<td class="px-4 py-3 text-right font-mono text-xs text-[#6B6558] dark:text-[#8E8674]">
							{formatTokens(row.medianInputTokens, row.medianOutputTokens)}
						</td>
						<td class="px-4 py-3 text-right text-[#6B6558] tabular-nums dark:text-[#8E8674]">
							{formatDuration(row.medianDurationMs)}
						</td>
						<td class="px-4 py-3 text-right text-[#6B6558] tabular-nums dark:text-[#8E8674]">
							{formatCost(row.medianCostUsd)}
						</td>
					</tr>
				{/each}
				{#if filteredRows.length === 0}
					<tr>
						<td colspan="9" class="px-4 py-8 text-center text-[#6B6558] dark:text-[#8E8674]">
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
			class="border border-[#E4DDD0] p-4 dark:border-[#34302A]"
		>
			<div class="mb-3 flex items-center justify-between">
				<h2 class="text-base font-semibold text-[#1F2125] dark:text-[#EDE6D6]">Comparisons</h2>
				<span class="text-xs text-[#6B6558] dark:text-[#8E8674]"
					>{comparisonResults.length} records</span
				>
			</div>
			{#if comparisonResults.length > 0}
				<ul class="divide-y divide-[#E4DDD0] dark:divide-[#34302A]">
					{#each comparisonResults as comparison (comparison.id)}
						<li class="flex items-center justify-between gap-3 py-2">
							<a
								href={`/comparisons/${comparison.id}`}
								class="font-mono text-sm font-medium text-[#3B5B7A] hover:underline dark:text-[#7BA3CC]"
							>
								{comparison.id}
							</a>
							<span class="text-xs text-[#6B6558] dark:text-[#8E8674]">
								{comparison.state} · {comparison.armCount} arms
							</span>
						</li>
					{/each}
				</ul>
			{:else}
				<p class="text-sm text-[#6B6558] dark:text-[#8E8674]">No comparisons yet.</p>
			{/if}
		</section>

		{#if selectedRowKey}
			<aside
				aria-label="Attempts detail"
				class="border border-[#E4DDD0] p-4 dark:border-[#34302A]"
			>
				<h2 class="text-base font-semibold text-[#1F2125] dark:text-[#EDE6D6]">Last 10 attempts</h2>
				<p class="mt-1 mb-3 text-xs text-[#6B6558] dark:text-[#8E8674]">{selectedRowLabel}</p>
				{#if attemptsLoading}
					<p class="text-sm text-[#6B6558] dark:text-[#8E8674]">Loading attempts...</p>
				{:else}
					<table class="w-full text-sm">
						<thead>
							<tr class="border-b border-[#E4DDD0] dark:border-[#34302A]">
								<th class="py-2 pr-3 text-left text-[11px] font-bold tracking-[0.05em] uppercase text-[#6B6558] dark:text-[#8E8674]"
									>Bead</th
								>
								<th class="px-3 py-2 text-left text-[11px] font-bold tracking-[0.05em] uppercase text-[#6B6558] dark:text-[#8E8674]">
									Outcome
								</th>
								<th class="px-3 py-2 text-right text-[11px] font-bold tracking-[0.05em] uppercase text-[#6B6558] dark:text-[#8E8674]">
									Cost
								</th>
							</tr>
						</thead>
						<tbody>
							{#each attempts as attempt (attempt.beadId)}
								<tr class="border-b border-[#E4DDD0] last:border-0 dark:border-[#34302A]">
									<td class="py-2 pr-3 align-top">
										<a
											href={beadHref(attempt.beadId)}
											class="font-mono text-xs font-medium text-[#3B5B7A] hover:underline dark:text-[#7BA3CC]"
										>
											{attempt.beadId}
										</a>
										<a
											href={attempt.evidenceBundleUrl}
											class="mt-1 flex items-center gap-1 text-xs text-[#6B6558] hover:text-[#1F2125] hover:underline dark:text-[#8E8674] dark:hover:text-[#EDE6D6]"
										>
											<Link2 class="h-3 w-3" />
											Evidence bundle
										</a>
									</td>
									<td class="px-3 py-2 align-top text-[#6B6558] dark:text-[#8E8674]">
										{attempt.outcome}
										<div class="text-xs text-[#6B6558] dark:text-[#8E8674]">
											{formatDuration(attempt.durationMs)}
										</div>
									</td>
									<td
										class="px-3 py-2 text-right align-top text-[#6B6558] tabular-nums dark:text-[#8E8674]"
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
	<div class="fixed inset-0 z-40 bg-[#1F2125]/40" aria-hidden="true"></div>
	<div class="fixed inset-0 z-50 grid place-items-center p-4">
		<dialog
			open
			aria-modal="true"
			aria-labelledby="compare-title"
			class="max-h-[90vh] w-full max-w-2xl overflow-auto bg-[#FFFFFF] p-5 dark:bg-[#2E2A25]"
		>
			<div class="mb-4 flex items-center justify-between gap-3">
				<h2 id="compare-title" class="text-lg font-semibold text-[#1F2125] dark:text-[#EDE6D6]">
					Compare
				</h2>
				<button
					type="button"
					onclick={() => (compareOpen = false)}
					aria-label="Close compare dialog"
					class="p-1 text-[#6B6558] hover:bg-[#E4DDD0]/40 hover:text-[#1F2125] dark:hover:bg-[#34302A] dark:hover:text-[#EDE6D6]"
				>
					<X class="h-5 w-5" />
				</button>
			</div>

			<p class="mb-3 text-sm text-[#6B6558] dark:text-[#8E8674]">
				Comparing {comparisonArms.length} selected rows. They will all run the same prompt.
			</p>

			<ul class="mb-4 space-y-2">
				{#each comparisonArms as arm (`${arm.harness}|${arm.provider}|${arm.model}`)}
					<li
						data-testid="comparison-arm"
						class="flex items-center justify-between gap-3 border border-[#E4DDD0] px-3 py-2 text-sm dark:border-[#34302A]"
					>
						<span class="font-mono text-xs text-[#1F2125] dark:text-[#EDE6D6]">
							{arm.harness} / {arm.provider} / {arm.model}
						</span>
					</li>
				{/each}
			</ul>

			<label class="block space-y-1 text-[11px] font-bold tracking-[0.05em] uppercase text-[#6B6558] dark:text-[#8E8674]">
				<span>Prompt</span>
				<textarea
					name="prompt"
					rows="4"
					value={comparisonPrompt}
					oninput={(event) =>
						setPromptForAllArms((event.currentTarget as HTMLTextAreaElement).value)}
					placeholder="Prompt run against every selected arm"
					class="w-full resize-y border border-[#E4DDD0] bg-[#FFFFFF] px-3 py-2 text-sm text-[#1F2125] focus:border-[#3B5B7A] focus:ring-1 focus:ring-[#3B5B7A] focus:outline-none dark:border-[#34302A] dark:bg-[#1A1815] dark:text-[#EDE6D6]"
				></textarea>
			</label>

			<div class="mt-4 flex items-center justify-end gap-3">
				<button
					type="button"
					onclick={submitComparison}
					disabled={dispatching || comparisonArms.length === 0 || !comparisonPrompt.trim()}
					class="bg-[#3B5B7A] px-3 py-2 text-[11px] font-bold tracking-[0.05em] uppercase text-white hover:opacity-90 disabled:cursor-not-allowed disabled:opacity-50 dark:bg-[#7BA3CC] dark:text-[#1A1815]"
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
