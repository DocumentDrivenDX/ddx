import type { PageLoad } from './$types'
import { createClient } from '$lib/gql/client'
import { gql } from 'graphql-request'

const BEADS_QUERY = gql`
	query BeadsAllProjects($first: Int, $after: String, $status: String, $label: String, $projectID: String) {
		beads(first: $first, after: $after, status: $status, label: $label, projectID: $projectID) {
			edges {
				node {
					id
					title
					status
					priority
					labels
					projectID
				}
				cursor
			}
			pageInfo {
				hasNextPage
				endCursor
			}
			totalCount
		}
		projects {
			edges {
				node {
					id
					name
				}
			}
		}
	}
`

const FEDERATED_BEADS_QUERY = gql`
	query FederatedBeads($status: String, $label: String, $projectID: String) {
		federatedBeads(status: $status, label: $label, projectID: $projectID) {
			nodeId
			projectId
			projectUrl
			writeCapability
			status
			bead {
				id
				title
				status
				priority
				labels
				projectID
			}
		}
		projects {
			edges {
				node {
					id
					name
				}
			}
		}
	}
`

interface BeadNode {
	id: string
	title: string
	status: string
	priority: number
	labels: string[] | null
	projectID: string | null
}

interface BeadEdge {
	node: BeadNode
	cursor: string
}

interface PageInfo {
	hasNextPage: boolean
	endCursor: string | null
}

interface BeadConnection {
	edges: BeadEdge[]
	pageInfo: PageInfo
	totalCount: number
}

interface ProjectNode {
	id: string
	name: string
}

interface LocalQueryResult {
	beads: BeadConnection
	projects: { edges: Array<{ node: ProjectNode }> }
}

interface FederatedBeadRow {
	nodeId: string
	projectId: string | null
	projectUrl: string
	writeCapability: boolean
	status: string
	bead: BeadNode
}

interface FederatedQueryResult {
	federatedBeads: FederatedBeadRow[]
	projects: { edges: Array<{ node: ProjectNode }> }
}

export const load: PageLoad = async ({ url, fetch }) => {
	const status = url.searchParams.get('status') ?? undefined
	const label = url.searchParams.get('label') ?? undefined
	const projectID = url.searchParams.get('project') ?? undefined
	const scope = url.searchParams.get('scope') === 'federation' ? 'federation' : 'local'

	const client = createClient(fetch as unknown as typeof globalThis.fetch)

	if (scope === 'federation') {
		try {
			const data = await client.request<FederatedQueryResult>(FEDERATED_BEADS_QUERY, {
				status,
				label,
				projectID
			})
			const projectNames: Record<string, string> = {}
			for (const { node } of data.projects.edges) projectNames[node.id] = node.name

			// Adapt federated rows into the same edge shape the page expects, with extra federation metadata.
			const edges: BeadEdge[] = data.federatedBeads.map((row, i) => ({
				node: row.bead,
				cursor: `fed:${i}`
			}))
			const federationByBeadId: Record<string, FederatedBeadRow> = {}
			for (const row of data.federatedBeads) federationByBeadId[row.bead.id] = row

			return {
				beads: {
					edges,
					pageInfo: { hasNextPage: false, endCursor: null },
					totalCount: edges.length
				},
				projects: data.projects.edges.map((e) => e.node),
				projectNames,
				activeStatus: status ?? null,
				activeLabel: label ?? null,
				activeProject: projectID ?? null,
				scope: 'federation' as const,
				federationByBeadId,
				federationError: null as string | null
			}
		} catch (e) {
			return {
				beads: {
					edges: [] as BeadEdge[],
					pageInfo: { hasNextPage: false, endCursor: null },
					totalCount: 0
				},
				projects: [] as ProjectNode[],
				projectNames: {} as Record<string, string>,
				activeStatus: status ?? null,
				activeLabel: label ?? null,
				activeProject: projectID ?? null,
				scope: 'federation' as const,
				federationByBeadId: {} as Record<string, FederatedBeadRow>,
				federationError: (e as Error).message
			}
		}
	}

	const data = await client.request<LocalQueryResult>(BEADS_QUERY, {
		first: 20,
		status,
		label,
		projectID
	})

	const projectNames: Record<string, string> = {}
	for (const { node } of data.projects.edges) projectNames[node.id] = node.name

	return {
		beads: data.beads,
		projects: data.projects.edges.map((e) => e.node),
		projectNames,
		activeStatus: status ?? null,
		activeLabel: label ?? null,
		activeProject: projectID ?? null,
		scope: 'local' as const,
		federationByBeadId: {} as Record<string, FederatedBeadRow>,
		federationError: null as string | null
	}
}
