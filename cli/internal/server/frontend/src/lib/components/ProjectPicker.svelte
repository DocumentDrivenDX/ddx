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
		try {
			const [data, current] = await Promise.all([
				client.request<ProjectsResult>(PROJECTS_QUERY),
				fetch('/api/projects/current')
					.then((resp) => (resp.ok ? (resp.json() as Promise<ProjectNode>) : null))
					.catch(() => null)
			]);
			projects = data.projects.edges.map((e) => e.node);
			if (current) {
				projects = [current, ...projects.filter((p) => p.id !== current.id)];
			}
			if (!projectStore.value && current) {
				selectProject(current, false);
			}
		} finally {
			loading = false;
		}
	});

	function selectProject(project: ProjectNode, navigate: boolean) {
		projectStore.set({ id: project.id, name: project.name, path: project.path });

		const nodeId = nodeStore.value?.id;
		if (navigate && nodeId) {
			goto(`/nodes/${nodeId}/projects/${project.id}`);
		}
	}

	function handleChange(event: Event) {
		const select = event.target as HTMLSelectElement;
		const projectId = select.value;
		if (!projectId) return;

		const project = projects.find((p) => p.id === projectId);
		if (!project) return;

		selectProject(project, true);
	}
</script>

<select
	aria-label="Project"
	class="rounded-none border border-border-line px-3 py-1 text-sm text-fg-ink disabled:text-fg-ink dark:border-dark-border-line dark:bg-dark-bg-canvas dark:text-dark-fg-ink dark:disabled:text-dark-fg-ink"
	value={projectStore.value?.id ?? ''}
	onchange={handleChange}
	disabled={loading}
>
	<option value="">{loading ? 'Loading…' : 'Select project…'}</option>
	{#each projects as project}
		<option value={project.id}>{project.name}</option>
	{/each}
</select>
