import type { PageLoad } from './$types'
import { createClient } from '$lib/gql/client'
import { gql } from 'graphql-request'

const DOCS_QUERY = gql`
	query Documents($first: Int, $after: String) {
		documents(first: $first, after: $after) {
			edges {
				node {
					id
					path
					title
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

interface DocumentNode {
	id: string
	path: string
	title: string
}

interface DocumentEdge {
	node: DocumentNode
	cursor: string
}

interface PageInfo {
	hasNextPage: boolean
	endCursor: string | null
}

interface DocumentConnection {
	edges: DocumentEdge[]
	pageInfo: PageInfo
	totalCount: number
}

interface DocsResult {
	documents: DocumentConnection
}

export const load: PageLoad = async ({ fetch }) => {
	const client = createClient(fetch as unknown as typeof globalThis.fetch)
	const data = await client.request<DocsResult>(DOCS_QUERY, { first: 200 })
	return {
		docs: data.documents
	}
}
