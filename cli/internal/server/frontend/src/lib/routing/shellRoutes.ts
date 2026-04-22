import { gql } from 'graphql-request';
import { createClient } from '$lib/gql/client';

const SHELL_ROUTE_DEFAULTS_QUERY = gql`
	query ShellRouteDefaults {
		nodeInfo {
			id
		}
		projects {
			edges {
				node {
					id
				}
			}
		}
	}
`;

interface ShellRouteDefaultsResult {
	nodeInfo: {
		id: string;
	};
	projects: {
		edges: Array<{
			node: {
				id: string;
			};
		}>;
	};
}

export type ProjectShellSection = 'beads' | 'documents' | 'graph' | 'sessions' | 'personas';

export async function resolveDefaultProjectRoute(
	section: ProjectShellSection,
	fetchFn: typeof globalThis.fetch
): Promise<string> {
	const client = createClient(fetchFn);
	const data = await client.request<ShellRouteDefaultsResult>(SHELL_ROUTE_DEFAULTS_QUERY);
	const nodeId = data.nodeInfo.id;
	const projectId = data.projects.edges[0]?.node.id;

	if (!projectId) {
		return `/nodes/${nodeId}`;
	}

	return `/nodes/${nodeId}/projects/${projectId}/${section}`;
}
