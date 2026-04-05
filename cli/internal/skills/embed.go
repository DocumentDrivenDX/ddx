package skills

import "embed"

//go:embed all:ddx-bead all:ddx-agent all:ddx-install all:ddx-status
var SkillFiles embed.FS
