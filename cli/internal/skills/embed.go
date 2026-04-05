package skills

import "embed"

//go:embed all:ddx-bead all:ddx-agent all:ddx-install all:ddx-status all:ddx-review all:ddx-run
var SkillFiles embed.FS
