import type { PageLoad } from './$types';
import { redirect } from '@sveltejs/kit';

// Executions has been merged into the layer-aware Runs row expansion.
// Client-side navigations are 302-redirected to /runs?layer=try with the
// existing query parameters preserved where possible.
export const load: PageLoad = ({ params, url }) => {
	const search = new URLSearchParams();
	const harness = url.searchParams.get('harness');
	if (harness) search.set('harness', harness);
	search.set('layer', 'try');
	const target = `/nodes/${params.nodeId}/projects/${params.projectId}/runs?${search.toString()}`;
	throw redirect(302, target);
};
