<script lang="ts">
	import type { PageData } from './$types';
	import { goto } from '$app/navigation';
	import { page } from '$app/stores';
	import { subscribeWorkerProgress } from '$lib/gql/subscriptions';

	let { data }: { data: PageData } = $props();

	let logLines = $state<string[]>([]);
	let logContainer = $state<HTMLPreElement | null>(null);
	let autoScroll = $state(true);

	// Initialize log lines from initial captured stdout
	$effect(() => {
		const raw = data.initialLog ?? '';
		logLines = raw.length > 0 ? raw.split('\n') : [];
	});

	// Auto-scroll to bottom when new lines arrive (if autoScroll is enabled)
	$effect(() => {
		// Depend on logLines length to trigger on each new line
		const _len = logLines.length;
		if (autoScroll && logContainer) {
			// Defer so DOM updates before we measure
			Promise.resolve().then(() => {
				if (logContainer) logContainer.scrollTop = logContainer.scrollHeight;
			});
		}
	});

	// Subscribe to live worker progress events
	$effect(() => {
		const workerId = data.worker?.id;
		if (!workerId) return;

		const dispose = subscribeWorkerProgress(workerId, (evt) => {
			if (evt.logLine != null && evt.logLine.length > 0) {
				logLines = [...logLines, evt.logLine];
			}
		});

		return dispose;
	});

	function handleScroll() {
		if (!logContainer) return;
		const distFromBottom =
			logContainer.scrollHeight - logContainer.scrollTop - logContainer.clientHeight;
		autoScroll = distFromBottom < 20;
	}

	function handleClose() {
		const pathParts = $page.url.pathname.split('/');
		pathParts.pop(); // remove workerId segment
		const basePath = pathParts.join('/');
		goto(basePath);
	}

	function formatElapsed(ms: number): string {
		if (ms < 1000) return `${ms}ms`;
		if (ms < 60000) return `${(ms / 1000).toFixed(1)}s`;
		const m = Math.floor(ms / 60000);
		const s = Math.floor((ms % 60000) / 1000);
		return `${m}m${s}s`;
	}
</script>

{#if data.worker}
	<!-- Backdrop -->
	<div
		class="fixed inset-0 z-40 bg-black/20 dark:bg-black/40"
		onclick={handleClose}
		role="button"
		tabindex="-1"
		aria-label="Close panel"
		onkeydown={(e) => e.key === 'Escape' && handleClose()}
	></div>

	<!-- Detail panel -->
	<div
		class="fixed right-0 top-0 z-50 flex h-full w-full max-w-2xl flex-col bg-white shadow-xl dark:bg-gray-900"
	>
		<!-- Header -->
		<div
			class="flex shrink-0 items-center justify-between border-b border-gray-200 px-6 py-4 dark:border-gray-700"
		>
			<div>
				<h2 class="text-base font-semibold text-gray-900 dark:text-white">
					{data.worker.kind}
				</h2>
				<p class="font-mono text-xs text-gray-500 dark:text-gray-400">{data.worker.id}</p>
			</div>
			<button
				onclick={handleClose}
				class="rounded p-1.5 text-gray-400 hover:bg-gray-100 hover:text-gray-600 dark:hover:bg-gray-800 dark:hover:text-gray-300"
				aria-label="Close"
			>
				✕
			</button>
		</div>

		<!-- Metadata grid -->
		<div
			class="grid shrink-0 grid-cols-2 gap-x-6 gap-y-2 border-b border-gray-200 px-6 py-4 text-sm dark:border-gray-700"
		>
			<div>
				<span class="text-gray-500 dark:text-gray-400">State: </span>
				<span class="font-medium text-gray-900 dark:text-white">{data.worker.state}</span>
			</div>
			{#if data.worker.harness}
				<div>
					<span class="text-gray-500 dark:text-gray-400">Harness: </span>
					<span class="text-gray-900 dark:text-white">{data.worker.harness}</span>
				</div>
			{/if}
			{#if data.worker.model}
				<div>
					<span class="text-gray-500 dark:text-gray-400">Model: </span>
					<span class="text-gray-900 dark:text-white">{data.worker.model}</span>
				</div>
			{/if}
			{#if data.worker.effort}
				<div>
					<span class="text-gray-500 dark:text-gray-400">Effort: </span>
					<span class="text-gray-900 dark:text-white">{data.worker.effort}</span>
				</div>
			{/if}
			{#if data.worker.currentBead}
				<div class="col-span-2">
					<span class="text-gray-500 dark:text-gray-400">Current bead: </span>
					<span class="font-mono text-xs text-gray-900 dark:text-white"
						>{data.worker.currentBead}</span
					>
				</div>
			{/if}
			{#if data.worker.attempts != null}
				<div>
					<span class="text-gray-500 dark:text-gray-400">Attempts: </span>
					<span class="text-gray-900 dark:text-white">
						{data.worker.attempts}
						<span class="text-xs text-gray-500 dark:text-gray-400">
							({data.worker.successes ?? 0}✓ / {data.worker.failures ?? 0}✗)
						</span>
					</span>
				</div>
			{/if}
			{#if data.worker.currentAttempt}
				<div>
					<span class="text-gray-500 dark:text-gray-400">Phase: </span>
					<span class="font-medium text-yellow-600 dark:text-yellow-400">
						{data.worker.currentAttempt.phase}
					</span>
					<span class="ml-1 text-xs text-gray-400 dark:text-gray-500">
						({formatElapsed(data.worker.currentAttempt.elapsedMs)})
					</span>
				</div>
			{/if}
			{#if data.worker.lastError}
				<div class="col-span-2">
					<span class="text-gray-500 dark:text-gray-400">Last error: </span>
					<span class="text-red-600 dark:text-red-400">{data.worker.lastError}</span>
				</div>
			{/if}
		</div>

		<!-- Log area -->
		<div class="flex min-h-0 flex-1 flex-col">
			<div
				class="flex shrink-0 items-center justify-between border-b border-gray-200 px-4 py-2 dark:border-gray-700"
			>
				<span class="text-xs font-medium text-gray-500 dark:text-gray-400">Log output</span>
				<div class="flex items-center gap-3">
					<span class="text-xs text-gray-400 dark:text-gray-500">{logLines.length} lines</span>
					{#if !autoScroll}
						<button
							onclick={() => {
								autoScroll = true;
								if (logContainer) logContainer.scrollTop = logContainer.scrollHeight;
							}}
							class="rounded px-2 py-0.5 text-xs text-blue-600 hover:bg-blue-50 dark:text-blue-400 dark:hover:bg-blue-900/20"
						>
							↓ Follow
						</button>
					{/if}
				</div>
			</div>
			<!-- svelte-ignore a11y_no_noninteractive_element_interactions -->
			<pre
				bind:this={logContainer}
				onscroll={handleScroll}
				class="flex-1 overflow-auto bg-gray-950 px-4 py-3 font-mono text-xs leading-relaxed text-green-400 dark:text-green-300"
			>{#if logLines.length === 0}<span class="text-gray-600 dark:text-gray-500"
						>No log output yet…</span
					>{:else}{logLines.join('\n')}{/if}</pre>
		</div>
	</div>
{/if}
