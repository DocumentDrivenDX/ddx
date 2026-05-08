import type { PageLoad } from './$types';
import { redirect } from '@sveltejs/kit';
import { resolveDefaultProjectRoute } from '$lib/routing/shellRoutes';

export const load: PageLoad = async ({ fetch }) => {
	throw redirect(
		307,
		await resolveDefaultProjectRoute('personas', fetch as unknown as typeof globalThis.fetch)
	);
};
