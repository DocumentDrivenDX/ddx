import type { PageLoad } from './$types'
import { createClient } from '$lib/gql/client'
import { gql } from 'graphql-request'

export const PROJECT_RUNS_QUERY = gql`
	query ProjectRuns(
		$projectID: ID!
		$first: Int
		$after: String
		$layer: RunLayer
		$status: String
		$harness: String
	) {
		runs(
			projectID: $projectID
			first: $first
			after: $after
			layer: $layer
			status: $status
			harness: $harness
		) {
			edges {
				node {
					id
					layer
					status
					projectID
					beadId
					startedAt
					durationMs
					harness
					queueInputs
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

export interface RunNode {
	id: string
	layer: string
	status: string
	projectID: string | null
	beadId: string | null
	startedAt: string | null
	durationMs: number | null
	harness: string | null
	queueInputs: string | null
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

interface QueryResult {
	runs: RunConnection
}

export const PAGE_SIZE = 50

export const load: PageLoad = async ({ params, url, fetch }) => {
	const layer = url.searchParams.get('layer') ?? null
	const status = url.searchParams.get('status') ?? null
	const harness = url.searchParams.get('harness') ?? null

	const client = createClient(fetch as unknown as typeof globalThis.fetch)
	const data = await client.request<QueryResult>(PROJECT_RUNS_QUERY, {
		projectID: params.projectId,
		first: PAGE_SIZE,
		layer: layer ?? undefined,
		status: status ?? undefined,
		harness: harness ?? undefined
	})

	return {
		nodeId: params.nodeId,
		projectId: params.projectId,
		runs: data.runs,
		activeLayer: layer,
		activeStatus: status,
		activeHarness: harness
	}
}
