import type { PageLoad } from './$types';
import { loadPersonas } from '../data';

export const load: PageLoad = async ({ params, fetch }) => {
	return loadPersonas(
		fetch as unknown as typeof globalThis.fetch,
		params.projectId,
		decodeURIComponent(params.name)
	);
};
