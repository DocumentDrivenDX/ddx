package docgraph

import (
	"bytes"
	"fmt"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// DocFrontmatter holds the parsed ddx: (or legacy dun:) frontmatter block.
type DocFrontmatter struct {
	ID         string      `yaml:"id"`
	DependsOn  []string    `yaml:"depends_on"`
	Prompt     string      `yaml:"prompt"`
	Inputs     []string    `yaml:"inputs"`
	Review     DocReview   `yaml:"review"`
	ParkingLot bool        `yaml:"parking_lot"`
	Exec       *DocExecDef `yaml:"exec"`
}

// DocExecDef describes an execution definition embedded in a graph document.
type DocExecDef struct {
	ArtifactIDs []string        `yaml:"artifact_ids"`
	Executor    DocExecExecutor `yaml:"executor"`
	Required    bool            `yaml:"required"`
	Active      bool            `yaml:"active"`
}

// DocExecExecutor describes how a graph-authored execution definition is invoked.
type DocExecExecutor struct {
	Kind      string            `yaml:"kind"`
	Command   []string          `yaml:"command"`
	Cwd       string            `yaml:"cwd"`
	Env       map[string]string `yaml:"env"`
	TimeoutMS int               `yaml:"timeout_ms"`
}

// DocReview holds staleness tracking metadata.
type DocReview struct {
	SelfHash   string            `yaml:"self_hash"`
	Deps       map[string]string `yaml:"deps"`
	ReviewedAt string            `yaml:"reviewed_at"`
}

// Frontmatter is the parsed result from a markdown file.
type Frontmatter struct {
	Doc            DocFrontmatter
	Raw            *yaml.Node
	HasFrontmatter bool
	// Namespace records which prefix was found: "ddx" or "dun".
	Namespace string
}

// ParseFrontmatter parses YAML frontmatter from markdown content.
// It reads both ddx: and dun: namespaces (preferring ddx: if both present).
// Returns the parsed frontmatter, the body text after the closing ---, and any error.
func ParseFrontmatter(content []byte) (Frontmatter, string, error) {
	frontmatterText, body, ok := splitFrontmatter(content)
	if !ok {
		return Frontmatter{}, string(content), nil
	}

	trimmed := strings.TrimSpace(frontmatterText)

	// Try ddx: first, then dun: for backward compatibility
	var doc DocFrontmatter
	namespace := ""

	var ddxWrapper struct {
		DDx DocFrontmatter `yaml:"ddx"`
	}
	if err := yaml.Unmarshal([]byte(trimmed), &ddxWrapper); err != nil {
		return Frontmatter{}, "", fmt.Errorf("parse frontmatter: %w", err)
	}
	if ddxWrapper.DDx.ID != "" {
		doc = ddxWrapper.DDx
		namespace = "ddx"
	} else {
		var dunWrapper struct {
			Dun DocFrontmatter `yaml:"dun"`
		}
		if err := yaml.Unmarshal([]byte(trimmed), &dunWrapper); err != nil {
			return Frontmatter{}, "", fmt.Errorf("parse frontmatter: %w", err)
		}
		if dunWrapper.Dun.ID != "" {
			doc = dunWrapper.Dun
			namespace = "dun"
		}
	}

	var node yaml.Node
	if err := yaml.Unmarshal([]byte(trimmed), &node); err != nil {
		return Frontmatter{}, "", fmt.Errorf("parse frontmatter node: %w", err)
	}

	var root *yaml.Node
	if len(node.Content) > 0 {
		root = node.Content[0]
	}

	return Frontmatter{
		Doc:            doc,
		Raw:            root,
		HasFrontmatter: true,
		Namespace:      namespace,
	}, body, nil
}

// MigrateLegacyDunFrontmatter converts top-level `dun:` metadata to `ddx:`.
//
// Migration is lossless for fields:
//   - If both `ddx` and `dun` are present, keys from `dun` are merged into `ddx`
//     when missing.
//   - If only `dun` is present, it is renamed to `ddx`.
//   - If `ddx` is already present and no other legacy-compatible namespace exists,
//     no changes are made.
func MigrateLegacyDunFrontmatter(root *yaml.Node) bool {
	if root == nil || root.Kind != yaml.MappingNode {
		return false
	}

	ddxNode := findMappingValue(root, "ddx")
	dunNode := findMappingValue(root, "dun")
	if dunNode == nil {
		return false
	}

	if ddxNode == nil {
		dunKey := findMappingKey(root, "dun")
		if dunKey != nil {
			dunKey.Value = "ddx"
		}
		return true
	}

	// Merge legacy keys into ddx only when missing.
	mergeIntoDDX(ddxNode, dunNode)

	removeMappingKey(root, "dun")
	return true
}

func splitFrontmatter(content []byte) (string, string, bool) {
	if !bytes.HasPrefix(content, []byte("---")) {
		return "", string(content), false
	}

	firstLineEnd := bytes.IndexByte(content, '\n')
	if firstLineEnd == -1 {
		return "", string(content), false
	}

	firstLine := bytes.TrimRight(content[:firstLineEnd], "\r")
	if !bytes.Equal(firstLine, []byte("---")) {
		return "", string(content), false
	}

	rest := content[firstLineEnd+1:]
	idx := 0
	for idx <= len(rest) {
		lineEnd := bytes.IndexByte(rest[idx:], '\n')
		if lineEnd == -1 {
			lineEnd = len(rest) - idx
		}
		line := rest[idx : idx+lineEnd]
		lineTrimmed := bytes.TrimRight(line, "\r")
		if bytes.Equal(lineTrimmed, []byte("---")) {
			frontmatter := rest[:idx]
			bodyStart := idx + lineEnd
			if idx+lineEnd < len(rest) && rest[idx+lineEnd] == '\n' {
				bodyStart = idx + lineEnd + 1
			}
			return string(frontmatter), string(rest[bodyStart:]), true
		}
		if idx+lineEnd >= len(rest) {
			break
		}
		idx += lineEnd + 1
	}

	return "", string(content), false
}

// SetReview updates the review block in the frontmatter YAML node.
// Always writes under the ddx: namespace.
func SetReview(root *yaml.Node, review DocReview) error {
	if root == nil {
		return fmt.Errorf("frontmatter missing")
	}
	if root.Kind != yaml.MappingNode {
		return fmt.Errorf("frontmatter root must be mapping")
	}

	// Ensure ddx: namespace exists; if only dun: exists, create ddx:
	ddxNode := findMappingValue(root, "ddx")
	if ddxNode == nil {
		// Copy from dun: if it exists, then write as ddx:
		dunNode := findMappingValue(root, "dun")
		if dunNode != nil {
			ddxNode = ensureMappingNode(root, "ddx")
			// Copy content from dun to ddx
			ddxNode.Content = append(ddxNode.Content[:0], dunNode.Content...)
			// Remove dun: namespace
			removeMappingKey(root, "dun")
		} else {
			ddxNode = ensureMappingNode(root, "ddx")
		}
	}

	reviewNode := ensureMappingNode(ddxNode, "review")
	setScalarNode(reviewNode, "self_hash", review.SelfHash)
	setMappingNode(reviewNode, "deps", review.Deps)
	if review.ReviewedAt != "" {
		setScalarNode(reviewNode, "reviewed_at", review.ReviewedAt)
	}
	return nil
}

// EncodeFrontmatter encodes a YAML node back to string.
func EncodeFrontmatter(root *yaml.Node) (string, error) {
	if root == nil {
		return "", fmt.Errorf("frontmatter missing")
	}
	var buf strings.Builder
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(root); err != nil {
		return "", err
	}
	if err := enc.Close(); err != nil {
		return "", err
	}
	return strings.TrimRight(buf.String(), "\n"), nil
}

func ensureMappingNode(parent *yaml.Node, key string) *yaml.Node {
	if parent.Kind != yaml.MappingNode {
		parent.Kind = yaml.MappingNode
	}
	for i := 0; i < len(parent.Content); i += 2 {
		k := parent.Content[i]
		if k.Value == key {
			v := parent.Content[i+1]
			if v.Kind != yaml.MappingNode {
				v.Kind = yaml.MappingNode
				v.Content = nil
			}
			return v
		}
	}
	keyNode := &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: key}
	valueNode := &yaml.Node{Kind: yaml.MappingNode}
	parent.Content = append(parent.Content, keyNode, valueNode)
	return valueNode
}

func setScalarNode(parent *yaml.Node, key string, value string) {
	for i := 0; i < len(parent.Content); i += 2 {
		k := parent.Content[i]
		if k.Value == key {
			parent.Content[i+1] = &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: value}
			return
		}
	}
	parent.Content = append(parent.Content,
		&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: key},
		&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: value},
	)
}

func setMappingNode(parent *yaml.Node, key string, values map[string]string) {
	mapping := &yaml.Node{Kind: yaml.MappingNode}
	if len(values) > 0 {
		keys := make([]string, 0, len(values))
		for k := range values {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			mapping.Content = append(mapping.Content,
				&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: k},
				&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: values[k]},
			)
		}
	}
	for i := 0; i < len(parent.Content); i += 2 {
		k := parent.Content[i]
		if k.Value == key {
			parent.Content[i+1] = mapping
			return
		}
	}
	parent.Content = append(parent.Content,
		&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: key},
		mapping,
	)
}

func findMappingValue(node *yaml.Node, key string) *yaml.Node {
	if node == nil || node.Kind != yaml.MappingNode {
		return nil
	}
	for i := 0; i < len(node.Content); i += 2 {
		k := node.Content[i]
		if k.Value == key {
			return node.Content[i+1]
		}
	}
	return nil
}

func findMappingKey(node *yaml.Node, key string) *yaml.Node {
	if node == nil || node.Kind != yaml.MappingNode {
		return nil
	}
	for i := 0; i < len(node.Content); i += 2 {
		k := node.Content[i]
		if k.Value == key {
			return k
		}
	}
	return nil
}

func mergeIntoDDX(ddxNode *yaml.Node, dunNode *yaml.Node) {
	if ddxNode == nil || dunNode == nil {
		return
	}
	if ddxNode.Kind != yaml.MappingNode {
		ddxNode.Kind = yaml.MappingNode
	}
	if dunNode.Kind != yaml.MappingNode {
		return
	}

	existing := map[string]struct{}{}
	for i := 0; i < len(ddxNode.Content); i += 2 {
		existing[ddxNode.Content[i].Value] = struct{}{}
	}

	merged := false
	for i := 0; i < len(dunNode.Content); i += 2 {
		keyNode := dunNode.Content[i]
		valNode := dunNode.Content[i+1]
		if keyNode == nil || valNode == nil {
			continue
		}
		if _, ok := existing[keyNode.Value]; ok {
			continue
		}
		clonedValue := cloneNode(valNode)
		ddxNode.Content = append(ddxNode.Content, &yaml.Node{
			Kind:  keyNode.Kind,
			Tag:   keyNode.Tag,
			Value: keyNode.Value,
		}, clonedValue)
		existing[keyNode.Value] = struct{}{}
		merged = true
	}
	if merged {
		sortMappingNodes(ddxNode)
	}
}

func removeMappingKey(node *yaml.Node, key string) {
	if node == nil || node.Kind != yaml.MappingNode {
		return
	}
	for i := 0; i < len(node.Content); i += 2 {
		k := node.Content[i]
		if k.Value == key {
			node.Content = append(node.Content[:i], node.Content[i+2:]...)
			return
		}
	}
}
