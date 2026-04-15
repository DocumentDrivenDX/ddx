import type { LayoutLoad } from './$types'
import { load_NodeInfo } from '$houdini'

export const load: LayoutLoad = async (event) => {
	return await load_NodeInfo({ event })
}
