import type { LayoutLoad } from './$types'
import { createClient } from '$lib/gql/client'
import { gql } from 'graphql-request'

const BEADS_QUERY = gql`
	query BeadsByProject($projectID: String!, $first: Int, $after: String, $status: String, $label: String, $search: String) {
		beadsByProject(projectID: $projectID, first: $first, after: $after, status: $status, label: $label, search: $search) {
			edges {
				node {
					id
					title
					status
					priority
					owner
					updatedAt
					labels
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
	owner: string | null
	updatedAt: string
	labels: string[] | null
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

export const load: LayoutLoad = async ({ params, url, fetch }) => {
	const status = url.searchParams.get('status') ?? undefined
	const label = url.searchParams.get('label') ?? undefined
	const search = url.searchParams.get('q') ?? undefined

	const client = createClient(fetch as unknown as typeof globalThis.fetch)
	const data = await client.request<BeadsResult>(BEADS_QUERY, {
		projectID: params.projectId,
		first: 10,
		status,
		label,
		search
	})
	return {
		projectId: params.projectId,
		beads: data.beadsByProject,
		activeStatus: status ?? null,
		activeLabel: label ?? null,
		activeSearch: search ?? null
	}
}
