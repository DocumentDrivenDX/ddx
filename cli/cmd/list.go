package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"golang.org/x/text/cases"
	"golang.org/x/text/language"

	"github.com/DocumentDrivenDX/ddx/internal/config"
	"github.com/spf13/cobra"
)

// Resource represents a single asset for JSON output
type Resource struct {
	Name        string   `json:"name"`
	Type        string   `json:"type"`
	Description string   `json:"description"`
	Path        string   `json:"path"`
	IsDirectory bool     `json:"is_directory"`
	Size        int64    `json:"size,omitempty"`
	Tags        []string `json:"tags,omitempty"`
}

// ListResponse represents the complete JSON response
type ListResponse struct {
	Resources []Resource     `json:"resources"`
	Summary   map[string]int `json:"summary"`
	Filter    string         `json:"filter,omitempty"`
	Type      string         `json:"type,omitempty"`
}

// CommandFactory method - CLI interface layer
func (f *CommandFactory) runList(cmd *cobra.Command, args []string) error {
	// Get flag values
	filterValue, _ := cmd.Flags().GetString("filter")
	jsonOutput, _ := cmd.Flags().GetBool("json")
	treeOutput, _ := cmd.Flags().GetBool("tree")

	// Get resource type from args
	var resourceType string
	if len(args) > 0 {
		resourceType = args[0]
	}

	// Call pure business logic
	response, err := listResources(f.WorkingDir, resourceType, filterValue)
	if err != nil {
		return err
	}

	// Handle output formatting
	if jsonOutput {
		return outputListJSON(cmd, response)
	}
	if treeOutput {
		return outputListTree(cmd, response)
	}
	return outputListHuman(cmd, response, filterValue, resourceType)
}

// listResources is the pure business logic function
func listResources(workingDir, resourceType, filter string) (*ListResponse, error) {
	// Load config to get library path
	cfg, err := config.LoadWithWorkingDir(workingDir)
	if err != nil {
		return nil, fmt.Errorf("failed to load config: %w", err)
	}

	var libPath string
	if cfg.Library != nil {
		libPath = cfg.Library.Path
	}

	// Resolve library path relative to working directory if it's relative
	if !filepath.IsAbs(libPath) {
		libPath = filepath.Join(workingDir, libPath)
	}

	// Check if library exists
	if _, err := os.Stat(libPath); os.IsNotExist(err) {
		return &ListResponse{
			Resources: []Resource{},
			Summary:   map[string]int{},
		}, nil
	}

	// Define resource types to list
	resourceTypes := []string{"templates", "workflows", "mcp-servers", "prompts", "personas", "configs", "scripts", "tools", "environments"}

	// Filter by type if specified
	if resourceType != "" {
		resourceTypes = []string{resourceType}
	}

	// Simplified: no configuration loading needed for resource discovery

	// Collect all resources
	var allResources []Resource
	summary := make(map[string]int)

	for _, resType := range resourceTypes {
		// Simplified: just use manual discovery (no complex filtering)
		filteredPaths := discoverResourcesManually(libPath, resType)

		var categoryResources []Resource
		for _, itemPath := range filteredPaths {
			// Apply additional text filter if specified
			if filter != "" {
				relPath := strings.TrimPrefix(itemPath, filepath.Join(libPath, resType)+"/")
				if !strings.Contains(strings.ToLower(relPath), strings.ToLower(filter)) {
					continue
				}
			}

			info, err := os.Stat(itemPath)
			if err != nil {
				continue
			}

			description := getResourceInfo(itemPath, &dirEntryWrapper{info})

			var size int64
			if !info.IsDir() {
				size = info.Size()
			}

			// Get relative name for display
			relPath := strings.TrimPrefix(itemPath, filepath.Join(libPath, resType)+"/")
			if relPath == itemPath {
				// Fallback to basename if prefix trimming didn't work
				relPath = filepath.Base(itemPath)
			}

			resource := Resource{
				Name:        relPath,
				Type:        resType,
				Description: description,
				Path:        itemPath,
				IsDirectory: info.IsDir(),
				Size:        size,
				Tags:        extractTags(itemPath, &dirEntryWrapper{info}),
			}

			categoryResources = append(categoryResources, resource)
		}

		if len(categoryResources) > 0 {
			allResources = append(allResources, categoryResources...)
			summary[resType] = len(categoryResources)
		}
	}

	return &ListResponse{
		Resources: allResources,
		Summary:   summary,
		Filter:    filter,
		Type:      resourceType,
	}, nil
}

// Output formatting functions
func outputListJSON(cmd *cobra.Command, response *ListResponse) error {
	jsonData, err := json.MarshalIndent(response, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal JSON: %w", err)
	}
	cmd.Println(string(jsonData))
	return nil
}

func outputListTree(cmd *cobra.Command, response *ListResponse) error {
	return displayTreeOutput(cmd, response.Resources, response.Filter)
}

func outputListHuman(cmd *cobra.Command, response *ListResponse, filter, resourceType string) error {
	// Human-readable output
	if len(response.Resources) == 0 {
		cmd.Println("📋 No DDx resources found")
		if filter != "" {
			cmd.Printf("No resources match filter: '%s'\n", filter)
		}
		return nil
	}

	cmd.Println("📋 Available DDx Resources")
	if filter != "" {
		cmd.Printf("Filtered by: '%s'\n", filter)
	}
	cmd.Println()

	// Show summary if listing all types
	if resourceType == "" && len(response.Summary) > 1 {
		cmd.Println("Summary:")
		caser := cases.Title(language.English)
		for resType, count := range response.Summary {
			cmd.Printf("  %s: %d items\n", caser.String(resType), count)
		}
		cmd.Println()
	}

	// Group resources by type for display
	resourcesByType := make(map[string][]Resource)
	for _, resource := range response.Resources {
		resourcesByType[resource.Type] = append(resourcesByType[resource.Type], resource)
	}

	// Display each category
	resourceTypes := []string{"templates", "workflows", "mcp-servers", "prompts", "personas", "configs", "scripts", "tools", "environments"}
	caser := cases.Title(language.English)
	for _, resType := range resourceTypes {
		resources, exists := resourcesByType[resType]
		if !exists || len(resources) == 0 {
			continue
		}

		cmd.Printf("%s:\n", caser.String(resType))
		for _, resource := range resources {
			if resource.IsDirectory {
				cmd.Printf("  📁 %s", resource.Name)
			} else {
				cmd.Printf("  📄 %s", resource.Name)
			}

			if resource.Description != "" {
				cmd.Printf(" - %s", resource.Description)
			}
			cmd.Println()
		}
		cmd.Println()
	}

	// Show usage examples
	cmd.Println("Usage examples:")
	cmd.Println("  ddx mcp install github     # Install GitHub MCP server")
	cmd.Println("  ddx list --filter react    # Search for react-related items")
	cmd.Println("  ddx list --json            # Output as JSON")

	return nil
}

// getResourceInfo returns descriptive information about a resource
func getResourceInfo(path string, entry os.DirEntry) string {
	// Try to read description from README or description file
	if entry.IsDir() {
		readmePath := filepath.Join(path, "README.md")
		if content, err := os.ReadFile(readmePath); err == nil {
			lines := strings.Split(string(content), "\n")
			for _, line := range lines {
				line = strings.TrimSpace(line)
				if line != "" && !strings.HasPrefix(line, "#") {
					if len(line) > 60 {
						return line[:60] + "..."
					}
					return line
				}
			}
		}

		// Count items in directory
		if entries, err := os.ReadDir(path); err == nil {
			return fmt.Sprintf("%d items", len(entries))
		}
	} else {
		// For files, show size or type
		if stat, err := entry.Info(); err == nil {
			ext := strings.ToLower(filepath.Ext(entry.Name()))
			switch ext {
			case ".md":
				return "Markdown document"
			case ".yml", ".yaml":
				return "Configuration file"
			case ".sh":
				return "Shell script"
			case ".py":
				return "Python script"
			case ".js":
				return "JavaScript file"
			case ".go":
				return "Go source"
			default:
				if stat.Size() < 1024 {
					return fmt.Sprintf("%d bytes", stat.Size())
				} else if stat.Size() < 1024*1024 {
					return fmt.Sprintf("%.1f KB", float64(stat.Size())/1024)
				} else {
					return fmt.Sprintf("%.1f MB", float64(stat.Size())/(1024*1024))
				}
			}
		}
	}

	return ""
}

// extractTags extracts tags from resource metadata or filename
func extractTags(path string, entry os.DirEntry) []string {
	var tags []string

	// Extract tags from filename patterns
	name := strings.ToLower(entry.Name())

	// Common technology tags
	if strings.Contains(name, "react") || strings.Contains(name, "jsx") {
		tags = append(tags, "react")
	}
	if strings.Contains(name, "vue") {
		tags = append(tags, "vue")
	}
	if strings.Contains(name, "angular") {
		tags = append(tags, "angular")
	}
	if strings.Contains(name, "nextjs") || strings.Contains(name, "next") {
		tags = append(tags, "nextjs")
	}
	if strings.Contains(name, "python") || strings.Contains(name, "py") {
		tags = append(tags, "python")
	}
	if strings.Contains(name, "go") || strings.Contains(name, "golang") {
		tags = append(tags, "go")
	}
	if strings.Contains(name, "javascript") || strings.Contains(name, "js") {
		tags = append(tags, "javascript")
	}
	if strings.Contains(name, "typescript") || strings.Contains(name, "ts") {
		tags = append(tags, "typescript")
	}
	if strings.Contains(name, "docker") {
		tags = append(tags, "docker")
	}
	if strings.Contains(name, "api") || strings.Contains(name, "rest") {
		tags = append(tags, "api")
	}
	if strings.Contains(name, "auth") {
		tags = append(tags, "authentication")
	}
	if strings.Contains(name, "test") {
		tags = append(tags, "testing")
	}
	if strings.Contains(name, "claude") || strings.Contains(name, "ai") {
		tags = append(tags, "ai")
	}

	return tags
}

// discoverResourcesManually discovers resources without filtering
func discoverResourcesManually(libPath, resourceType string) []string {
	var resources []string
	resourcePath := filepath.Join(libPath, resourceType)

	if _, err := os.Stat(resourcePath); os.IsNotExist(err) {
		return resources
	}

	// Only read the immediate children of the resource directory
	entries, err := os.ReadDir(resourcePath)
	if err != nil {
		return resources
	}

	for _, entry := range entries {
		fullPath := filepath.Join(resourcePath, entry.Name())
		resources = append(resources, fullPath)
	}

	return resources
}

// dirEntryWrapper wraps os.FileInfo to implement os.DirEntry interface
type dirEntryWrapper struct {
	info os.FileInfo
}

func (d *dirEntryWrapper) Name() string {
	return d.info.Name()
}

func (d *dirEntryWrapper) IsDir() bool {
	return d.info.IsDir()
}

func (d *dirEntryWrapper) Type() os.FileMode {
	return d.info.Mode().Type()
}

func (d *dirEntryWrapper) Info() (os.FileInfo, error) {
	return d.info, nil
}

// displayTreeOutput displays resources in tree format
func displayTreeOutput(cmd *cobra.Command, resources []Resource, filter string) error {
	if len(resources) == 0 {
		cmd.Println("📋 No DDx resources found")
		if filter != "" {
			cmd.Printf("No resources match filter: '%s'\n", filter)
		}
		return nil
	}

	cmd.Println("📋 DDx Resources Tree")
	if filter != "" {
		cmd.Printf("Filtered by: '%s'\n", filter)
	}
	cmd.Println()

	// Group resources by type
	resourcesByType := make(map[string][]Resource)
	for _, resource := range resources {
		resourcesByType[resource.Type] = append(resourcesByType[resource.Type], resource)
	}

	// Sort types
	types := []string{"templates", "workflows", "mcp-servers", "prompts", "personas", "configs", "scripts", "tools", "environments"}
	for _, resourceType := range types {
		typeResources, exists := resourcesByType[resourceType]
		if !exists || len(typeResources) == 0 {
			continue
		}

		// Display type header
		cmd.Printf("📁 %s\n", resourceType)

		// Build tree structure for this type
		treeNodes := buildTreeStructure(typeResources)
		displayTreeNodes(cmd, treeNodes, "")
		cmd.Println()
	}

	return nil
}

// TreeNode represents a node in the resource tree
type TreeNode struct {
	Name        string
	IsDirectory bool
	Children    map[string]*TreeNode
	Resource    *Resource
}

// buildTreeStructure builds a tree structure from flat resource list
func buildTreeStructure(resources []Resource) map[string]*TreeNode {
	root := make(map[string]*TreeNode)

	for _, resource := range resources {
		parts := strings.Split(resource.Name, "/")
		current := root

		// Build path through tree
		for i, part := range parts {
			if current[part] == nil {
				current[part] = &TreeNode{
					Name:        part,
					IsDirectory: i < len(parts)-1 || resource.IsDirectory,
					Children:    make(map[string]*TreeNode),
				}
			}

			// If this is the last part, store the resource
			if i == len(parts)-1 {
				current[part].Resource = &resource
			}

			current = current[part].Children
		}
	}

	return root
}

// displayTreeNodes recursively displays tree nodes
func displayTreeNodes(cmd *cobra.Command, nodes map[string]*TreeNode, prefix string) {
	// Sort node names for consistent output
	var names []string
	for name := range nodes {
		names = append(names, name)
	}
	sort.Strings(names)

	for i, name := range names {
		node := nodes[name]
		isLast := i == len(names)-1

		// Choose tree characters
		var connector, nextPrefix string
		if isLast {
			connector = "└── "
			nextPrefix = prefix + "    "
		} else {
			connector = "├── "
			nextPrefix = prefix + "│   "
		}

		// Display node
		icon := "📄"
		if node.IsDirectory {
			icon = "📁"
		}

		displayName := name
		if node.Resource != nil && node.Resource.Description != "" {
			displayName += " - " + node.Resource.Description
		}

		cmd.Printf("%s%s%s %s\n", prefix, connector, icon, displayName)

		// Display children
		if len(node.Children) > 0 {
			displayTreeNodes(cmd, node.Children, nextPrefix)
		}
	}
}
