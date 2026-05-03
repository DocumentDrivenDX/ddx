import type { PageLoad } from './$types';
import { redirect } from '@sveltejs/kit';

// Sessions has been merged into the layer-aware Runs row expansion.
// Client-side navigations land here and are 302-redirected to /runs?layer=run.
// Direct/bookmarked URLs are also redirected (with a Sunset header) by the
// Go server (see cli/internal/server/server.go).
export const load: PageLoad = ({ params, url }) => {
	const search = new URLSearchParams(url.search);
	search.set('layer', 'run');
	const target = `/nodes/${params.nodeId}/projects/${params.projectId}/runs?${search.toString()}`;
	throw redirect(302, target);
};
