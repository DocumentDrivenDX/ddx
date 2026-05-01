package docgraph

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// HashDocument computes a deterministic hash of a document's content,
// excluding the review block from the frontmatter.
func HashDocument(frontmatter *yaml.Node, body string) (string, error) {
	frontmatterText := ""
	if frontmatter != nil {
		clone := cloneNode(frontmatter)
		removeReviewFromBothNamespaces(clone)
		sortMappingNodes(clone)
		encoded, err := encodeYAML(clone)
		if err != nil {
			return "", fmt.Errorf("encode frontmatter: %w", err)
		}
		frontmatterText = normalizeNewlines(encoded)
	}

	normalizedBody := normalizeNewlines(body)
	content := normalizedBody
	if frontmatterText != "" {
		content = frontmatterText + "\n\n" + normalizedBody
	}

	sum := sha256.Sum256([]byte(content))
	return hex.EncodeToString(sum[:]), nil
}

func normalizeNewlines(value string) string {
	value = strings.ReplaceAll(value, "\r\n", "\n")
	value = strings.ReplaceAll(value, "\r", "\n")
	return value
}

func encodeYAML(node *yaml.Node) (string, error) {
	var buf strings.Builder
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(node); err != nil {
		return "", err
	}
	if err := enc.Close(); err != nil {
		return "", err
	}
	return strings.TrimRight(buf.String(), "\n"), nil
}

func cloneNode(node *yaml.Node) *yaml.Node {
	if node == nil {
		return nil
	}
	clone := *node
	if len(node.Content) > 0 {
		clone.Content = make([]*yaml.Node, len(node.Content))
		for i, child := range node.Content {
			clone.Content[i] = cloneNode(child)
		}
	}
	return &clone
}

// removeReviewFromBothNamespaces strips review from both ddx: and dun: blocks.
func removeReviewFromBothNamespaces(root *yaml.Node) {
	if root == nil || root.Kind != yaml.MappingNode {
		return
	}
	for _, ns := range []string{"ddx", "dun"} {
		nsNode := findMappingValue(root, ns)
		if nsNode != nil && nsNode.Kind == yaml.MappingNode {
			removeMappingKey(nsNode, "review")
		}
	}
}

func sortMappingNodes(node *yaml.Node) {
	if node == nil {
		return
	}
	if node.Kind == yaml.MappingNode {
		type pair struct {
			key   *yaml.Node
			value *yaml.Node
		}
		pairs := make([]pair, 0, len(node.Content)/2)
		for i := 0; i < len(node.Content); i += 2 {
			pairs = append(pairs, pair{key: node.Content[i], value: node.Content[i+1]})
		}
		sort.SliceStable(pairs, func(i, j int) bool {
			return pairs[i].key.Value < pairs[j].key.Value
		})
		node.Content = node.Content[:0]
		for _, p := range pairs {
			node.Content = append(node.Content, p.key, p.value)
		}
	}
	for _, child := range node.Content {
		sortMappingNodes(child)
	}
}
