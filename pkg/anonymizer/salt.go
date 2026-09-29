package anonymizer

import (
	"crypto/rand"
	"encoding/hex"
)

// DefaultSaltLength is the default number of random bytes used for generating a salt (16 bytes = 32 hex chars).
const DefaultSaltLength = 16

// GenerateSalt creates a cryptographically secure random salt of the specified byte length
// and returns it as a hexadecimal string. If byteLength <= 0, DefaultSaltLength is used.
func GenerateSalt(byteLength int) (string, error) {
	if byteLength <= 0 {
		byteLength = DefaultSaltLength
	}
	buf := make([]byte, byteLength)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}
