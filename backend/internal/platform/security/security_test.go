package security

import (
	"strings"
	"testing"
)

func TestNewIDProducesULIDShape(t *testing.T) {
	id, err := NewID()
	if err != nil {
		t.Fatal(err)
	}
	if len(id) != 26 {
		t.Fatalf("unexpected id length: %q", id)
	}
	for _, character := range id {
		if !strings.ContainsRune(crockfordAlphabet, character) {
			t.Fatalf("unexpected id character %q", character)
		}
	}
}

func TestPasswordHashAndVerify(t *testing.T) {
	hasher := NewPasswordHasherWithParams(PasswordParams{Memory: 1024, Iterations: 1, Parallelism: 1, SaltLength: 16, KeyLength: 32})
	encoded, err := hasher.Hash("correct horse battery staple")
	if err != nil {
		t.Fatal(err)
	}

	ok, err := hasher.Verify("correct horse battery staple", encoded)
	if err != nil || !ok {
		t.Fatalf("valid password rejected: ok=%v err=%v", ok, err)
	}
	ok, err = hasher.Verify("incorrect password", encoded)
	if err != nil || ok {
		t.Fatalf("invalid password accepted: ok=%v err=%v", ok, err)
	}
}

func TestValidatePasswordCountsUnicodeCharacters(t *testing.T) {
	if err := ValidatePassword("短密码"); err == nil {
		t.Fatal("expected short password to be rejected")
	}
	if err := ValidatePassword("八个字符密码足够"); err != nil {
		t.Fatalf("expected unicode password to be accepted: %v", err)
	}
}
