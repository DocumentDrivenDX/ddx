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

export const PAGE_SIZE = 50

export const load: PageLoad = async ({ params, url, fetch }) => {
	const layer = url.searchParams.get('layer') ?? null
	const status = url.searchParams.get('status') ?? null

	const client = createClient(fetch as unknown as typeof globalThis.fetch)
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
		activeStatus: status
	}
}
