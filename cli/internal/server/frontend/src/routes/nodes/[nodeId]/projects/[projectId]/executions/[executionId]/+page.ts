import type { PageLoad } from './$types';
import { redirect } from '@sveltejs/kit';

// Execution detail has been merged into the layer-aware Runs row expansion.
// We map the execution id to its synthetic try-layer run id ("exec-<bundleID>")
// and 302-redirect into the Runs detail page with bundle expansion.
export const load: PageLoad = ({ params }) => {
	const runId = params.executionId.startsWith('exec-')
		? params.executionId
		: `exec-${params.executionId}`;
	const target = `/nodes/${params.nodeId}/projects/${params.projectId}/runs/${runId}`;
	throw redirect(302, target);
};
