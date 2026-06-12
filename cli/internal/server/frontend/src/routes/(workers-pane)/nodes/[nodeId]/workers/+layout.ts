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

const FEDERATED_WORKERS_QUERY = gql`
	query NodeWorkersFederation($first: Int) {
		workers(first: $first) {
			edges {
				node {
					id
					kind
					state
					status
					harness
					model
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

export interface WorkerNode {
	id: string
	kind: string
	state: string
	status: string | null
	harness: string | null
	model: string | null
	projectRoot?: string | null
	currentBead: string | null
	attempts: number | null
	successes: number | null
	failures: number | null
	startedAt: string | null
	// Optional fields that may be present in mocked or extended responses
	projectId?: string | null
	projectName?: string | null
	nodeName?: string | null
	nodeId?: string | null
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
	federatedWorkers: WorkerConnection
}

interface ProjectNode {
	id: string
	name: string
	path: string
}

interface ProjectsResult {
	projects: {
		edges: Array<{ node: ProjectNode }>
	}
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
		const data = await client
			.request<FederatedWorkersResult>(FEDERATED_WORKERS_QUERY, { first: 100 })
			.catch(() => ({ federatedWorkers: EMPTY_CONNECTION }))
		return {
			nodeId: params.nodeId,
			workers: data.federatedWorkers ?? EMPTY_CONNECTION,
			scope: 'federation' as const,
			projectsByPath: {} as Record<string, { id: string; name: string }>
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
