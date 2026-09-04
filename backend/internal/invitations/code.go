package invitations

import (
	"crypto/rand"
	"crypto/sha256"
	"errors"
	"strings"
)

const codeAlphabet = "23456789ABCDEFGHJKLMNPQRSTUVWXYZ"

var (
	ErrInvalidCodeFormat = errors.New("invalid invitation code format")
	ErrInvalidOrUsed     = errors.New("invitation code is invalid or already used")
)

// GenerateCode creates a cryptographically random code in the XXXX-XXXX format.
// The 32-character alphabet deliberately excludes 0, 1, I, and O.
func GenerateCode() (string, error) {
	random := make([]byte, 8)
	if _, err := rand.Read(random); err != nil {
		return "", err
	}
	code := make([]byte, 9)
	for index := range random {
		target := index
		if index >= 4 {
			target++
		}
		code[target] = codeAlphabet[int(random[index])&31]
	}
	code[4] = '-'
	return string(code), nil
}

func NormalizeCode(value string) (string, error) {
	value = strings.ToUpper(strings.TrimSpace(value))
	if len(value) != 9 || value[4] != '-' {
		return "", ErrInvalidCodeFormat
	}
	for index, character := range value {
		if index == 4 {
			continue
		}
		if !strings.ContainsRune(codeAlphabet, character) {
			return "", ErrInvalidCodeFormat
		}
	}
	return value, nil
}

func HashCode(normalizedCode string) [sha256.Size]byte {
	return sha256.Sum256([]byte(normalizedCode))
}

func CodeSuffix(normalizedCode string) string {
	if len(normalizedCode) != 9 {
		return ""
	}
	return normalizedCode[5:]
}
