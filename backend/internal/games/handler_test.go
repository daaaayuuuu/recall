package games

import "testing"

func TestValidateGameMetadata(t *testing.T) {
	title, description, fields := validateGameMetadata("  一段旅程  ", "  夏日回忆  ")
	if len(fields) != 0 {
		t.Fatalf("expected valid metadata, got %#v", fields)
	}
	if title != "一段旅程" || !description.Valid || description.String != "夏日回忆" {
		t.Fatalf("unexpected normalized metadata: %q %#v", title, description)
	}
}

func TestValidateGameMetadataRejectsInvalidLengths(t *testing.T) {
	_, _, fields := validateGameMetadata("   ", string(make([]rune, 501)))
	if fields["title"] == "" || fields["description"] == "" {
		t.Fatalf("expected title and description validation errors, got %#v", fields)
	}
}

func TestUploadGateLimitsPerUser(t *testing.T) {
	gate := newUploadGate(2)
	if !gate.acquire("creator-a") || !gate.acquire("creator-a") {
		t.Fatal("expected first two uploads to be accepted")
	}
	if gate.acquire("creator-a") {
		t.Fatal("expected third upload for the same creator to be rejected")
	}
	if !gate.acquire("creator-b") {
		t.Fatal("expected independent creator limit")
	}
	gate.release("creator-a")
	if !gate.acquire("creator-a") {
		t.Fatal("expected capacity after release")
	}
}

func TestDecodePreviewDocumentValidatesArtifactAndVersion(t *testing.T) {
	preview := Preview{TemplateID: "memory-game", TemplateVersion: "1.0.0"}
	valid := []byte(`{"templateId":"memory-game","templateVersion":"1.0.0","configVersion":1,"config":{"openingTitle":"夏日回忆","rounds":[]}}`)

	document, err := decodePreviewDocument(valid, preview)
	if err != nil {
		t.Fatalf("expected valid preview config, got %v", err)
	}
	if document.Config.OpeningTitle != "夏日回忆" {
		t.Fatalf("unexpected opening title %q", document.Config.OpeningTitle)
	}

	mismatch := preview
	mismatch.TemplateVersion = "2.0.0"
	if _, err := decodePreviewDocument(valid, mismatch); err == nil {
		t.Fatal("expected template mismatch to be rejected")
	}
	if _, err := decodePreviewDocument([]byte(`{"templateId":"memory-game"}`), preview); err == nil {
		t.Fatal("expected invalid artifact to be rejected")
	}
}
