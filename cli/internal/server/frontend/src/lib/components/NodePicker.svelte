<script lang="ts">
	import { onMount } from 'svelte';
	import { goto } from '$app/navigation';
	import { gql } from 'graphql-request';
	import { createClient } from '$lib/gql/client';
	import { nodeStore } from '$lib/stores/node.svelte';
	import { federationBadgeClass } from '$lib/federationStatus';

	const FEDERATION_NODES_QUERY = gql`
		query NavFederationNodes {
			federationNodes {
				id
				nodeId
				name
				url
				status
			}
		}
	`;

	interface FederationNodeLite {
		id: string;
		nodeId: string;
		name: string;
		url: string;
		status: string;
	}

	interface Result {
		federationNodes: FederationNodeLite[];
	}

	let nodes = $state<FederationNodeLite[]>([]);
	let loading = $state(true);

	onMount(async () => {
		try {
			const client = createClient();
			const data = await client.request<Result>(FEDERATION_NODES_QUERY);
			nodes = data.federationNodes;
		} catch {
			nodes = [];
		} finally {
			loading = false;
		}
	});

	function handleChange(event: Event) {
		const select = event.target as HTMLSelectElement;
		const value = select.value;
		if (!value) return;
		if (value === '__local__') {
			const localId = nodeStore.value?.id;
			if (localId) goto(`/nodes/${localId}`);
			return;
		}
		const node = nodes.find((n) => n.nodeId === value);
		if (!node) return;
		// Spoke deep-link — open in new tab so the local hub session is preserved
		window.open(node.url, '_blank', 'noopener,noreferrer');
	}

	let activeStatus = $derived(
		(() => {
			const id = nodeStore.value?.id;
			if (!id) return null;
			const match = nodes.find((n) => n.nodeId === id);
			return match?.status ?? null;
		})()
	);
</script>

<div class="flex items-center gap-1" data-testid="node-picker">
	<select
		aria-label="Federation node"
		class="rounded-none border border-border-line px-3 py-1 text-sm text-fg-ink dark:border-dark-border-line dark:bg-dark-bg-canvas dark:text-dark-fg-ink"
		value=""
		onchange={handleChange}
		disabled={loading}
	>
		<option value=""
			>{loading ? 'Loading…' : nodes.length === 0 ? 'No federated nodes' : 'Switch node…'}</option
		>
		{#if nodeStore.value}
			<option value="__local__">{nodeStore.value.name} (local)</option>
		{/if}
		{#each nodes as node}
			<option value={node.nodeId}>{node.name} — {node.status}</option>
		{/each}
	</select>
	{#if activeStatus}
		<span
			data-testid="node-picker-status-badge"
			class="inline-block border px-1.5 py-0.5 font-mono-code text-[10px] uppercase {federationBadgeClass(
				activeStatus
			)}"
		>
			{activeStatus}
		</span>
	{/if}
</div>
