package evidence

import "testing"

func TestDefaultCapsValues(t *testing.T) {
	c := DefaultCaps()
	if c.MaxPromptBytes != 4*1024*1024 {
		t.Errorf("MaxPromptBytes = %d, want 4 MiB", c.MaxPromptBytes)
	}
	if c.MaxInlinedFileBytes != 512*1024 {
		t.Errorf("MaxInlinedFileBytes = %d, want 512 KiB", c.MaxInlinedFileBytes)
	}
	if c.MaxDiffBytes != 2*1024*1024 {
		t.Errorf("MaxDiffBytes = %d, want 2 MiB", c.MaxDiffBytes)
	}
	if c.MaxGoverningDocBytes != 256*1024 {
		t.Errorf("MaxGoverningDocBytes = %d, want 256 KiB", c.MaxGoverningDocBytes)
	}
}

func intp(v int) *int { return &v }

func TestApplyOverride(t *testing.T) {
	c := DefaultCaps().Apply(CapsOverride{
		MaxPromptBytes: intp(100),
	})
	if c.MaxPromptBytes != 100 {
		t.Errorf("MaxPromptBytes override not applied: %d", c.MaxPromptBytes)
	}
	if c.MaxDiffBytes != DefaultMaxDiffBytes {
		t.Errorf("Unset override mutated MaxDiffBytes: %d", c.MaxDiffBytes)
	}
}

func TestResolveCapsPrecedence(t *testing.T) {
	project := CapsOverride{MaxPromptBytes: intp(100), MaxDiffBytes: intp(200)}
	role := CapsOverride{MaxPromptBytes: intp(50)}

	// Role wins where set; project still applies elsewhere.
	c := ResolveCaps(project, role)
	if c.MaxPromptBytes != 50 {
		t.Errorf("role override not applied: %d", c.MaxPromptBytes)
	}
	if c.MaxDiffBytes != 200 {
		t.Errorf("project layer lost when role applied: %d", c.MaxDiffBytes)
	}

	// An empty role override leaves project values intact.
	c = ResolveCaps(project, CapsOverride{})
	if c.MaxPromptBytes != 100 {
		t.Errorf("empty role should not change project MaxPromptBytes: %d", c.MaxPromptBytes)
	}
}
