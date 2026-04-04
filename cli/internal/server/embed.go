package server

import "embed"

//go:embed all:frontend/dist
var frontendFiles embed.FS
