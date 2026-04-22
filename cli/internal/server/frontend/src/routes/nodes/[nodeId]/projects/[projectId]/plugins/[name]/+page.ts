import type { PageLoad } from './$types';
import { error } from '@sveltejs/kit';
import { createClient } from '$lib/gql/client';
import { PLUGIN_DETAIL_QUERY } from '$lib/gql/feat008';
import type { PluginInfo } from '../+page';

interface PluginDetailResult {
	pluginDetail: PluginInfo | null;
}

export const load: PageLoad = async ({ params, fetch }) => {
	const client = createClient(fetch as unknown as typeof globalThis.fetch);
	const data = await client.request<PluginDetailResult>(PLUGIN_DETAIL_QUERY, { name: params.name });
	if (!data.pluginDetail) {
		throw error(404, `plugin ${params.name} not found`);
	}
	return {
		plugin: data.pluginDetail
	};
};
