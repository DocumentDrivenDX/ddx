package graphql

import (
	"encoding/base64"
	"strings"
)

// encodeStableCursor encodes a stable string key (typically an entity ID) as
// an opaque, base64-encoded cursor. Unlike index-based cursors, stable cursors
// remain valid when the underlying collection is reordered or mutated between
// pages.
func encodeStableCursor(id string) string {
	return base64.StdEncoding.EncodeToString([]byte("cursor:" + id))
}

// decodeStableCursor decodes a cursor back to its stable key.
// Returns the key and true on success, or "", false if the cursor is invalid.
func decodeStableCursor(cursor string) (string, bool) {
	b, err := base64.StdEncoding.DecodeString(cursor)
	if err != nil {
		return "", false
	}
	s := string(b)
	if !strings.HasPrefix(s, "cursor:") {
		return "", false
	}
	return s[7:], true
}
