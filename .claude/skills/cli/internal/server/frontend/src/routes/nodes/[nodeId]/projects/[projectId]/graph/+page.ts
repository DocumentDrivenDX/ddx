import type { PageLoad } from './$types'
import { createClient } from '$lib/gql/client'
import { gql } from 'graphql-request'

const DOC_GRAPH_QUERY = gql`
	query DocGraph {
		docGraph {
			rootDir
			documents {
				id
				path
				title
				dependsOn
				dependents
			}
			warnings
			issues {
				kind
				path
				id
				message
				relatedPath
			}
		}
	}
`

interface GraphDocument {
	id: string
	path: string
	title: string
	dependsOn: string[]
	dependents: string[]
}

export interface GraphIssue {
	kind: string
	path: string | null
	id: string | null
	message: string
	relatedPath: string | null
}

interface DocGraph {
	rootDir: string
	documents: GraphDocument[]
	warnings: string[]
	issues: GraphIssue[]
}

interface DocGraphResult {
	docGraph: DocGraph
}

export const load: PageLoad = async ({ fetch }) => {
	const client = createClient(fetch as unknown as typeof globalThis.fetch)
	const data = await client.request<DocGraphResult>(DOC_GRAPH_QUERY)
	const graph = data.docGraph
	return {
		graph: {
			...graph,
			issues: graph.issues ?? []
		}
	}
}
