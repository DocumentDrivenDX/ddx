package docprose

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// TempValeConfig holds paths to a temporary Vale configuration scoped to one invocation.
type TempValeConfig struct {
	dir     string
	iniPath string
}

// INIPath returns the path to the generated .vale.ini file.
func (t *TempValeConfig) INIPath() string {
	return t.iniPath
}

// Cleanup removes all temporary files created for this config.
func (t *TempValeConfig) Cleanup() {
	os.RemoveAll(t.dir)
}

// NewTempValeConfig generates a temporary Vale configuration from DDx settings.
// StylesPath is pointed at the packaged DDx styles via a symlink so no copy is needed.
// Project vocabulary is rendered into Vale accept/reject files under Vocab/Project/.
// The caller must call Cleanup() when done.
func NewTempValeConfig(settings Settings) (*TempValeConfig, error) {
	assetRoot, err := defaultAssetRoot()
	if err != nil {
		return nil, fmt.Errorf("resolve asset root: %w", err)
	}
	packagedDDxStyles := filepath.Join(assetRoot, "styles", "DDx")

	dir, err := os.MkdirTemp("", "ddx-vale-")
	if err != nil {
		return nil, fmt.Errorf("create temp vale dir: %w", err)
	}

	stylesDir := filepath.Join(dir, "styles")
	if err := os.MkdirAll(stylesDir, 0o755); err != nil {
		os.RemoveAll(dir)
		return nil, fmt.Errorf("create styles dir: %w", err)
	}

	// Symlink the packaged DDx style directory so we avoid copying rule files.
	if err := os.Symlink(packagedDDxStyles, filepath.Join(stylesDir, "DDx")); err != nil {
		os.RemoveAll(dir)
		return nil, fmt.Errorf("symlink DDx styles: %w", err)
	}

	vocabName := ""
	if len(settings.Vocabulary.Accept) > 0 || len(settings.Vocabulary.Reject) > 0 {
		vocabName = "Project"
		vocabDir := filepath.Join(stylesDir, "Vocab", vocabName)
		if err := os.MkdirAll(vocabDir, 0o755); err != nil {
			os.RemoveAll(dir)
			return nil, fmt.Errorf("create vocab dir: %w", err)
		}
		if err := writeTextLines(filepath.Join(vocabDir, "accept.txt"), settings.Vocabulary.Accept); err != nil {
			os.RemoveAll(dir)
			return nil, fmt.Errorf("write accept vocabulary: %w", err)
		}
		if err := writeTextLines(filepath.Join(vocabDir, "reject.txt"), settings.Vocabulary.Reject); err != nil {
			os.RemoveAll(dir)
			return nil, fmt.Errorf("write reject vocabulary: %w", err)
		}
	}

	style := settings.Vale.Style
	if style == "" {
		style = "DDx"
	}

	iniPath := filepath.Join(dir, ".vale.ini")
	ini := buildValeINI(stylesDir, style, vocabName)
	if err := os.WriteFile(iniPath, []byte(ini), 0o644); err != nil {
		os.RemoveAll(dir)
		return nil, fmt.Errorf("write .vale.ini: %w", err)
	}

	return &TempValeConfig{dir: dir, iniPath: iniPath}, nil
}

func buildValeINI(stylesPath, style, vocabName string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "StylesPath = %s\n", stylesPath)
	fmt.Fprintf(&b, "MinAlertLevel = suggestion\n")
	if vocabName != "" {
		fmt.Fprintf(&b, "Vocab = %s\n", vocabName)
	}
	fmt.Fprintf(&b, "\n[*.md]\n")
	fmt.Fprintf(&b, "BasedOnStyles = %s\n", style)
	return b.String()
}

func writeTextLines(path string, lines []string) error {
	content := strings.Join(lines, "\n")
	if len(lines) > 0 {
		content += "\n"
	}
	return os.WriteFile(path, []byte(content), 0o644)
}
