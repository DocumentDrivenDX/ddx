import { QueryStore } from '../runtime/stores/query'
import artifact from '$houdini/artifacts/NodeInfo'
import { initClient } from '$houdini/plugins/houdini-svelte/runtime/client'

export class NodeInfoStore extends QueryStore {
	constructor() {
		super({
			artifact,
			storeName: "NodeInfoStore",
			variables: false,
		})
	}
}

export async function load_NodeInfo(params) {
  await initClient()

	const store = new NodeInfoStore()

	await store.fetch(params)

	return {
		NodeInfo: store,
	}
}
