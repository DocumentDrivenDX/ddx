import type { PageLoad } from './$types'
import { createClient } from '$lib/gql/client'
import { gql } from 'graphql-request'

export const RUNS_QUERY = gql`
	query NodeRuns($first: Int, $after: String, $layer: RunLayer, $status: String) {
		runs(first: $first, after: $after, layer: $layer, status: $status) {
			edges {
				node {
					id
					layer
					status
					projectID
					beadId
					startedAt
					durationMs
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

const FEDERATED_RUNS_QUERY = gql`
	query FederatedRuns($layer: RunLayer, $projectID: String) {
		federatedRuns(layer: $layer, projectID: $projectID) {
			nodeId
			projectId
			projectUrl
			writeCapability
			status
			run {
				id
				layer
				status
				projectID
				beadId
				startedAt
				durationMs
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

export interface RunNode {
	id: string
	layer: string
	status: string
	projectID: string | null
	beadId: string | null
	startedAt: string | null
	durationMs: number | null
}

export interface RunEdge {
	node: RunNode
	cursor: string
}

export interface PageInfo {
	hasNextPage: boolean
	endCursor: string | null
}

export interface RunConnection {
	edges: RunEdge[]
	pageInfo: PageInfo
	totalCount: number
}

interface ProjectNode {
	id: string
	name: string
}

interface QueryResult {
	runs: RunConnection
	projects: {
		edges: Array<{ node: ProjectNode }>
	}
}

export interface FederatedRunRow {
	nodeId: string
	projectId: string | null
	projectUrl: string
	writeCapability: boolean
	status: string
	run: RunNode
}

interface FederatedQueryResult {
	federatedRuns: FederatedRunRow[]
	projects: { edges: Array<{ node: ProjectNode }> }
}

export const PAGE_SIZE = 50

export const load: PageLoad = async ({ params, url, fetch }) => {
	const layer = url.searchParams.get('layer') ?? null
	const status = url.searchParams.get('status') ?? null
	const scope = url.searchParams.get('scope') === 'federation' ? 'federation' : 'local'

	const client = createClient(fetch as unknown as typeof globalThis.fetch)

	if (scope === 'federation') {
		try {
			const data = await client.request<FederatedQueryResult>(FEDERATED_RUNS_QUERY, {
				layer: layer ?? undefined
			})
			const projectNames: Record<string, string> = {}
			for (const { node } of data.projects.edges) projectNames[node.id] = node.name

			const filtered = status
				? data.federatedRuns.filter((row) => row.run.status === status)
				: data.federatedRuns

			const edges: RunEdge[] = filtered.map((row, i) => ({
				node: row.run,
				cursor: `fed:${i}`
			}))
			const federationByRunId: Record<string, FederatedRunRow> = {}
			for (const row of filtered) federationByRunId[row.run.id] = row

			return {
				nodeId: params.nodeId,
				runs: {
					edges,
					pageInfo: { hasNextPage: false, endCursor: null },
					totalCount: edges.length
				} as RunConnection,
				projects: data.projects.edges.map((e) => e.node),
				projectNames,
				activeLayer: layer,
				activeStatus: status,
				scope: 'federation' as const,
				federationByRunId,
				federationError: null as string | null
			}
		} catch (e) {
			return {
				nodeId: params.nodeId,
				runs: {
					edges: [] as RunEdge[],
					pageInfo: { hasNextPage: false, endCursor: null },
					totalCount: 0
				} as RunConnection,
				projects: [] as ProjectNode[],
				projectNames: {} as Record<string, string>,
				activeLayer: layer,
				activeStatus: status,
				scope: 'federation' as const,
				federationByRunId: {} as Record<string, FederatedRunRow>,
				federationError: (e as Error).message
			}
		}
	}

	const data = await client.request<QueryResult>(RUNS_QUERY, {
		first: PAGE_SIZE,
		layer: layer ?? undefined,
		status: status ?? undefined
	})

	const projectNames: Record<string, string> = {}
	for (const { node } of data.projects.edges) {
		projectNames[node.id] = node.name
	}

	return {
		nodeId: params.nodeId,
		runs: data.runs,
		projects: data.projects.edges.map((e) => e.node),
		projectNames,
		activeLayer: layer,
		activeStatus: status,
		scope: 'local' as const,
		federationByRunId: {} as Record<string, FederatedRunRow>,
		federationError: null as string | null
	}
}
