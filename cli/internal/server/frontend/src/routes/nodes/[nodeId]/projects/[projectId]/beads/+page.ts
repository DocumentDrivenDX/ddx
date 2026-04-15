import type { PageLoad } from './$types'
import { createClient } from '$lib/gql/client'
import { gql } from 'graphql-request'

const BEADS_QUERY = gql`
	query BeadsByProject($projectID: String!, $first: Int, $after: String) {
		beadsByProject(projectID: $projectID, first: $first, after: $after) {
			edges {
				node {
					id
					title
					status
					priority
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

interface BeadNode {
	id: string
	title: string
	status: string
	priority: number
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

interface BeadsResult {
	beadsByProject: BeadConnection
}

export const load: PageLoad = async ({ params, fetch }) => {
	const client = createClient(fetch as unknown as typeof globalThis.fetch)
	const data = await client.request<BeadsResult>(BEADS_QUERY, {
		projectID: params.projectId,
		first: 10
	})
	return {
		projectId: params.projectId,
		beads: data.beadsByProject
	}
}
