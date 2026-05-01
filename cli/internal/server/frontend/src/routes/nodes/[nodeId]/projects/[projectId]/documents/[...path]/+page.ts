import type { PageLoad } from './$types'
import { createClient } from '$lib/gql/client'
import { gql } from 'graphql-request'

const DOCUMENT_BY_PATH = gql`
	query DocumentByPath($path: String!) {
		documentByPath(path: $path) {
			id
			path
			title
			content
			dependsOn
			dependents
		}
	}
`

const DOC_GRAPH_QUERY = gql`
	query DocGraphForSearch {
		docGraph {
			documents {
				id
				path
				title
			}
		}
	}
`

interface DocumentByPathResult {
	documentByPath: {
		id: string
		path: string
		title: string
		content: string
		dependsOn: string[]
		dependents: string[]
	} | null
}

interface DocGraphForSearchResult {
	docGraph: {
		documents: { id: string; path: string; title: string }[]
	}
}

export const load: PageLoad = async ({ params, fetch }) => {
	const client = createClient(fetch as unknown as typeof globalThis.fetch)
	const [docResult, graphResult] = await Promise.allSettled([
		client.request<DocumentByPathResult>(DOCUMENT_BY_PATH, { path: params.path }),
		client.request<DocGraphForSearchResult>(DOC_GRAPH_QUERY)
	])

	const doc = docResult.status === 'fulfilled' ? docResult.value.documentByPath : null
	const allDocuments =
		graphResult.status === 'fulfilled' ? graphResult.value.docGraph.documents : []

	if (!doc) {
		return { path: params.path, content: null, dependsOn: [], dependents: [], title: '', allDocuments }
	}

	return {
		path: doc.path,
		title: doc.title,
		content: doc.content,
		dependsOn: doc.dependsOn,
		dependents: doc.dependents,
		allDocuments
	}
}
