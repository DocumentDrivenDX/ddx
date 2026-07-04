package storage

import bead "github.com/DocumentDrivenDX/ddx/internal/bead"

type ExternalBackend = bead.ExternalBackend

var NewExternalBackend = bead.NewExternalBackend
var NewExternalBackendWithFallback = bead.NewExternalBackendWithFallback
