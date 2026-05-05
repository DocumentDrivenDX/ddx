import { error } from '@sveltejs/kit';
import type { PageLoad } from './$types';
import { projectStore } from '$lib/stores/project.svelte';

interface ProjectSummary {
	id: string;
	name: string;
	path: string;
}

type ProjectsResponse = ProjectSummary[];

function chooseProject(projects: ProjectSummary[]): ProjectSummary | null {
	return (
		projects.find(
			(project) => /(^|\/)ddx-e2e-/.test(project.path) || /^ddx-e2e-/.test(project.name)
		) ??
		projects[0] ??
		null
	);
}

export const load: PageLoad = async ({ fetch }) => {
	const resp = await fetch('/api/projects');
	if (!resp.ok) {
		throw error(resp.status, 'could not load projects');
	}

	const projects = (await resp.json()) as ProjectsResponse;
	const project = chooseProject(projects);
	if (!project) {
		throw error(404, 'no project available');
	}

	projectStore.set(project);
	return { project };
};
