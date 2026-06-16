package cmd

import (
	"fmt"
	"io/fs"
	"path/filepath"

	"github.com/DocumentDrivenDX/ddx/internal/registry/defaultplugin"
	"github.com/DocumentDrivenDX/ddx/internal/skills"
)

func syncBuiltinDDxSkillAdapters(projectRoot string, force bool) ([]string, error) {
	if projectRoot == "" {
		return nil, fmt.Errorf("project root is required")
	}
	skillFS, err := fs.Sub(defaultplugin.FS(), "skills")
	if err != nil {
		return nil, fmt.Errorf("open baked-in ddx skills: %w", err)
	}
	if force {
		for _, surface := range []string{filepath.Join(projectRoot, ".agents", "skills"), filepath.Join(projectRoot, ".claude", "skills")} {
			cleanupBootstrapSkills(surface, []string{"ddx"})
		}
	}
	if err := skills.Install(skillFS, projectRoot, skills.Options{Force: force}); err != nil {
		return nil, fmt.Errorf("sync baked-in ddx skill adapters: %w", err)
	}
	return []string{
		filepath.Join(".agents", "skills", "ddx"),
		filepath.Join(".claude", "skills", "ddx"),
	}, nil
}
