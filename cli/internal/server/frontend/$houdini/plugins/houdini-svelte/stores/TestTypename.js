import { QueryStore } from '../runtime/stores/query'
import artifact from '$houdini/artifacts/TestTypename'
import { initClient } from '$houdini/plugins/houdini-svelte/runtime/client'

export class TestTypenameStore extends QueryStore {
	constructor() {
		super({
			artifact,
			storeName: "TestTypenameStore",
			variables: false,
		})
	}
}

export async function load_TestTypename(params) {
  await initClient()

	const store = new TestTypenameStore()

	await store.fetch(params)

	return {
		TestTypename: store,
	}
}
