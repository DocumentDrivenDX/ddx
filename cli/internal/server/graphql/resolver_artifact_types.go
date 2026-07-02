package graphql

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/DocumentDrivenDX/ddx/internal/artifacttypes"
	"github.com/DocumentDrivenDX/ddx/internal/config"
	"github.com/DocumentDrivenDX/ddx/internal/evidence"
)

const artifactTypeFileCapBytes = 64 * 1024

var artifactTypeLoader = artifacttypes.NewLoader()

func artifactTypeDefinitionsForPath(workingDir, artifactPath string) ([]*ArtifactTypeDefinition, error) {
	prefix := prefixOf(artifactPath)
	if prefix == "" {
		return []*ArtifactTypeDefinition{}, nil
	}
	if workingDir == "" {
		return []*ArtifactTypeDefinition{}, nil
	}

	cfg, err := config.LoadWithWorkingDir(workingDir)
	if err != nil {
		return nil, fmt.Errorf("loading config: %w", err)
	}
	if cfg.Library == nil || strings.TrimSpace(cfg.Library.Path) == "" {
		return []*ArtifactTypeDefinition{}, nil
	}

	roots, err := artifactTypeRootsForLibraryPath(workingDir, cfg.Library.Path)
	if err != nil {
		return nil, err
	}

	seen := make(map[string]struct{})
	out := make([]*ArtifactTypeDefinition, 0)
	for _, root := range roots {
		index, err := artifactTypeLoader.Load(root)
		if err != nil {
			return nil, fmt.Errorf("loading artifact types from %s: %w", root, err)
		}
		if index == nil {
			return nil, fmt.Errorf("loading artifact types from %s: received nil index", root)
		}
		for _, typ := range index.Types {
			if typ.Prefix != prefix {
				continue
			}
			key := typ.Plugin + "\x00" + typ.TypeID + "\x00" + typ.SourceMetaPath
			if _, ok := seen[key]; ok {
				continue
			}
			defn, err := buildArtifactTypeDefinition(root, typ)
			if err != nil {
				return nil, err
			}
			seen[key] = struct{}{}
			out = append(out, defn)
		}
	}

	sort.Slice(out, func(i, j int) bool {
		if out[i].Plugin != out[j].Plugin {
			return out[i].Plugin < out[j].Plugin
		}
		if out[i].TypeID != out[j].TypeID {
			return out[i].TypeID < out[j].TypeID
		}
		return out[i].SourceMetaPath < out[j].SourceMetaPath
	})

	return out, nil
}

func attachArtifactTypeDefinitions(workingDir string, artifact *Artifact) error {
	defs, err := artifactTypeDefinitionsForPath(workingDir, artifact.Path)
	if err != nil {
		return err
	}
	artifact.TypeDefinitions = defs
	return nil
}

func attachArtifactTypeDefinitionsToArtifacts(workingDir string, artifacts []*Artifact) error {
	for _, artifact := range artifacts {
		if artifact == nil {
			continue
		}
		if err := attachArtifactTypeDefinitions(workingDir, artifact); err != nil {
			return err
		}
	}
	return nil
}

func artifactTypeRootsForLibraryPath(workingDir, libraryPath string) ([]string, error) {
	libPath := libraryPath
	if !filepath.IsAbs(libPath) {
		libPath = filepath.Join(workingDir, libPath)
	}

	roots := []string{libPath}
	parent := filepath.Dir(libPath)
	if filepath.Base(parent) == "plugins" {
		entries, err := os.ReadDir(parent)
		if err != nil {
			if os.IsNotExist(err) {
				return []string{libPath}, nil
			}
			return nil, fmt.Errorf("reading plugin root directory %s: %w", parent, err)
		}
		for _, entry := range entries {
			if entry.IsDir() {
				roots = append(roots, filepath.Join(parent, entry.Name()))
			}
		}
	}

	return dedupeArtifactTypeRoots(roots), nil
}

func dedupeArtifactTypeRoots(roots []string) []string {
	if len(roots) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(roots))
	out := make([]string, 0, len(roots))
	for _, root := range roots {
		if root == "" {
			continue
		}
		cleaned := filepath.Clean(root)
		if _, ok := seen[cleaned]; ok {
			continue
		}
		seen[cleaned] = struct{}{}
		out = append(out, cleaned)
	}
	return out
}

func buildArtifactTypeDefinition(root string, typ artifacttypes.Type) (*ArtifactTypeDefinition, error) {
	template, err := loadArtifactTypeDefinitionFile(root, typ.TemplatePath, true)
	if err != nil {
		return nil, fmt.Errorf("%s: template: %w", typ.SourceMetaPath, err)
	}
	prompt, err := loadArtifactTypeDefinitionFile(root, typ.PromptPath, true)
	if err != nil {
		return nil, fmt.Errorf("%s: prompt: %w", typ.SourceMetaPath, err)
	}

	examples := make([]*ArtifactTypeDefinitionExample, 0, len(typ.Examples))
	for _, ex := range typ.Examples {
		example, err := loadArtifactTypeDefinitionExample(root, ex)
		if err != nil {
			return nil, fmt.Errorf("%s: example %s: %w", typ.SourceMetaPath, ex.Path, err)
		}
		if example != nil {
			examples = append(examples, example)
		}
	}

	return &ArtifactTypeDefinition{
		Plugin:         typ.Plugin,
		TypeID:         typ.TypeID,
		Name:           typ.Name,
		Description:    typ.Description,
		Prefix:         typ.Prefix,
		Pattern:        typ.Pattern,
		Phase:          typ.Phase,
		SourceMetaPath: typ.SourceMetaPath,
		Template:       template,
		Prompt:         prompt,
		Examples:       examples,
	}, nil
}

func loadArtifactTypeDefinitionFile(root, relPath string, required bool) (*ArtifactTypeDefinitionFile, error) {
	relPath = strings.TrimSpace(relPath)
	if relPath == "" {
		if required {
			return nil, fmt.Errorf("missing file path")
		}
		return nil, nil
	}

	absPath := filepath.Join(root, filepath.FromSlash(relPath))
	content, truncated, originalBytes, err := evidence.ReadFileClamped(absPath, artifactTypeFileCapBytes)
	if err != nil {
		if required {
			return nil, err
		}
		return nil, nil
	}
	inline := string(content)
	return &ArtifactTypeDefinitionFile{
		Path:        filepath.ToSlash(relPath),
		Content:     inline,
		IsTruncated: truncated,
		SizeBytes:   int(originalBytes),
	}, nil
}

func loadArtifactTypeDefinitionExample(root string, ex artifacttypes.Example) (*ArtifactTypeDefinitionExample, error) {
	file, err := loadArtifactTypeDefinitionFile(root, ex.Path, false)
	if err != nil || file == nil {
		return nil, err
	}
	desc := ex.Description
	return &ArtifactTypeDefinitionExample{
		Path:        file.Path,
		Description: &desc,
		Content:     file.Content,
		IsTruncated: file.IsTruncated,
		SizeBytes:   file.SizeBytes,
	}, nil
}

// prefixOf returns the first path segment, matching the frontend grouping helper.
func prefixOf(path string) string {
	trimmed := strings.TrimSpace(filepath.ToSlash(path))
	trimmed = strings.TrimPrefix(trimmed, "/")
	if trimmed == "" {
		return ""
	}
	if idx := strings.IndexByte(trimmed, '/'); idx >= 0 {
		return trimmed[:idx]
	}
	return trimmed
}
