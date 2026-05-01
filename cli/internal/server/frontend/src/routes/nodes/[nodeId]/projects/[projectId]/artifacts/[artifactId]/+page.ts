import type { PageLoad } from './$types'
import { createClient } from '$lib/gql/client'
import { gql } from 'graphql-request'

const ARTIFACT_DETAIL_QUERY = gql`
	query ArtifactDetail($projectID: ID!, $id: ID!) {
		artifact(projectID: $projectID, id: $id) {
			id
			path
			title
			mediaType
			staleness
			description
			updatedAt
			ddxFrontmatter
			content
		}
	}
`

export interface ArtifactDetail {
	id: string
	path: string
	title: string
	mediaType: string
	staleness: string
	description: string | null
	updatedAt: string | null
	ddxFrontmatter: string | null
	content: string | null
}

interface ArtifactDetailResult {
	artifact: ArtifactDetail | null
}

export const load: PageLoad = async ({ params, url, fetch }) => {
	const client = createClient(fetch as unknown as typeof globalThis.fetch)
	const data = await client.request<ArtifactDetailResult>(ARTIFACT_DETAIL_QUERY, {
		projectID: params.projectId,
		id: decodeURIComponent(params.artifactId)
	})

	// Build content URL for binary types (images, PDFs, unknown).
	// The UI uses this URL for <img src>, <embed src>, and download links.
	const contentUrl = data.artifact
		? `/api/projects/${encodeURIComponent(params.projectId)}/artifact-content?path=${encodeURIComponent(data.artifact.path)}`
		: null

	// Preserve back-link state so the detail page can return to the filtered list.
	const back = url.searchParams.get('back') ?? null

	return {
		nodeId: params.nodeId,
		projectId: params.projectId,
		artifactId: params.artifactId,
		artifact: data.artifact,
		contentUrl,
		back
	}
}
