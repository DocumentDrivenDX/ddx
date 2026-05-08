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

interface QueryResult {
	beads: BeadConnection
	projects: {
		edges: Array<{ node: ProjectNode }>
	}
}

export const load: PageLoad = async ({ url, fetch }) => {
	const status = url.searchParams.get('status') ?? undefined
	const label = url.searchParams.get('label') ?? undefined
	const projectID = url.searchParams.get('project') ?? undefined

	const client = createClient(fetch as unknown as typeof globalThis.fetch)
	const data = await client.request<QueryResult>(BEADS_QUERY, {
		first: 20,
		status,
		label,
		projectID
	})

	const projectNames: Record<string, string> = {}
	for (const { node } of data.projects.edges) {
		projectNames[node.id] = node.name
	}

	return {
		beads: data.beads,
		projects: data.projects.edges.map((e) => e.node),
		projectNames,
		activeStatus: status ?? null,
		activeLabel: label ?? null,
		activeProject: projectID ?? null
	}
}
