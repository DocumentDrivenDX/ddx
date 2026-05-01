import { createClient } from '$lib/gql/client';
import { gql } from 'graphql-request';

export const PERSONAS_QUERY = gql`
	query Personas($projectId: String) {
		personas(projectId: $projectId) {
			id
			name
			roles
			description
			tags
			body
			source
			bindings {
				projectId
				role
				persona
			}
			filePath
			modTime
		}
	}
`;

export const PERSONA_CREATE_MUTATION = gql`
	mutation PersonaCreate($name: String!, $body: String!, $projectId: String!) {
		personaCreate(name: $name, body: $body, projectId: $projectId) {
			id
			name
			source
		}
	}
`;

export const PERSONA_UPDATE_MUTATION = gql`
	mutation PersonaUpdate($name: String!, $body: String!, $projectId: String!) {
		personaUpdate(name: $name, body: $body, projectId: $projectId) {
			id
			name
			source
		}
	}
`;

export const PERSONA_DELETE_MUTATION = gql`
	mutation PersonaDelete($name: String!, $projectId: String!) {
		personaDelete(name: $name, projectId: $projectId) {
			ok
			name
		}
	}
`;

export const PERSONA_FORK_MUTATION = gql`
	mutation PersonaFork($libraryName: String!, $newName: String, $projectId: String!) {
		personaFork(libraryName: $libraryName, newName: $newName, projectId: $projectId) {
			id
			name
			source
		}
	}
`;

export interface PersonaBinding {
	projectId: string;
	role: string;
	persona: string;
}

export interface PersonaNode {
	id: string;
	name: string;
	roles: string[];
	description: string;
	tags: string[];
	body: string;
	source: string;
	bindings: PersonaBinding[];
	filePath: string | null;
	modTime: string | null;
}

export interface ProjectOption {
	id: string;
	name: string;
	path: string;
}

interface PersonasResult {
	personas: PersonaNode[];
}

export interface PersonasPageData {
	projectId: string;
	selectedName: string | null;
	personas: PersonaNode[];
}

export async function loadPersonas(
	fetchFn: typeof globalThis.fetch,
	projectId: string,
	selectedName: string | null
): Promise<PersonasPageData> {
	const client = createClient(fetchFn);
	const data = await client.request<PersonasResult>(PERSONAS_QUERY, { projectId });
	return {
		projectId,
		selectedName,
		personas: data.personas
	};
}
