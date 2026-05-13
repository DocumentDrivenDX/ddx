package bead

import "errors"

var (
	ErrNotFound          = errors.New("bead: not found")
	ErrConflict          = errors.New("bead: id already exists")
	ErrInvalidID         = errors.New("bead: invalid id")
	ErrAlreadyClaimed    = errors.New("bead: already claimed by another owner")
	ErrNotClaimedByOwner = errors.New("bead: not claimed by requesting owner")
	ErrUnsupported       = errors.New("bead: operation not supported by this backend")
)
