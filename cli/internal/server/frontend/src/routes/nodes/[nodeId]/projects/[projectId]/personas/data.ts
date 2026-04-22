import { createClient } from '$lib/gql/client';
import { gql } from 'graphql-request';

export const PERSONAS_QUERY = gql`
	query Personas {
		personas {
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
	const data = await client.request<PersonasResult>(PERSONAS_QUERY);
	return {
		projectId,
		selectedName,
		personas: data.personas
	};
}
