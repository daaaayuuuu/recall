package aisettings

import (
	"bytes"
	"context"
	"errors"
	"testing"
	"time"

	"gamegen/backend/internal/platform/config"
)

const testEncryptionKey = "AgICAgICAgICAgICAgICAgICAgICAgICAgICAgICAgI="

type memoryStore struct {
	record Record
	saved  SaveInput
	err    error
}

func (store *memoryStore) Current(context.Context) (Record, error) {
	return store.record, store.err
}

func (store *memoryStore) Save(_ context.Context, input SaveInput) error {
	if store.err != nil {
		return store.err
	}
	if input.ExpectedVersion != store.record.Version {
		return ErrVersionConflict
	}
	store.saved = input
	store.record = input.Record
	return nil
}

func TestManagerPublishesEncryptedVersionAndReturnsEffectiveConfig(t *testing.T) {
	store := &memoryStore{}
	base := testBaseAIConfig()
	manager, err := NewManager(store, base, "test", config.DynamicAIConfig{
		Enabled: true, EncryptionKeyV1: testEncryptionKey, EncryptionKeyVersion: 1, RefreshInterval: time.Second,
	})
	if err != nil {
		t.Fatal(err)
	}
	settings := snapshotFromConfig(base)
	settings.Text.Enabled = true
	view, fields, err := manager.Publish(
		context.Background(), 0, settings,
		SecretChanges{Text: SecretChange{Value: "dynamic-secret-1234"}},
		"session-1", "admin", "request-1",
	)
	if err != nil || len(fields) != 0 {
		t.Fatalf("publish fields=%v err=%v", fields, err)
	}
	if view.Version != 1 || view.Source != "admin" || !view.APIKeys.Text.Configured || view.APIKeys.Text.Hint != "••••1234" {
		t.Fatalf("unexpected view: %#v", view)
	}
	if store.saved.AdminSessionID != "session-1" || store.saved.RequestID != "request-1" || store.saved.Record.Version != 1 {
		t.Fatalf("missing audit/version metadata: %#v", store.saved)
	}
	if len(store.saved.Record.TextSecret.Ciphertext) == 0 || bytes.Contains(store.saved.Record.TextSecret.Ciphertext, []byte("dynamic-secret-1234")) {
		t.Fatal("API key was not encrypted before persistence")
	}
	effective, err := manager.Current(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if effective.Text.APIKey != "dynamic-secret-1234" || effective.Text.Model != base.Text.Model {
		t.Fatalf("unexpected effective text config: %#v", effective.Text)
	}
}

func TestManagerUsesEnvironmentUntilFirstPublishedVersion(t *testing.T) {
	base := testBaseAIConfig()
	base.Text.APIKey = "environment-secret"
	manager, err := NewManager(&memoryStore{}, base, "test", config.DynamicAIConfig{
		Enabled: true, EncryptionKeyV1: testEncryptionKey, EncryptionKeyVersion: 1, RefreshInterval: time.Second,
	})
	if err != nil {
		t.Fatal(err)
	}
	view, err := manager.View(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if view.Version != 0 || view.Source != "environment" || view.APIKeys.Text.Source != "environment" || !view.Settings.Text.Enabled {
		t.Fatalf("unexpected environment view: %#v", view)
	}
}

func TestManagerRejectsMissingKeyAndVersionConflict(t *testing.T) {
	store := &memoryStore{}
	base := testBaseAIConfig()
	manager, err := NewManager(store, base, "test", config.DynamicAIConfig{
		Enabled: true, EncryptionKeyV1: testEncryptionKey, EncryptionKeyVersion: 1, RefreshInterval: time.Second,
	})
	if err != nil {
		t.Fatal(err)
	}
	settings := snapshotFromConfig(base)
	settings.ImageModeration.Enabled = true
	_, fields, err := manager.Publish(context.Background(), 0, settings, SecretChanges{}, "session", "admin", "request")
	if err != nil || fields["imageModeration.apiKey"] == "" {
		t.Fatalf("expected API key validation, fields=%v err=%v", fields, err)
	}
	store.record.Version = 2
	_, _, err = manager.Publish(context.Background(), 1, snapshotFromConfig(base), SecretChanges{}, "session", "admin", "request")
	if !errors.Is(err, ErrVersionConflict) {
		t.Fatalf("expected version conflict, got %v", err)
	}
}

func testBaseAIConfig() config.AIConfig {
	return config.AIConfig{
		Text: config.AITextConfig{
			Provider: "deepseek", BaseURL: "https://api.deepseek.com", Model: "deepseek-v4-flash",
			Timeout: 30 * time.Second, TimeoutEvery: "30s", MaxOutputTokens: 2000,
		},
		ImageModeration: config.AIImageModerationConfig{
			Timeout: 20 * time.Second, TimeoutEvery: "20s", MaxOutputTokens: 300,
		},
		ImageToImage: config.AIImageToImageConfig{
			Quality: "medium", Timeout: 3 * time.Minute, TimeoutEvery: "3m",
			MaxInputBytes: 25 << 20, MaxOutputBytes: 25 << 20,
		},
	}
}
