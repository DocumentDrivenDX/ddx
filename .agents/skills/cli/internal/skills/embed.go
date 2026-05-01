package skills

import "embed"

// SkillFiles contains the single portable `ddx` skill tree. The content
// is copied from the repo-root `/skills/ddx/` tree by `make copy-skills`
// (run automatically as a prereq of `make build`, `make test`, `make dev`).
// //go:embed cannot traverse upward out of the package directory, so we
// copy into this package and embed from here.
//
//go:embed all:ddx
var SkillFiles embed.FS
