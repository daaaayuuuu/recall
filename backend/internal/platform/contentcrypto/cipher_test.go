package contentcrypto

import (
	"encoding/base64"
	"testing"
)

func TestCipherRoundTripAndAdditionalData(t *testing.T) {
	key := base64.StdEncoding.EncodeToString(make([]byte, 32))
	cipher, err := New(key, 1)
	if err != nil {
		t.Fatal(err)
	}

	ciphertext, nonce, version, err := cipher.Encrypt([]byte("private memory"), []byte("version-id"))
	if err != nil {
		t.Fatal(err)
	}
	plaintext, err := cipher.Decrypt(ciphertext, nonce, []byte("version-id"), version)
	if err != nil {
		t.Fatal(err)
	}
	if string(plaintext) != "private memory" {
		t.Fatalf("unexpected plaintext %q", plaintext)
	}
	if _, err := cipher.Decrypt(ciphertext, nonce, []byte("different-version"), version); err == nil {
		t.Fatal("expected altered additional data to fail authentication")
	}
}
