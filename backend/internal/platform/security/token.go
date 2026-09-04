package security

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
)

func NewToken() (string, [sha256.Size]byte, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", [sha256.Size]byte{}, fmt.Errorf("generate token: %w", err)
	}

	encoded := base64.RawURLEncoding.EncodeToString(raw)
	return encoded, sha256.Sum256([]byte(encoded)), nil
}

func HashToken(token string) [sha256.Size]byte {
	return sha256.Sum256([]byte(token))
}
