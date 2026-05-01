import type { PageLoad } from './$types'
import { createClient } from '$lib/gql/client'
import { gql } from 'graphql-request'

const ARTIFACT_NODE_QUERY = gql`
	query ArtifactNode($id: ID!) {
		node(id: $id) {
			... on Artifact {
				id
				path
				title
				mediaType
				staleness
				description
				updatedAt
			}
		}
	}
`

interface ArtifactDetail {
	id: string
	path: string
	title: string
	mediaType: string
	staleness: string
	description: string | null
	updatedAt: string | null
}

interface ArtifactNodeResult {
	node: ArtifactDetail | null
}

export const load: PageLoad = async ({ params, fetch }) => {
	const client = createClient(fetch as unknown as typeof globalThis.fetch)
	const data = await client.request<ArtifactNodeResult>(ARTIFACT_NODE_QUERY, {
		id: params.artifactId
	})
	return {
		nodeId: params.nodeId,
		projectId: params.projectId,
		artifactId: params.artifactId,
		artifact: data.node
	}
}
