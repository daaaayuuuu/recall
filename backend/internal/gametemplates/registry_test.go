package gametemplates

import "testing"

func TestLoveJourneyUsesUnifiedMaterialInputs(t *testing.T) {
	definition, ok := Find(LoveJourneyID, LoveJourneyVersion)
	if !ok {
		t.Fatal("love journey template must be registered")
	}
	if !definition.GenerationEnabled {
		t.Fatal("love journey placeholder generation must be enabled")
	}

	if definition.InputSchemaVersion != 3 {
		t.Fatalf("unexpected input schema version: %d", definition.InputSchemaVersion)
	}
	password, ok := definition.TextInput("letterPassword")
	if !ok || password.MinLength != 4 || password.MaxLength != 4 || password.Format != "four-digit-code" {
		t.Fatalf("unexpected letter password contract: %#v", password)
	}
	cover, ok := definition.AssetSlot(CoverSlotKey)
	if !ok || cover.Required || cover.MaxItems != 1 {
		t.Fatalf("unexpected couple selfie slot: %#v", cover)
	}
	travel, ok := definition.AssetSlot("travelPhotos")
	if !ok || travel.Required || travel.MinItems != 0 || travel.MaxItems != 2 {
		t.Fatalf("unexpected travel photo slot: %#v", travel)
	}
}

func TestLoveJourneyValidatesLetterAndPassword(t *testing.T) {
	definition, _ := Find(LoveJourneyID, LoveJourneyVersion)
	values, fields := definition.ValidateSceneInputs(map[string]string{
		"loveLetter": "  一封情书  ", "passwordHint": "纪念日", "letterPassword": "0820",
	})
	if len(fields) != 0 || values["letterPassword"] != "0820" || values["loveLetter"] != "一封情书" {
		t.Fatalf("expected valid normalized inputs, values=%#v fields=%#v", values, fields)
	}

	_, fields = definition.ValidateSceneInputs(map[string]string{"loveLetter": "hello", "letterPassword": "1A20"})
	if fields["sceneInputs.letterPassword"] != "请输入 4 位数字密码" {
		t.Fatalf("expected format error to remain visible, got %#v", fields)
	}
	_, fields = definition.ValidateSceneInputs(map[string]string{"loveLetter": "hello", "letterPassword": "123"})
	if fields["sceneInputs.letterPassword"] != "不能少于 4 个字符" {
		t.Fatalf("expected length error to remain visible, got %#v", fields)
	}
	_, fields = definition.ValidateSceneInputs(map[string]string{"letterPassword": "1234"})
	if fields["sceneInputs.loveLetter"] == "" {
		t.Fatalf("expected required love letter validation, got %#v", fields)
	}
}

func TestLoveJourneyLegacyDefinitionRemainsAvailable(t *testing.T) {
	definition, ok := Find(LoveJourneyID, LoveJourneyLegacyVersion)
	if !ok {
		t.Fatal("legacy love journey template must remain available")
	}
	partner, ok := definition.AssetSlot("firstMeetingPartnerPhoto")
	if !ok || partner.MinItems != 1 || partner.MaxItems != 1 {
		t.Fatalf("unexpected legacy partner photo slot: %#v", partner)
	}
}
