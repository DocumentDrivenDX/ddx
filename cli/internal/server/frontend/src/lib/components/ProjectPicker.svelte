<script lang="ts">
	import { onMount } from 'svelte';
	import { goto } from '$app/navigation';
	import { gql } from 'graphql-request';
	import { createClient } from '$lib/gql/client';
	import { nodeStore } from '$lib/stores/node.svelte';
	import { projectStore } from '$lib/stores/project.svelte';

	const PROJECTS_QUERY = gql`
		query Projects {
			projects {
				edges {
					node {
						id
						name
						path
					}
				}
			}
		}
	`;

	interface ProjectNode {
		id: string;
		name: string;
		path: string;
	}

	interface ProjectsResult {
		projects: {
			edges: Array<{ node: ProjectNode }>;
		};
	}

	let projects = $state<ProjectNode[]>([]);
	let loading = $state(true);

	onMount(async () => {
		const client = createClient();
		const data = await client.request<ProjectsResult>(PROJECTS_QUERY);
		projects = data.projects.edges.map((e) => e.node);
		loading = false;
	});

	function handleChange(event: Event) {
		const select = event.target as HTMLSelectElement;
		const projectId = select.value;
		if (!projectId) return;

		const project = projects.find((p) => p.id === projectId);
		if (!project) return;

		projectStore.set({ id: project.id, name: project.name, path: project.path });

		const nodeId = nodeStore.value?.id;
		if (nodeId) {
			goto(`/nodes/${nodeId}/projects/${projectId}`);
		}
	}
</script>

<select
	aria-label="Project"
	class="rounded border border-gray-300 px-3 py-1 text-sm text-gray-700 disabled:text-gray-700 dark:border-gray-600 dark:bg-gray-900 dark:text-gray-300 dark:disabled:text-gray-300"
	value={projectStore.value?.id ?? ''}
	onchange={handleChange}
	disabled={loading}
>
	<option value="">{loading ? 'Loading…' : 'Select project…'}</option>
	{#each projects as project}
		<option value={project.id}>{project.name}</option>
	{/each}
</select>
