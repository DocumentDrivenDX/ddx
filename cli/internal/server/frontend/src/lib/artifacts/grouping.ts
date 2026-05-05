// Shared artifact grouping helpers.
// Keep prefix derivation in one place so list and detail views agree.

// prefixOf("docs/helix/01-frame/prd.md") => "docs"
// prefixOf("/library/personas/x.md") => "library"
const PREFIX_RE = /^\/?([^/]+)/;

export function prefixOf(path: string): string {
	const m = path.match(PREFIX_RE);
	return m ? m[1] : '';
}
