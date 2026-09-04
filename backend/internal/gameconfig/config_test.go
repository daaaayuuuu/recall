package gameconfig

import "testing"

func TestDocumentValidate(t *testing.T) {
	document := Document{
		TemplateID: "memory-game", TemplateVersion: "1.0.0", ConfigVersion: 1,
		Config: Config{OpeningTitle: "夏日回忆", Rounds: []map[string]any{}},
	}
	if err := document.Validate(); err != nil {
		t.Fatal(err)
	}
	document.TemplateVersion = "2.0.0"
	if err := document.Validate(); err == nil {
		t.Fatal("expected unsupported template version")
	}
}

func TestLoveJourneyPlaceholderDocumentValidate(t *testing.T) {
	document := Document{
		TemplateID: "love-journey", TemplateVersion: "1.0.0", ConfigVersion: 1,
		Config: Config{OpeningTitle: "爱的旅程", Rounds: []map[string]any{}},
	}
	if err := document.Validate(); err != nil {
		t.Fatalf("love journey placeholder config should be valid: %v", err)
	}
}

func TestLoveJourneyUnifiedMaterialDocumentValidate(t *testing.T) {
	document := Document{
		TemplateID: "love-journey", TemplateVersion: "1.1.0", ConfigVersion: 1,
		Config: Config{
			OpeningTitle: "我们的礼物", Rounds: []map[string]any{}, LoveLetter: "写给你的信",
			LetterPassword: "0820", PasswordHint: "第一次见面的日期",
		},
	}
	if err := document.Validate(); err != nil {
		t.Fatalf("expected love journey 1.1 config to be valid, got %v", err)
	}
	for name, mutate := range map[string]func(*Document){
		"missing letter":   func(document *Document) { document.Config.LoveLetter = "" },
		"invalid password": func(document *Document) { document.Config.LetterPassword = "1A20" },
		"long hint":        func(document *Document) { document.Config.PasswordHint = string(make([]rune, 101)) },
	} {
		t.Run(name, func(t *testing.T) {
			invalid := document
			mutate(&invalid)
			if err := invalid.Validate(); err == nil {
				t.Fatal("expected invalid love journey material config")
			}
		})
	}
	sixDigit := document
	sixDigit.Config.LetterPassword = "012345"
	if err := sixDigit.Validate(); err == nil {
		t.Fatal("expected six digit love journey password to be rejected")
	}
}

func TestDecodeRejectsUnknownFields(t *testing.T) {
	_, err := Decode([]byte(`{"templateId":"memory-game","templateVersion":"1.0.0","configVersion":1,"config":{"openingTitle":"回忆","rounds":[]},"script":"alert(1)"}`))
	if err == nil {
		t.Fatal("expected unknown executable field to be rejected")
	}
}
