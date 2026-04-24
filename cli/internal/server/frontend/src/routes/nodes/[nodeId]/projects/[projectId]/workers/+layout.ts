import type { LayoutLoad } from './$types'
import { createClient } from '$lib/gql/client'
import { gql } from 'graphql-request'

const WORKERS_QUERY = gql`
	query WorkersByProject($projectID: String!) {
		workersByProject(projectID: $projectID, first: 50) {
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
		queueAndWorkersSummary(projectId: $projectID) {
			maxCount
		}
	}
`

interface WorkerNode {
	id: string
	kind: string
	state: string
	status: string | null
	harness: string | null
	model: string | null
	currentBead: string | null
	attempts: number | null
	successes: number | null
	failures: number | null
	startedAt: string | null
}

interface WorkerEdge {
	node: WorkerNode
	cursor: string
}

interface WorkerConnection {
	edges: WorkerEdge[]
	pageInfo: { hasNextPage: boolean; endCursor: string | null }
	totalCount: number
}

interface WorkersResult {
	workersByProject: WorkerConnection
	queueAndWorkersSummary: { maxCount: number | null }
}

export const load: LayoutLoad = async ({ params, fetch }) => {
	const client = createClient(fetch as unknown as typeof globalThis.fetch)
	const data = await client.request<WorkersResult>(WORKERS_QUERY, {
		projectID: params.projectId
	})
	return {
		projectId: params.projectId,
		workers: data.workersByProject,
		maxCount: data.queueAndWorkersSummary?.maxCount ?? null
	}
}
