package server

import "embed"

// Keep frontend/build/.gitkeep tracked so fresh checkouts can compile this
// package before Bun populates the real SPA bundle.
//
//go:embed all:frontend/build
var frontendFiles embed.FS
