package docprose

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDocProseValeConfig_NoProjectValeINI(t *testing.T) {
	settings, err := DefaultSettings()
	if err != nil {
		t.Fatal(err)
	}

	cfg, err := NewTempValeConfig(settings)
	if err != nil {
		t.Fatal(err)
	}
	defer cfg.Cleanup()

	data, err := os.ReadFile(cfg.INIPath())
	if err != nil {
		t.Fatalf("read generated .vale.ini: %v", err)
	}
	if len(data) == 0 {
		t.Fatal("generated .vale.ini is empty")
	}
	content := string(data)
	if !strings.Contains(content, "StylesPath") {
		t.Error("generated .vale.ini is missing StylesPath")
	}
	if !strings.Contains(content, "[*.md]") {
		t.Error("generated .vale.ini is missing [*.md] section")
	}
	if !strings.Contains(content, "BasedOnStyles") {
		t.Error("generated .vale.ini is missing BasedOnStyles")
	}
}

func TestDocProseValeConfig_StylesPathUsesPackagedRules(t *testing.T) {
	settings, err := DefaultSettings()
	if err != nil {
		t.Fatal(err)
	}

	cfg, err := NewTempValeConfig(settings)
	if err != nil {
		t.Fatal(err)
	}
	defer cfg.Cleanup()

	// DDx style directory must be accessible via the temp dir's styles subdirectory.
	stylesDir := filepath.Join(filepath.Dir(cfg.INIPath()), "styles")
	ddxStyleDir := filepath.Join(stylesDir, "DDx")
	info, err := os.Stat(ddxStyleDir)
	if err != nil {
		t.Fatalf("DDx style dir not accessible at %s: %v", ddxStyleDir, err)
	}
	if !info.IsDir() {
		t.Fatalf("expected %s to be a directory", ddxStyleDir)
	}

	// The INI file must reference the generated StylesPath.
	data, err := os.ReadFile(cfg.INIPath())
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), stylesDir) {
		t.Errorf("generated .vale.ini does not reference expected styles dir %s\ncontent:\n%s", stylesDir, string(data))
	}
}

func TestDocProseValeConfig_ProjectVocabularyRendered(t *testing.T) {
	settings, err := DefaultSettings()
	if err != nil {
		t.Fatal(err)
	}
	settings.Vocabulary.Accept = []string{"DDx", "bead", "Quartz"}
	settings.Vocabulary.Reject = []string{"system"}

	cfg, err := NewTempValeConfig(settings)
	if err != nil {
		t.Fatal(err)
	}
	defer cfg.Cleanup()

	// Vale 3.x: vocabulary lives under <StylesPath>/config/vocabularies/<name>/.
	stylesDir := filepath.Join(filepath.Dir(cfg.INIPath()), "styles")
	vocabDir := filepath.Join(stylesDir, "config", "vocabularies", "Project")
	acceptPath := filepath.Join(vocabDir, "accept.txt")
	data, err := os.ReadFile(acceptPath)
	if err != nil {
		t.Fatalf("read accept.txt at Vale 3.x path: %v", err)
	}
	acceptContent := string(data)
	for _, term := range []string{"DDx", "bead", "Quartz"} {
		if !strings.Contains(acceptContent, term) {
			t.Errorf("accept.txt missing term %q", term)
		}
	}

	// reject.txt must contain each reject term.
	rejectPath := filepath.Join(vocabDir, "reject.txt")
	data, err = os.ReadFile(rejectPath)
	if err != nil {
		t.Fatalf("read reject.txt at Vale 3.x path: %v", err)
	}
	if !strings.Contains(string(data), "system") {
		t.Error("reject.txt missing term 'system'")
	}

	// Must not write the pre-3.x Vocab/ layout (would leave Vale 3.x looking at an empty path).
	legacyVocabDir := filepath.Join(stylesDir, "Vocab", "Project")
	if _, err := os.Stat(legacyVocabDir); !os.IsNotExist(err) {
		t.Errorf("vocabulary must not be written under legacy path %s (err=%v)", legacyVocabDir, err)
	}

	// The INI must declare Vocab = Project, and that name must resolve to the dir we wrote.
	iniData, err := os.ReadFile(cfg.INIPath())
	if err != nil {
		t.Fatal(err)
	}
	iniContent := string(iniData)
	if !strings.Contains(iniContent, "Vocab = Project") {
		t.Errorf("generated .vale.ini does not declare Vocab = Project\ncontent:\n%s", iniContent)
	}
	info, err := os.Stat(vocabDir)
	if err != nil {
		t.Fatalf("Vocab = Project must resolve to on-disk dir %s: %v", vocabDir, err)
	}
	if !info.IsDir() {
		t.Fatalf("expected %s to be a directory", vocabDir)
	}
}

func TestDocProseValeConfig_CleansTemporaryFiles(t *testing.T) {
	settings, err := DefaultSettings()
	if err != nil {
		t.Fatal(err)
	}
	settings.Vocabulary.Accept = []string{"DDx"}

	cfg, err := NewTempValeConfig(settings)
	if err != nil {
		t.Fatal(err)
	}

	iniPath := cfg.INIPath()
	tempDir := filepath.Dir(iniPath)

	if _, err := os.Stat(iniPath); err != nil {
		t.Fatalf("expected .vale.ini to exist before cleanup: %v", err)
	}

	cfg.Cleanup()

	if _, err := os.Stat(iniPath); !os.IsNotExist(err) {
		t.Errorf("expected .vale.ini to be removed after Cleanup, got: %v", err)
	}
	if _, err := os.Stat(tempDir); !os.IsNotExist(err) {
		t.Errorf("expected temp dir %s to be removed after Cleanup, got: %v", tempDir, err)
	}
}
