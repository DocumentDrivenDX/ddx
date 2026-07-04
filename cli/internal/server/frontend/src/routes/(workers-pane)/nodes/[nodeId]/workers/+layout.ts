import type { LayoutLoad } from './$types'
import { createClient } from '$lib/gql/client'
import { gql } from 'graphql-request'

const NODE_WORKERS_QUERY = gql`
	query NodeWideWorkers($first: Int) {
		workers(first: $first) {
			edges {
				node {
					id
					kind
					state
					status
					harness
					model
					projectRoot
					currentBead
					attempts
					successes
					failures
					startedAt
				}
				cursor
			}
			pageInfo {
				hasNextPage
				endCursor
			}
			totalCount
		}
	}
`

const PROJECTS_QUERY = gql`
	query ProjectsForWorkerPane {
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
`

const FEDERATION_NODES_QUERY = gql`
	query FederationNodesForWorkerPane {
		federationNodes {
			nodeId
			name
			status
		}
	}
`

export interface WorkerNode {
	id: string
	kind: string
	state: string
	status: string | null
	harness: string | null
	model: string | null
	projectRoot?: string | null
	projectId?: string | null
	projectName?: string | null
	nodeId?: string | null
	nodeName?: string | null
	currentBead: string | null
	attempts: number | null
	successes: number | null
	failures: number | null
	startedAt: string | null
	// Optional fields that may be present in mocked or extended responses
}

export interface WorkerEdge {
	node: WorkerNode
	cursor: string
}

export interface WorkerConnection {
	edges: WorkerEdge[]
	pageInfo: { hasNextPage: boolean; endCursor: string | null }
	totalCount: number
}

interface WorkersResult {
	workers: WorkerConnection
}

interface FederatedWorkersResult {
	workers: WorkerConnection
}

interface ProjectNode {
	id: string
	name: string
	path: string
	nodeId?: string | null
}

interface ProjectsResult {
	projects: {
		edges: Array<{ node: ProjectNode }>
	}
}

interface FederationNode {
	nodeId: string
	name: string
	status: string
}

interface FederationNodesResult {
	federationNodes: FederationNode[]
}

const EMPTY_CONNECTION: WorkerConnection = {
	edges: [],
	pageInfo: { hasNextPage: false, endCursor: null },
	totalCount: 0
}

export const load: LayoutLoad = async ({ params, url, fetch }) => {
	const scope = url.searchParams.get('scope') === 'federation' ? 'federation' : 'local'
	const client = createClient(fetch as unknown as typeof globalThis.fetch)

	if (scope === 'federation') {
		const [workersData, projectsData, federationNodesData] = await Promise.all([
			client
				.request<FederatedWorkersResult>(NODE_WORKERS_QUERY, { first: 100 })
				.catch(() => ({ workers: EMPTY_CONNECTION })),
			client
				.request<ProjectsResult>(PROJECTS_QUERY)
				.catch(() => ({ projects: { edges: [] as Array<{ node: ProjectNode }> } })),
			client
				.request<FederationNodesResult>(FEDERATION_NODES_QUERY)
				.catch(() => ({ federationNodes: [] as FederationNode[] }))
		])
		const projectsByPath: Record<string, { id: string; name: string; nodeId: string | null }> = {}
		const projectsById: Record<string, { id: string; name: string; path: string; nodeId: string | null }> = {}
		for (const { node } of projectsData.projects.edges) {
			const entry = { id: node.id, name: node.name, path: node.path, nodeId: node.nodeId ?? null }
			projectsById[node.id] = entry
			if (node.path) {
				projectsByPath[node.path] = { id: node.id, name: node.name, nodeId: node.nodeId ?? null }
			}
		}
		const federationNodesById: Record<string, FederationNode> = {}
		for (const node of federationNodesData.federationNodes ?? []) {
			federationNodesById[node.nodeId] = node
		}
		return {
			nodeId: params.nodeId,
			workers: workersData.workers ?? EMPTY_CONNECTION,
			scope: 'federation' as const,
			projectsByPath,
			projectsById,
			federationNodesById
		}
	}

	const [workersData, projectsData] = await Promise.all([
		client
			.request<WorkersResult>(NODE_WORKERS_QUERY, { first: 100 })
			.catch(() => ({
				workers: {
					edges: [] as WorkerEdge[],
					pageInfo: { hasNextPage: false, endCursor: null },
					totalCount: 0
				}
			})),
		client
			.request<ProjectsResult>(PROJECTS_QUERY)
			.catch(() => ({ projects: { edges: [] as Array<{ node: ProjectNode }> } }))
	])

	const projectsByPath: Record<string, { id: string; name: string }> = {}
	for (const { node } of projectsData.projects.edges) {
		if (node.path) {
			projectsByPath[node.path] = { id: node.id, name: node.name }
		}
	}

	return {
		nodeId: params.nodeId,
		workers: workersData.workers,
		scope: 'local' as const,
		projectsByPath
	}
}
