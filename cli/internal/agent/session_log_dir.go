package agent

import (
	"github.com/DocumentDrivenDX/ddx/internal/config"
)

// SessionLogDirForWorkDir returns the resolved session-log directory for a
// project root: project config when present, otherwise the default log dir
// resolved against workDir.
func SessionLogDirForWorkDir(workDir string) string {
	configured := ""
	if c, err := config.LoadWithWorkingDir(workDir); err == nil && c != nil && c.Agent != nil {
		configured = c.Agent.SessionLogDir
	}
	return ResolveLogDir(workDir, configured)
}
