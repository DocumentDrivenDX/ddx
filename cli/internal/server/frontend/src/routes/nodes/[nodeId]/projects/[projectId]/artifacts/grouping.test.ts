import { describe, it, expect } from 'vitest'
import {
	folderOf,
	prefixOf,
	workflowStageOf,
	axisAvailable,
	groupItems,
	WORKFLOW_STAGE_LABEL
} from './grouping'

describe('folderOf', () => {
	it('returns parent dir for nested paths', () => {
		expect(folderOf('docs/helix/01-frame/prd.md')).toBe('docs/helix/01-frame')
	})
	it('returns "/" for top-level files', () => {
		expect(folderOf('README.md')).toBe('/')
	})
	it('handles leading slash', () => {
		expect(folderOf('/a/b/c.md')).toBe('/a/b')
	})
})

describe('prefixOf', () => {
	it('returns the first path segment', () => {
		expect(prefixOf('docs/helix/x.md')).toBe('docs')
	})
	it('strips leading slash', () => {
		expect(prefixOf('/library/personas/x.md')).toBe('library')
	})
	it('returns the whole name when there is no slash', () => {
		expect(prefixOf('README.md')).toBe('README.md')
	})
})

describe('workflowStageOf', () => {
	it('extracts a HELIX-style numbered stage segment', () => {
		expect(workflowStageOf('docs/helix/01-frame/prd.md')).toBe('frame')
		expect(workflowStageOf('docs/helix/04-build/spec.md')).toBe('build')
	})
	it('returns null when no stage segment is present', () => {
		expect(workflowStageOf('library/personas/x.md')).toBeNull()
		expect(workflowStageOf('README.md')).toBeNull()
	})
})

describe('axisAvailable', () => {
	it('is true when at least one path has a stage', () => {
		expect(axisAvailable(['README.md', 'docs/helix/02-design/x.md'])).toBe(true)
	})
	it('is false when no path has a stage', () => {
		expect(axisAvailable(['README.md', 'library/x.md'])).toBe(false)
	})
})

describe('groupItems', () => {
	const items = [
		{ path: 'docs/a.md', mediaType: 'text/markdown' },
		{ path: 'docs/b.md', mediaType: 'text/markdown' },
		{ path: 'src/x.svg', mediaType: 'image/svg+xml' }
	]
	it('groups by folder', () => {
		const groups = groupItems(items, 'folder')
		expect(groups.map((g) => g.key)).toEqual(['docs', 'src'])
		expect(groups[0].items).toHaveLength(2)
	})
	it('groups by mediaType', () => {
		const groups = groupItems(items, 'mediaType')
		expect(groups.map((g) => g.key)).toEqual(['image/svg+xml', 'text/markdown'])
	})
	it('groups by prefix', () => {
		const groups = groupItems(items, 'prefix')
		expect(groups.map((g) => g.key)).toEqual(['docs', 'src'])
	})
})

describe('label constants', () => {
	it('uses generic Workflow stage label', () => {
		expect(WORKFLOW_STAGE_LABEL).toBe('Workflow stage')
	})
})
