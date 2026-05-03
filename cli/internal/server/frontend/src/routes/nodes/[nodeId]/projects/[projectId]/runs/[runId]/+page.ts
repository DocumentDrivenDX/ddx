import type { PageLoad } from './$types';
import { createClient } from '$lib/gql/client';
import { gql } from 'graphql-request';

const RUN_HEADER_QUERY = gql`
	query RunHeader($id: ID!) {
		run(id: $id) {
			id
			layer
			status
			parentRunId
			artifactId
		}
	}
`;

export interface RunHeader {
	id: string;
	layer: string;
	status: string;
	parentRunId: string | null;
	artifactId: string | null;
}

const PARENT_RUN_QUERY = gql`
	query ParentRunParent($id: ID!) {
		run(id: $id) {
			parentRunId
		}
	}
`;

const PRODUCED_ARTIFACT_QUERY = gql`
	query ProducedArtifact($projectID: ID!, $id: ID!) {
		artifact(projectID: $projectID, id: $id) {
			id
			title
		}
	}
`;

export interface ProducedArtifact {
	id: string;
	title: string;
}

export const load: PageLoad = async ({ params, fetch }) => {
	const client = createClient(fetch as unknown as typeof globalThis.fetch);
	const data = await client.request<{ run: RunHeader | null }>(RUN_HEADER_QUERY, {
		id: params.runId
	});

	let grandparentRunId: string | null = null;
	if (data.run?.layer === 'run' && data.run.parentRunId) {
		const parentData = await client.request<{ run: { parentRunId: string | null } | null }>(
			PARENT_RUN_QUERY,
			{ id: data.run.parentRunId }
		);
		grandparentRunId = parentData.run?.parentRunId ?? null;
	}

	let producedArtifact: ProducedArtifact | null = null;
	if (data.run?.artifactId) {
		try {
			const artData = await client.request<{ artifact: ProducedArtifact | null }>(
				PRODUCED_ARTIFACT_QUERY,
				{ projectID: params.projectId, id: data.run.artifactId }
			);
			producedArtifact = artData.artifact;
		} catch {
			producedArtifact = null;
		}
	}

	return {
		nodeId: params.nodeId,
		projectId: params.projectId,
		runId: params.runId,
		run: data.run,
		grandparentRunId,
		producedArtifact
	};
};
