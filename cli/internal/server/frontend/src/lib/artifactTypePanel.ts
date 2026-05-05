export interface ArtifactTypeDefinitionFile {
	path: string;
	content: string;
	isTruncated: boolean;
	sizeBytes: number;
}

export interface ArtifactTypeDefinitionExample {
	path: string;
	description?: string | null;
	content: string;
	isTruncated: boolean;
	sizeBytes: number;
}

export interface ArtifactTypeDefinition {
	plugin: string;
	typeId: string;
	name: string;
	description: string;
	prefix: string;
	pattern: string;
	phase: string;
	sourceMetaPath: string;
	template: ArtifactTypeDefinitionFile;
	prompt: ArtifactTypeDefinitionFile;
	examples: ArtifactTypeDefinitionExample[];
}

export type ArtifactTypeTab = 'referencePrompt' | 'template' | 'examples';

export const ARTIFACT_TYPE_TABS: { value: ArtifactTypeTab; label: string }[] = [
	{ value: 'referencePrompt', label: 'Reference Prompt' },
	{ value: 'template', label: 'Template' },
	{ value: 'examples', label: 'Examples' }
];

export function artifactTypeKey(def: ArtifactTypeDefinition): string {
	return `${def.plugin}::${def.typeId}::${def.sourceMetaPath}`;
}

export function artifactTypeLabel(def: ArtifactTypeDefinition): string {
	return def.name || `${def.plugin}/${def.typeId}`;
}

export function hasArtifactTypeCollision(definitions: ArtifactTypeDefinition[]): boolean {
	return definitions.length > 1;
}

export function selectedArtifactTypeDefinition(
	definitions: ArtifactTypeDefinition[],
	typeDefParam: string | null
): ArtifactTypeDefinition | null {
	if (definitions.length === 0) return null;
	if (!typeDefParam) return definitions[0];
	return definitions.find((def) => artifactTypeKey(def) === typeDefParam) ?? definitions[0];
}

export function updateTypeDefUrl(url: URL, typeDefParam: string | null): string {
	const next = new URL(url);
	if (typeDefParam) next.searchParams.set('typeDef', typeDefParam);
	else next.searchParams.delete('typeDef');
	return next.pathname + next.search;
}
