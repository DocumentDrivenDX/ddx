package artifact

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

// Manager handles artifact creation, listing, and validation.
type Manager struct {
	Dir string // Base directory for artifacts (e.g. "docs/adr")
}

// NewADRManager creates a manager for ADRs.
func NewADRManager(dir string) *Manager {
	if dir == "" {
		dir = envOr("DDX_ADR_DIR", "docs/adr")
	}
	return &Manager{Dir: dir}
}

// NewSDManager creates a manager for Solution Designs.
func NewSDManager(dir string) *Manager {
	if dir == "" {
		dir = envOr("DDX_SD_DIR", "docs/designs")
	}
	return &Manager{Dir: dir}
}

// Create scaffolds a new artifact from a template.
func (m *Manager) Create(artType ArtifactType, title string, dependsOn []string) (string, error) {
	if strings.TrimSpace(title) == "" {
		return "", fmt.Errorf("artifact: title is required")
	}

	nextID, err := m.nextID(artType)
	if err != nil {
		return "", err
	}

	slug := slugify(title)
	if slug == "" {
		return "", fmt.Errorf("artifact: title produces empty slug")
	}

	filename := fmt.Sprintf("%s-%03d-%s.md", artType, nextID, slug)
	path := filepath.Join(m.Dir, filename)

	if err := os.MkdirAll(m.Dir, 0o755); err != nil {
		return "", fmt.Errorf("artifact: mkdir: %w", err)
	}

	idStr := fmt.Sprintf("%s-%03d", artType, nextID)

	var content string
	switch artType {
	case TypeADR:
		content = adrTemplate(idStr, title, dependsOn)
	case TypeSD:
		content = sdTemplate(idStr, title, dependsOn)
	default:
		return "", fmt.Errorf("artifact: unknown type: %s", artType)
	}

	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		return "", fmt.Errorf("artifact: write: %w", err)
	}

	return path, nil
}

// List scans the directory for artifacts and returns their metadata.
func (m *Manager) List(artType ArtifactType) ([]ArtifactInfo, error) {
	pattern := filepath.Join(m.Dir, fmt.Sprintf("%s-*.md", artType))
	matches, err := filepath.Glob(pattern)
	if err != nil {
		return nil, fmt.Errorf("artifact: glob: %w", err)
	}

	sort.Strings(matches)
	var infos []ArtifactInfo

	for _, path := range matches {
		info, err := m.parseInfo(path, artType)
		if err != nil {
			continue // skip unparseable files
		}
		infos = append(infos, info)
	}

	return infos, nil
}

// Show reads and returns the full content of an artifact by ID.
func (m *Manager) Show(artType ArtifactType, id string) (string, string, error) {
	// Find the file matching the ID
	pattern := filepath.Join(m.Dir, fmt.Sprintf("%s-*.md", artType))
	matches, err := filepath.Glob(pattern)
	if err != nil {
		return "", "", fmt.Errorf("artifact: glob: %w", err)
	}

	for _, path := range matches {
		meta, err := parseFrontmatter(path)
		if err != nil {
			continue
		}
		if meta.ID == id {
			data, err := os.ReadFile(path)
			if err != nil {
				return "", "", err
			}
			return string(data), path, nil
		}
	}

	return "", "", fmt.Errorf("artifact: not found: %s", id)
}

// Validate checks an artifact file for structural correctness.
func (m *Manager) Validate(artType ArtifactType, path string) []ValidationError {
	var errs []ValidationError

	// Check frontmatter
	meta, err := parseFrontmatter(path)
	if err != nil {
		errs = append(errs, ValidationError{Path: path, Message: "missing or invalid dun frontmatter"})
		return errs
	}

	if meta.ID == "" {
		errs = append(errs, ValidationError{Path: path, Message: "missing dun.id"})
	}

	// Check required sections
	content, err := os.ReadFile(path)
	if err != nil {
		errs = append(errs, ValidationError{Path: path, Message: fmt.Sprintf("cannot read: %v", err)})
		return errs
	}

	sections := extractSections(string(content))

	var required []string
	switch artType {
	case TypeADR:
		required = []string{"Context", "Decision", "Alternatives"}
	case TypeSD:
		required = []string{"Scope", "Acceptance Criteria", "Solution Approaches"}
	}

	for _, sec := range required {
		if !containsSection(sections, sec) {
			errs = append(errs, ValidationError{
				Path:    path,
				Message: fmt.Sprintf("missing required section: %s", sec),
			})
		}
	}

	return errs
}

// ValidateAll validates all artifacts of the given type in the directory.
func (m *Manager) ValidateAll(artType ArtifactType) []ValidationError {
	pattern := filepath.Join(m.Dir, fmt.Sprintf("%s-*.md", artType))
	matches, _ := filepath.Glob(pattern)

	var allErrs []ValidationError
	for _, path := range matches {
		errs := m.Validate(artType, path)
		allErrs = append(allErrs, errs...)
	}
	return allErrs
}

// nextID finds the maximum existing ID number and returns max+1.
func (m *Manager) nextID(artType ArtifactType) (int, error) {
	pattern := filepath.Join(m.Dir, fmt.Sprintf("%s-*.md", artType))
	matches, _ := filepath.Glob(pattern)

	re := regexp.MustCompile(fmt.Sprintf(`%s-(\d+)`, artType))
	maxID := 0

	for _, path := range matches {
		base := filepath.Base(path)
		m := re.FindStringSubmatch(base)
		if len(m) >= 2 {
			n, err := strconv.Atoi(m[1])
			if err == nil && n > maxID {
				maxID = n
			}
		}
	}

	return maxID + 1, nil
}

func (m *Manager) parseInfo(path string, artType ArtifactType) (ArtifactInfo, error) {
	meta, err := parseFrontmatter(path)
	if err != nil {
		// Fall back to filename parsing
		base := filepath.Base(path)
		return ArtifactInfo{
			Path:  path,
			Title: base,
			Type:  artType,
		}, nil
	}

	title := extractTitle(path)

	info := ArtifactInfo{
		ID:    meta.ID,
		Title: title,
		Path:  path,
		Type:  artType,
	}

	// Try to extract status from ADR table
	if artType == TypeADR {
		info.Status, info.Date = extractADRStatusAndDate(path)
	}

	return info, nil
}

func parseFrontmatter(path string) (*ArtifactMeta, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)

	// Find opening ---
	if !scanner.Scan() || strings.TrimSpace(scanner.Text()) != "---" {
		return nil, fmt.Errorf("no frontmatter")
	}

	var yamlLines []string
	for scanner.Scan() {
		line := scanner.Text()
		if strings.TrimSpace(line) == "---" {
			break
		}
		yamlLines = append(yamlLines, line)
	}

	var fm DunFrontmatter
	if err := yaml.Unmarshal([]byte(strings.Join(yamlLines, "\n")), &fm); err != nil {
		return nil, err
	}

	if fm.Dun.ID == "" {
		return nil, fmt.Errorf("no dun.id")
	}

	return &fm.Dun, nil
}

func extractTitle(path string) string {
	f, err := os.Open(path)
	if err != nil {
		return filepath.Base(path)
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	inFrontmatter := false
	for scanner.Scan() {
		line := scanner.Text()
		if strings.TrimSpace(line) == "---" {
			if !inFrontmatter {
				inFrontmatter = true
				continue
			}
			inFrontmatter = false
			continue
		}
		if inFrontmatter {
			continue
		}
		if strings.HasPrefix(line, "# ") {
			// Strip ADR/SD prefix from title
			title := strings.TrimPrefix(line, "# ")
			if idx := strings.Index(title, ": "); idx >= 0 {
				return title[idx+2:]
			}
			return title
		}
	}
	return filepath.Base(path)
}

func extractADRStatusAndDate(path string) (string, string) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", ""
	}

	// Look for | Date | Status | pattern
	lines := strings.Split(string(data), "\n")
	for i, line := range lines {
		if strings.Contains(line, "| Date") && strings.Contains(line, "| Status") {
			// Data row is 2 lines after header (skip separator)
			if i+2 < len(lines) {
				cells := strings.Split(lines[i+2], "|")
				if len(cells) >= 3 {
					date := strings.TrimSpace(cells[1])
					status := strings.TrimSpace(cells[2])
					return status, date
				}
			}
		}
	}
	return "", ""
}

func extractSections(content string) []string {
	var sections []string
	for _, line := range strings.Split(content, "\n") {
		if strings.HasPrefix(line, "## ") {
			sections = append(sections, strings.TrimPrefix(line, "## "))
		}
	}
	return sections
}

func containsSection(sections []string, name string) bool {
	for _, s := range sections {
		if strings.EqualFold(strings.TrimSpace(s), name) {
			return true
		}
	}
	return false
}

func slugify(s string) string {
	s = strings.ToLower(s)
	re := regexp.MustCompile(`[^a-z0-9]+`)
	s = re.ReplaceAllString(s, "-")
	s = strings.Trim(s, "-")
	return s
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
