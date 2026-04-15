import type { PageLoad } from './$types'
import { createClient } from '$lib/gql/client'
import { gql } from 'graphql-request'

const COMMITS_QUERY = gql`
	query Commits($projectID: ID!, $first: Int, $after: String) {
		commits(projectID: $projectID, first: $first, after: $after) {
			edges {
				node {
					sha
					shortSha
					author
					date
					subject
					body
					beadRefs
				}
				cursor
			}
			pageInfo {
				hasNextPage
				hasPreviousPage
				startCursor
				endCursor
			}
			totalCount
		}
	}
`

export interface CommitNode {
	sha: string
	shortSha: string
	author: string
	date: string
	subject: string
	body: string | null
	beadRefs: string[] | null
}

interface CommitEdge {
	node: CommitNode
	cursor: string
}

interface PageInfo {
	hasNextPage: boolean
	hasPreviousPage: boolean
	startCursor: string | null
	endCursor: string | null
}

interface CommitConnection {
	edges: CommitEdge[]
	pageInfo: PageInfo
	totalCount: number
}

interface CommitsResult {
	commits: CommitConnection
}

const PAGE_SIZE = 20

export const load: PageLoad = async ({ params, url, fetch }) => {
	const after = url.searchParams.get('after') ?? undefined

	const client = createClient(fetch as unknown as typeof globalThis.fetch)
	const data = await client.request<CommitsResult>(COMMITS_QUERY, {
		projectID: params.projectId,
		first: PAGE_SIZE,
		after
	})
	return {
		projectId: params.projectId,
		commits: data.commits,
		after: after ?? null
	}
}
