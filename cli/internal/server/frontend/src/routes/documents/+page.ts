import type { PageLoad } from './$types';
import { redirect } from '@sveltejs/kit';
import { resolveDefaultProjectRoute } from '$lib/routing/shellRoutes';

export const load: PageLoad = async ({ fetch }) => {
	const target = await resolveDefaultProjectRoute(
		'artifacts',
		fetch as unknown as typeof globalThis.fetch
	);
	throw redirect(307, `${target}?mediaType=text%2Fmarkdown`);
};
