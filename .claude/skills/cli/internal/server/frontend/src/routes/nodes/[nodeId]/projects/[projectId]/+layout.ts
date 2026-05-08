import type { LayoutLoad } from './$types';
import { gql } from 'graphql-request';
import { error } from '@sveltejs/kit';
import { createClient } from '$lib/gql/client';
import { projectStore } from '$lib/stores/project.svelte';

const PROJECTS_QUERY = gql`
	query ProjectsForLayout {
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

// Hydrate projectStore from params.projectId on every navigation into this
// route subtree so deep-links and page reloads see a populated store. Without
// this, the sidebar at NavShell.svelte disables every link because
// projectStore.value is null — only ProjectPicker.onchange would set it.
export const load: LayoutLoad = async ({ params, fetch }) => {
	const client = createClient(fetch as unknown as typeof globalThis.fetch);
	const data = await client.request<ProjectsResult>(PROJECTS_QUERY);
	const project = data.projects.edges
		.map((e) => e.node)
		.find((p) => p.id === params.projectId);
	if (!project) {
		throw error(404, `project ${params.projectId} not found`);
	}
	projectStore.set(project);
	return { project };
};
