package aisettings

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
)

type secretCipher struct {
	aead       cipher.AEAD
	keyVersion int
}

func newSecretCipher(encodedKey string, keyVersion int) (*secretCipher, error) {
	key, err := base64.StdEncoding.DecodeString(encodedKey)
	if err != nil {
		return nil, fmt.Errorf("decode AI config encryption key: %w", err)
	}
	if len(key) != 32 {
		return nil, fmt.Errorf("AI config encryption key must decode to 32 bytes, got %d", len(key))
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("create AI config cipher: %w", err)
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("create AI config AEAD: %w", err)
	}
	if keyVersion < 1 {
		return nil, errors.New("AI config encryption key version must be positive")
	}
	return &secretCipher{aead: aead, keyVersion: keyVersion}, nil
}

func (value *secretCipher) encrypt(plaintext string, additionalData []byte) (SecretEnvelope, error) {
	if plaintext == "" {
		return SecretEnvelope{}, nil
	}
	nonce := make([]byte, value.aead.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return SecretEnvelope{}, fmt.Errorf("generate AI config nonce: %w", err)
	}
	return SecretEnvelope{
		Ciphertext: value.aead.Seal(nil, nonce, []byte(plaintext), additionalData),
		Nonce:      nonce,
	}, nil
}

func (value *secretCipher) decrypt(secret SecretEnvelope, additionalData []byte, keyVersion int) (string, error) {
	if len(secret.Ciphertext) == 0 && len(secret.Nonce) == 0 {
		return "", nil
	}
	if keyVersion != value.keyVersion {
		return "", fmt.Errorf("unsupported AI config encryption key version %d", keyVersion)
	}
	plaintext, err := value.aead.Open(nil, secret.Nonce, secret.Ciphertext, additionalData)
	if err != nil {
		return "", errors.New("decrypt AI configuration secret")
	}
	return string(plaintext), nil
}

func secretAAD(recordID, capability string) []byte {
	return []byte("gamegen:ai-settings:" + recordID + ":" + capability)
}
