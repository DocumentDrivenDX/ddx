import type { PageLoad } from './$types';
import { createClient } from '$lib/gql/client';
import { gql } from 'graphql-request';
import { error, redirect } from '@sveltejs/kit';

const DOCUMENT_BY_PATH = gql`
	query DocumentByPath($path: String!) {
		documentByPath(path: $path) {
			id
			path
			title
		}
	}
`;

interface DocumentByPathResult {
	documentByPath: {
		id: string;
		path: string;
		title: string;
	} | null;
}

export const load: PageLoad = async ({ params, fetch }) => {
	const client = createClient(fetch as unknown as typeof globalThis.fetch);
	const data = await client.request<DocumentByPathResult>(DOCUMENT_BY_PATH, { path: params.path });
	const doc = data.documentByPath;

	if (!doc) {
		throw error(404, `document ${params.path} not found`);
	}

	throw redirect(
		308,
		`/nodes/${params.nodeId}/projects/${params.projectId}/artifacts/${encodeURIComponent(`doc:${doc.id}`)}`
	);
};
