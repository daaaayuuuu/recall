package contentcrypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
)

type Cipher struct {
	aead       cipher.AEAD
	keyVersion int
}

func New(encodedKey string, keyVersion int) (*Cipher, error) {
	key, err := base64.StdEncoding.DecodeString(encodedKey)
	if err != nil {
		return nil, fmt.Errorf("decode content encryption key: %w", err)
	}
	if len(key) != 32 {
		return nil, fmt.Errorf("content encryption key must decode to 32 bytes, got %d", len(key))
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("create AES cipher: %w", err)
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("create AES-GCM cipher: %w", err)
	}
	if keyVersion < 1 {
		return nil, errors.New("content encryption key version must be positive")
	}
	return &Cipher{aead: aead, keyVersion: keyVersion}, nil
}

func (cipher *Cipher) Encrypt(plaintext, additionalData []byte) (ciphertext, nonce []byte, keyVersion int, err error) {
	nonce = make([]byte, cipher.aead.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, nil, 0, fmt.Errorf("generate content nonce: %w", err)
	}
	ciphertext = cipher.aead.Seal(nil, nonce, plaintext, additionalData)
	return ciphertext, nonce, cipher.keyVersion, nil
}

func (cipher *Cipher) Decrypt(ciphertext, nonce, additionalData []byte, keyVersion int) ([]byte, error) {
	if keyVersion != cipher.keyVersion {
		return nil, fmt.Errorf("unsupported content encryption key version %d", keyVersion)
	}
	plaintext, err := cipher.aead.Open(nil, nonce, ciphertext, additionalData)
	if err != nil {
		return nil, errors.New("decrypt content")
	}
	return plaintext, nil
}
