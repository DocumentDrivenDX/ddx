package bead

import (
	"context"
	cryptoRand "crypto/rand"
	"encoding/hex"
	"fmt"
	"regexp"
)

const (
	DefaultIDPrefix = "ddx-"
	MaxIDLength     = 64
	MinIDLength     = 8
)

var idCharsetRe = regexp.MustCompile(`^[a-zA-Z0-9-]+$`)

// ValidateID reports whether id matches the bead identifier contract.
func ValidateID(id string) error {
	if len(id) < MinIDLength || len(id) > MaxIDLength {
		return fmt.Errorf("%w: length %d not in [%d, %d]", ErrInvalidID, len(id), MinIDLength, MaxIDLength)
	}
	if !idCharsetRe.MatchString(id) {
		return fmt.Errorf("%w: charset", ErrInvalidID)
	}
	return nil
}

// RandomHexIDGenerator emits a prefixed random hex identifier.
type RandomHexIDGenerator struct {
	Prefix string
	Bytes  int
}

// GenID generates a random hex identifier with the configured prefix.
func (g RandomHexIDGenerator) GenID(ctx context.Context) (string, error) {
	if ctx != nil {
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		default:
		}
	}
	prefix := g.Prefix
	if prefix == "" {
		prefix = DefaultIDPrefix
	}
	numBytes := g.Bytes
	if numBytes <= 0 {
		numBytes = 4
	}
	buf := make([]byte, numBytes)
	if _, err := cryptoRand.Read(buf); err != nil {
		return "", fmt.Errorf("bead: gen id: %w", err)
	}
	id := prefix + hex.EncodeToString(buf)
	if err := ValidateID(id); err != nil {
		return "", err
	}
	return id, nil
}
