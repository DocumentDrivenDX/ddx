import type { PageLoad } from './$types';
import { createClient } from '$lib/gql/client';
import { gql } from 'graphql-request';

const PERSONAS_QUERY = gql`
	query Personas {
		personas {
			id
			name
			roles
			description
			tags
			content
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
	content: string;
	body: string;
	source: string;
	bindings: PersonaBinding[];
	filePath: string | null;
	modTime: string | null;
}

interface PersonasResult {
	personas: PersonaNode[];
}

export const load: PageLoad = async ({ params, fetch }) => {
	const client = createClient(fetch as unknown as typeof globalThis.fetch);
	const data = await client.request<PersonasResult>(PERSONAS_QUERY);
	return {
		projectId: params.projectId,
		personas: data.personas
	};
};
