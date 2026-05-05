import { describe, it, expect } from 'vitest';
import {
	artifactTypeKey,
	hasArtifactTypeCollision,
	selectedArtifactTypeDefinition,
	updateTypeDefUrl,
	type ArtifactTypeDefinition
} from './artifactTypePanel';

function def(overrides: Partial<ArtifactTypeDefinition>): ArtifactTypeDefinition {
	return {
		plugin: 'ddx',
		typeId: 'docs-sample',
		name: 'Docs sample',
		description: 'Example type',
		prefix: 'docs',
		pattern: 'docs/*.md',
		phase: 'frame',
		sourceMetaPath: 'plugins/ddx/types/docs-sample/meta.yml',
		template: {
			path: 'template.md',
			content: '# template',
			isTruncated: false,
			sizeBytes: 10
		},
		prompt: {
			path: 'prompt.md',
			content: '# prompt',
			isTruncated: false,
			sizeBytes: 8
		},
		examples: [],
		...overrides
	};
}

describe('artifactTypePanel helpers', () => {
	it('selects the single matching type definition by default', () => {
		const defs = [def({ typeId: 'docs-sample' })];
		expect(selectedArtifactTypeDefinition(defs, null)).toBe(defs[0]);
		expect(hasArtifactTypeCollision(defs)).toBe(false);
	});

	it('exposes collision state and stable keys for multiple matches', () => {
		const defs = [def({ plugin: 'alpha' }), def({ plugin: 'beta' })];
		expect(hasArtifactTypeCollision(defs)).toBe(true);
		expect(artifactTypeKey(defs[0])).toBe('alpha::docs-sample::plugins/ddx/types/docs-sample/meta.yml');
		expect(artifactTypeKey(defs[1])).toBe('beta::docs-sample::plugins/ddx/types/docs-sample/meta.yml');
	});

	it('round-trips typeDef query state without dropping unrelated params', () => {
		const url = new URL('https://example.test/nodes/n/projects/p/artifacts/a?back=%2Flist&view=full');
		expect(updateTypeDefUrl(url, 'alpha::docs-sample::plugins/ddx/types/docs-sample/meta.yml')).toBe(
			'/nodes/n/projects/p/artifacts/a?back=%2Flist&view=full&typeDef=alpha%3A%3Adocs-sample%3A%3Aplugins%2Fddx%2Ftypes%2Fdocs-sample%2Fmeta.yml'
		);
		expect(updateTypeDefUrl(url, null)).toBe('/nodes/n/projects/p/artifacts/a?back=%2Flist&view=full');
	});
});
