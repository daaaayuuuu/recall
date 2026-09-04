package security

import (
	"crypto/rand"
	"fmt"
	"time"
)

const crockfordAlphabet = "0123456789ABCDEFGHJKMNPQRSTVWXYZ"

// NewID returns a monotonic-sortable ULID-compatible identifier.
func NewID() (string, error) {
	var source [16]byte
	milliseconds := uint64(time.Now().UTC().UnixMilli())
	source[0] = byte(milliseconds >> 40)
	source[1] = byte(milliseconds >> 32)
	source[2] = byte(milliseconds >> 24)
	source[3] = byte(milliseconds >> 16)
	source[4] = byte(milliseconds >> 8)
	source[5] = byte(milliseconds)
	if _, err := rand.Read(source[6:]); err != nil {
		return "", fmt.Errorf("generate identifier entropy: %w", err)
	}

	encoded := make([]byte, 26)
	for character := range encoded {
		var value byte
		for offset := 0; offset < 5; offset++ {
			value <<= 1
			bit := character*5 + offset - 2
			if bit < 0 {
				continue
			}
			value |= (source[bit/8] >> (7 - (bit % 8))) & 1
		}
		encoded[character] = crockfordAlphabet[value]
	}
	return string(encoded), nil
}
