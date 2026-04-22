import type { PageLoad } from './$types';
import { createClient } from '$lib/gql/client';
import { PLUGINS_LIST_QUERY } from '$lib/gql/feat008';

export interface PluginInfo {
	name: string;
	version: string;
	installedVersion: string | null;
	type: string;
	description: string;
	keywords: string[];
	status: string;
	registrySource: string;
	diskBytes: number;
	manifest?: unknown;
	skills: string[];
	prompts: string[];
	templates: string[];
}

interface PluginsListResult {
	pluginsList: PluginInfo[];
}

export const load: PageLoad = async ({ fetch }) => {
	const client = createClient(fetch as unknown as typeof globalThis.fetch);
	const data = await client.request<PluginsListResult>(PLUGINS_LIST_QUERY);
	return {
		plugins: data.pluginsList
	};
};
