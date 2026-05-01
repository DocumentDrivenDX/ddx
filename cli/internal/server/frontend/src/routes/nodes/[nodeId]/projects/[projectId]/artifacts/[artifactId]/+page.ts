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
			generatedBy {
				runId
				promptSummary
				sourceHashMatch
			}
		}
	}
`

const RUN_EXISTS_QUERY = gql`
	query RunExists($id: ID!) {
		run(id: $id) {
			id
		}
	}
`

export interface ArtifactGeneratedBy {
	runId: string
	promptSummary: string
	sourceHashMatch: boolean
}

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
	generatedBy: ArtifactGeneratedBy | null
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

	// Probe whether the producing run exists, so the UI can disable the link
	// (with tooltip) when the run id is unknown.
	let runExists = false
	const gb = data.artifact?.generatedBy
	if (gb?.runId) {
		try {
			const probe = await client.request<{ run: { id: string } | null }>(RUN_EXISTS_QUERY, {
				id: gb.runId
			})
			runExists = probe.run != null
		} catch {
			runExists = false
		}
	}

	return {
		nodeId: params.nodeId,
		projectId: params.projectId,
		artifactId: params.artifactId,
		artifact: data.artifact,
		contentUrl,
		back,
		runExists
	}
}
