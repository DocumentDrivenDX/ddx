package agent

import (
	"crypto/rand"
	"encoding/hex"
)

func containsString(slice []string, value string) bool {
	for _, candidate := range slice {
		if candidate == value {
			return true
		}
	}
	return false
}

func genSessionID() string {
	bytes := make([]byte, 4)
	_, _ = rand.Read(bytes)
	return "as-" + hex.EncodeToString(bytes)
}
