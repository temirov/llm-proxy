package utils

import (
	"crypto/sha256"
	"encoding/hex"
)

// Fingerprint returns a short hash you can safely log for diagnostics.
func Fingerprint(secret string) string {
	sum := sha256.Sum256([]byte(secret))
	hexed := hex.EncodeToString(sum[:])
	if len(hexed) < 8 {
		return hexed
	}
	return hexed[:8]
}
