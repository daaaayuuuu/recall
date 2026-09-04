package invitations

import (
	"strings"
	"testing"
)

func TestGenerateCodeUsesRequestedFormatAndAlphabet(t *testing.T) {
	seen := make(map[string]struct{})
	for index := 0; index < 1_000; index++ {
		code, err := GenerateCode()
		if err != nil {
			t.Fatal(err)
		}
		if len(code) != 9 || code[4] != '-' {
			t.Fatalf("generated code has wrong format: %q", code)
		}
		if normalized, err := NormalizeCode(code); err != nil || normalized != code {
			t.Fatalf("generated code was not valid: code=%q normalized=%q err=%v", code, normalized, err)
		}
		if _, duplicate := seen[code]; duplicate {
			t.Fatalf("unexpected duplicate in small generation sample: %q", code)
		}
		seen[code] = struct{}{}
	}
}

func TestNormalizeCodeAcceptsLowercaseAndRejectsAmbiguousOrMalformedValues(t *testing.T) {
	normalized, err := NormalizeCode("  7kdm-n4px  ")
	if err != nil || normalized != "7KDM-N4PX" {
		t.Fatalf("normalized=%q err=%v", normalized, err)
	}
	if CodeSuffix(normalized) != "N4PX" {
		t.Fatalf("suffix=%q", CodeSuffix(normalized))
	}

	for _, value := range []string{"", "7KDMN4PX", "7KDM-N4P", "7KDM-N4PX-", "0KDM-N4PX", "IKDM-N4PX", "7KDM-NOPX"} {
		if _, err := NormalizeCode(value); err == nil {
			t.Fatalf("expected %q to be rejected", value)
		}
	}

	first := HashCode(normalized)
	second := HashCode(strings.ToUpper(normalized))
	if first != second {
		t.Fatal("same normalized code produced different hashes")
	}
}
