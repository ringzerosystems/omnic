package service

import (
	"crypto/rand"
	"encoding/hex"
)

// GenerateSID generates a 34-character Twilio SID with a two-letter prefix and 32 hex chars.
func GenerateSID(prefix string) string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		// Fallback should never fail under normal OS runtime
		for i := range b {
			b[i] = byte(i)
		}
	}
	return prefix + hex.EncodeToString(b)
}
