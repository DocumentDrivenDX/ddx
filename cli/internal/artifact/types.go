package artifact

// ArtifactType represents the kind of artifact (ADR, SD, etc.)
type ArtifactType string

const (
	TypeADR ArtifactType = "ADR"
	TypeSD  ArtifactType = "SD"
)

// ArtifactMeta holds parsed metadata from an artifact's frontmatter.
type ArtifactMeta struct {
	ID        string   `yaml:"id" json:"id"`
	DependsOn []string `yaml:"depends_on" json:"depends_on"`
}

// DunFrontmatter wraps the dun namespace in YAML frontmatter.
type DunFrontmatter struct {
	Dun ArtifactMeta `yaml:"dun"`
}

// ArtifactInfo represents a discovered artifact for listing.
type ArtifactInfo struct {
	ID     string       `json:"id"`
	Title  string       `json:"title"`
	Status string       `json:"status,omitempty"`
	Date   string       `json:"date,omitempty"`
	Path   string       `json:"path"`
	Type   ArtifactType `json:"type"`
}

// ValidationError represents a structural problem in an artifact.
type ValidationError struct {
	Path    string `json:"path"`
	Message string `json:"message"`
}

func (e ValidationError) Error() string {
	return e.Path + ": " + e.Message
}
