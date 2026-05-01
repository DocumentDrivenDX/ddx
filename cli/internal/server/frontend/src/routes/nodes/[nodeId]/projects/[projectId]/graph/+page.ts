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
			pathToId
			warnings
			issues {
				issueId
				kind
				path
				id
				message
				relatedPath
			}
		}
	}
`

const DOC_STALE_QUERY = gql`
	query DocStale {
		docStale {
			id
		}
	}
`

export interface GraphDocument {
	id: string
	path: string
	title: string
	dependsOn: string[]
	dependents: string[]
	staleness: string
	mediaType: string
}

export interface GraphIssue {
	issueId: string
	kind: string
	path: string | null
	id: string | null
	message: string
	relatedPath: string | null
}

interface RawGraphDocument {
	id: string
	path: string
	title: string
	dependsOn: string[]
	dependents: string[]
}

interface DocGraph {
	rootDir: string
	documents: RawGraphDocument[]
	pathToId: string
	warnings: string[]
	issues: GraphIssue[]
}

interface DocGraphResult {
	docGraph: DocGraph
}

interface StaleResult {
	docStale: Array<{ id: string }>
}

function mediaTypeFromPath(path: string): string {
	const ext = path.split('.').pop()?.toLowerCase() ?? ''
	if (ext === 'md') return 'text/markdown'
	if (['png', 'jpg', 'jpeg', 'gif', 'webp'].includes(ext)) return 'image/*'
	if (ext === 'svg') return 'image/svg+xml'
	return 'unknown'
}

export const load: PageLoad = async ({ fetch, url }) => {
	const client = createClient(fetch as unknown as typeof globalThis.fetch)

	const [graphData, staleData] = await Promise.all([
		client.request<DocGraphResult>(DOC_GRAPH_QUERY),
		client
			.request<StaleResult>(DOC_STALE_QUERY)
			.catch(() => ({ docStale: [] as Array<{ id: string }> }))
	])

	const graph = graphData.docGraph
	const staleIds = new Set(staleData.docStale.map((s) => s.id))

	let pathToDocId: Record<string, string> = {}
	try {
		pathToDocId = JSON.parse(graph.pathToId ?? '{}')
	} catch {
		pathToDocId = {}
	}

	const documents: GraphDocument[] = graph.documents.map((doc) => ({
		...doc,
		staleness: staleIds.has(doc.id) ? 'stale' : 'fresh',
		mediaType: mediaTypeFromPath(doc.path)
	}))

	// Parse initial viewport from URL params
	const zoomParam = parseFloat(url.searchParams.get('zoom') ?? '')
	const panParam = url.searchParams.get('pan')
	const panParts = panParam ? panParam.split(',').map(parseFloat) : null
	const initialTransform =
		!isNaN(zoomParam) && panParts && panParts.length === 2 && panParts.every((v) => !isNaN(v))
			? { x: panParts[0], y: panParts[1], k: zoomParam }
			: null

	const highlightNodeId = url.searchParams.get('highlight') ?? undefined

	return {
		graph: {
			...graph,
			documents,
			pathToDocId,
			issues: graph.issues ?? []
		},
		initialTransform,
		highlightNodeId
	}
}
