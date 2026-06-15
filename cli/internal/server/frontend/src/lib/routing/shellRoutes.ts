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

export type ProjectShellSection =
	| 'artifacts'
	| 'beads'
	| 'documents'
	| 'graph'
	| 'sessions'
	| 'personas'
	| 'plugins';

type ProjectShellRoute = ProjectShellSection | 'runs';

export async function resolveDefaultProjectRoute(
	section: ProjectShellRoute,
	fetchFn: typeof globalThis.fetch
): Promise<string> {
	const client = createClient(fetchFn);
	const data = await client.request<ShellRouteDefaultsResult>(SHELL_ROUTE_DEFAULTS_QUERY);
	const nodeId = data.nodeInfo.id;
	const projectId = data.projects.edges[0]?.node.id;

	if (!projectId) {
		return `/nodes/${nodeId}`;
	}

	if (section === 'runs') {
		return `/nodes/${nodeId}/projects/${projectId}/runs?layer=run`;
	}

	return `/nodes/${nodeId}/projects/${projectId}/${section}`;
}

export const projectNavPages = [
	{ page: '', label: 'Overview', exact: true },
	{ page: 'beads', label: 'Beads' },
	{ page: 'artifacts', label: 'Artifacts' },
	{ page: 'graph', label: 'Graph' },
	{ page: 'runs', label: 'Runs' },
	{ page: 'workers', label: 'Workers' },
	{ page: 'personas', label: 'Personas' },
	{ page: 'plugins', label: 'Plugins' },
	{ page: 'commits', label: 'Commits' },
	{ page: 'efficacy', label: 'Efficacy' }
] as const;
