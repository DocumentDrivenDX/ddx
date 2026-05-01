package server

import "embed"

//go:embed all:frontend/build
var frontendFiles embed.FS
